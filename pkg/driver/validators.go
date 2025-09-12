// Copyright 2025 VDURA Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package driver

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/panasasinc/panfs-container-storage-interface/pkg/pancli"
	"github.com/panasasinc/panfs-container-storage-interface/pkg/utils"
)

var (
	layoutList = []string{"raid6+", "raid5+", "raid10+", "raid5", "raid10"}
	permList   = []string{"none", "read-only", "write-only", "execute-only", "read-write", "read-execute", "write-execute", "all"}
)

// validateVolumeCapacity validates the capacity range for a volume creation request.
// It checks that the required bytes do not exceed the soft quota and that the limit bytes match the hard quota.
//
// Parameters:
//
//	capacity - The requested capacity range for the volume.
//	vol      - The volume object containing soft and hard quota values.
//
// Returns:
//
//	error - Returns an error if requiredBytes exceeds soft quota or limitBytes does not match hard quota.
func validateVolumeCapacity(capacity *csi.CapacityRange, vol *utils.Volume) error {
	requiredBytes := capacity.GetRequiredBytes()
	softBytes := utils.GBToBytes(vol.Soft)

	if requiredBytes != 0 && requiredBytes > softBytes {
		return fmt.Errorf("requiredBytes bytes (%d) exceeds soft quota bytes (%d)", requiredBytes, softBytes)
	}

	limit := capacity.GetLimitBytes()
	hardBytes := utils.GBToBytes(vol.Hard)

	if limit != 0 && limit != hardBytes {
		return fmt.Errorf("limit bytes (%d) not equal to hard quota bytes (%d)", limit, hardBytes)
	}

	return nil
}

// validateCreateVolumeRequest validates the CreateVolumeRequest for correctness.
// Checks for required fields, unsupported content source, and valid capacity range.
//
// Parameters:
//
//	req - The CreateVolumeRequest to validate.
//
// Returns:
//
//	error - Returns an error if validation fails.
func validateCreateVolumeRequest(req *csi.CreateVolumeRequest) error {
	if req.GetName() == "" {
		return fmt.Errorf("name must be provided")
	}

	if len(req.VolumeCapabilities) == 0 {
		return fmt.Errorf("volume_capabilities must be provided")
	}

	// Content source is not supported in this driver
	if req.GetVolumeContentSource() != nil {
		return fmt.Errorf("create volume request with content source is not supported")
	}

	requiredBytes := req.CapacityRange.GetRequiredBytes()
	limitBytes := req.CapacityRange.GetLimitBytes()

	if requiredBytes < 0 {
		return fmt.Errorf("required_bytes (%d) cannot be less than zero", req.CapacityRange.GetRequiredBytes())
	}

	if limitBytes < 0 {
		return fmt.Errorf("limit_bytes (%d) cannot be less than zero", req.CapacityRange.GetLimitBytes())
	}

	if requiredBytes > limitBytes && limitBytes != 0 {
		return fmt.Errorf("required_bytes (%d) should not be greater than limit_bytes (%d)", requiredBytes, limitBytes)
	}

	if err := validateVolumeParameters(req.GetParameters()); err != nil {
		return err
	}

	return nil
}

// validateVolumeParameters validates parameters typically passed from storage class.
// Checks for required values, valid layouts, and correct ranges for numeric parameters.
//
// Parameters:
//
//	parameters - Map of volume parameters to validate.
//
// Returns:
//
//	error - Returns an error if any parameter is invalid.
func validateVolumeParameters(parameters map[string]string) error {
	// Validate optional parameters if they are present
	if val, exist := parameters[bladeSet]; exist && val == "" {
		return fmt.Errorf("%s must be provided", bladeSet)
	}

	if val, exist := parameters[volService]; exist && val == "" {
		return fmt.Errorf("%s must be provided", volService)
	}

	if val, exist := parameters[layout]; exist && !in(val, layoutList...) {
		return fmt.Errorf("%s must be one of: %v", layout, layoutList)
	}

	if val, exist := parameters[maxWidth]; exist {
		intValue, err := strconv.Atoi(val)
		if err != nil {
			return fmt.Errorf("%s is not integer", maxWidth)
		}

		if intValue < 1 {
			return fmt.Errorf("%s must be greater then 0", maxWidth)
		}
		//	todo: The minimum number of OSDs for RAID 5+ is 2; for RAID 6+, the minimum value is 3; for RAID 10+, the minimum value is 2.
	}

	if val, exist := parameters[stripeUnit]; exist {
		if valid := validateStripeUnit(val); !valid {
			return fmt.Errorf("%s is not valid", stripeUnit)
		}
	}

	if val, exist := parameters[rgWidth]; exist {
		intValue, err := strconv.Atoi(val)
		if err != nil {
			return fmt.Errorf("%s is not integer", rgWidth)
		}

		// Any integer between 3 and 20 (inclusive) is a valid width
		if intValue < 3 || intValue > 20 {
			return fmt.Errorf("%s must be between 3 and 20 (inclusive)", rgWidth)
		}

		// todo: Only available for volumes with RAID 6+ or RAID 5+ layout
	}

	if val, exist := parameters[rgDepth]; exist {
		intValue, err := strconv.Atoi(val)
		if err != nil {
			return fmt.Errorf("%s is not integer", rgDepth)
		}

		if intValue < 1 {
			return fmt.Errorf("%s must be greater then 0", rgDepth)
		}

		// todo: This option is only available for volumes with RAID 6+ or RAID 5+ layout.
	}

	if val, exist := parameters[user]; exist && val == "" {
		return fmt.Errorf("%s must be provided", user)
	}

	if val, exist := parameters[group]; exist && val == "" {
		return fmt.Errorf("%s must be provided", group)
	}

	if val, exist := parameters[uPerm]; exist && !in(val, permList...) {
		return fmt.Errorf("%s must be one of: %v", uPerm, permList)
	}

	if val, exist := parameters[gPerm]; exist && !in(val, permList...) {
		return fmt.Errorf("%s must be one of: %v", gPerm, permList)
	}

	if val, exist := parameters[oPerm]; exist && !in(val, permList...) {
		return fmt.Errorf("%s must be one of: %v", oPerm, permList)
	}

	// Additional validation rules can be added here as needed.
	return nil
}

// validateReqSecrets validates the secrets map for required authentication keys.
// Ensures realm, SSH user, and either password or private key are present.
//
// Parameters:
//
//	secrets - Map of secret keys and values.
//
// Returns:
//
//	error - Returns an error if required secrets are missing or invalid.
func validateReqSecrets(secrets map[string]string) error {
	if secrets == nil {
		return fmt.Errorf("secrets must be provided")
	}
	if _, ok := secrets[pancli.AuthSecretRealmKey]; !ok {
		return fmt.Errorf("missing %s in secrets", pancli.AuthSecretRealmKey)
	}

	if _, ok := secrets[pancli.AuthSecretSSHUserKey]; !ok {
		return fmt.Errorf("missing %s in secrets", pancli.AuthSecretSSHUserKey)
	}

	password, ok := secrets[pancli.AuthSecretSSHPasswordKey]
	if !ok {
		password = "" // Default to empty if not provided
	}

	privateKey, ok := secrets[pancli.AuthSecretSSHPrivateKeyKey]
	if !ok {
		privateKey = "" // Default to empty if not provided
	}

	if password == "" && privateKey == "" {
		// If neither password nor private key is provided, return an error.
		return fmt.Errorf("no valid authentication credentials provided in secrets, either password or public key is required")
	}

	return nil
}

// validateStripeUnit checks if the stripe unit string is valid.
// Accepts values in [number]K or [number]M format, within allowed range and divisible by 16K.
//
// Parameters:
//
//	input - The stripe unit string to validate.
//
// Returns:
//
//	bool - True if valid, false otherwise.
func validateStripeUnit(input string) bool {
	// Regular expression pattern to match [number]K or [number]M format
	pattern := `^([1-9][0-9]*)[KkMm]$`
	re := regexp.MustCompile(pattern)

	// Check if input matches the pattern
	if !re.MatchString(input) {
		return false
	}

	// Extract the numeric part of the input
	submatch := re.FindStringSubmatch(input)
	if len(submatch) < 2 {
		return false
	}
	numStr := submatch[1]

	// Convert the numeric part to an integer
	num, err := strconv.Atoi(numStr)
	if err != nil {
		return false
	}

	// Convert megabytes to kilobytes
	// If the unit is megabytes (M or m), convert to kilobytes
	unit := input[len(input)-1]
	if unit == 'M' || unit == 'm' {
		num *= 1024
	}

	// Check if the numeric part is within the valid range
	if num < 1 || num > 4096 {
		return false
	}

	// Check if the stripe unit is divisible by 16K
	if num%16 != 0 {
		return false
	}

	return true
}

// helpers

// in checks if the value is in the provided list of strings.
//
// Parameters:
//
//	value - The string value to check.
//	list  - Variadic list of strings to search.
//
// Returns:
//
//	bool - True if value is in list, false otherwise.
func in(value string, list ...string) bool {
	for i := range list {
		if value == list[i] {
			return true
		}
	}
	return false
}
