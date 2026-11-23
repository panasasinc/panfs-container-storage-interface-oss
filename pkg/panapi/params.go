// Copyright 2026 VDURA Inc.
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

package panapi

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/panasasinc/panfs-container-storage-interface-oss/pkg/utils"
)

// StoragesetIDParameter is the StorageClass parameter that names the storageset of new volumes by its numeric id.
// PanAPI accepts an id only, and a tenant token cannot list storagesets to resolve a name.
const StoragesetIDParameter = utils.VendorPrefix + "storageset_id"

// unsupportedParameters maps StorageClass parameters of the SSH backend that PanAPI cannot honor to what to do instead.
// The driver rejects them, so a StorageClass never carries a setting that silently does nothing.
var unsupportedParameters = map[string]string{
	"storageset":  "use " + StoragesetIDParameter + " with the numeric id of the storageset",
	"description": "PanAPI has no volume description",
	"encryption":  "PanAPI has no encryption setting for a volume",
	"recovery":    "use " + utils.VendorPrefix + "recoverypriority",
}

// UnsupportedParameter returns an error when key is a StorageClass parameter the driver no longer supports.
func UnsupportedParameter(key string) error {
	short, ok := strings.CutPrefix(key, utils.VendorPrefix)
	if !ok {
		return nil
	}
	if hint, unsupported := unsupportedParameters[short]; unsupported {
		return fmt.Errorf("parameter %s is not supported; %s", key, hint)
	}
	return nil
}

// buildCreateRequest translates StorageClass parameters and quotas into a PanAPI create body.
// The storageset id parameter is required.
// Unsupported parameters are rejected; parameters without the vendor prefix, and unknown ones, are skipped.
func buildCreateRequest(p CreateVolumeParams) (*volumeCreateRequest, error) {
	req := &volumeCreateRequest{
		Pathname: "/" + strings.TrimPrefix(p.Name, "/"),
	}
	if p.SoftQuotaBytes > 0 {
		req.SoftQuotaBytes = &p.SoftQuotaBytes
	}
	if p.HardQuotaBytes > 0 {
		req.HardQuotaBytes = &p.HardQuotaBytes
	}

	var err error
	for key, value := range p.Parameters {
		short, ok := strings.CutPrefix(key, utils.VendorPrefix)
		if !ok {
			continue
		}
		if unsupported := UnsupportedParameter(key); unsupported != nil {
			return nil, fmt.Errorf("%w: %v", ErrorInvalidArgument, unsupported)
		}
		if value == "" {
			continue
		}

		switch short {
		case "layout":
			req.RaidType = value
		case "efsa":
			req.EfsaMode = value
		case "volservice":
			req.VolumeServiceID = value
		case "uperm":
			req.UserPermissions = value
		case "gperm":
			req.GroupPermissions = value
		case "operm":
			req.OtherPermissions = value
		case "user":
			req.User, err = numericID(value)
		case "group":
			req.Group, err = numericID(value)
		case "recoverypriority":
			req.RecoveryPriority, err = parseInt(value)
		case "maxwidth":
			req.MaxWidth, err = parseInt(value)
		case "rgwidth":
			req.RGWidth, err = parseInt(value)
		case "rgdepth":
			req.RGDepth, err = parseInt(value)
		case "stripeunit":
			req.StripeUnitBytes, err = parseStripeUnit(value)
		}
		if err != nil {
			return nil, fmt.Errorf("%w: parameter %s: %v", ErrorInvalidArgument, key, err)
		}
	}

	if req.StoragesetID, err = StoragesetID(p.Parameters); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrorInvalidArgument, err)
	}

	return req, nil
}

// StoragesetID reads the required storageset id from StorageClass parameters.
func StoragesetID(parameters map[string]string) (int64, error) {
	value := parameters[StoragesetIDParameter]
	if value == "" {
		return 0, fmt.Errorf("parameter %s must be provided", StoragesetIDParameter)
	}
	id, err := strconv.ParseUint(value, 10, 32)
	if err != nil || id == 0 {
		return 0, fmt.Errorf("parameter %s: %q is not a storageset id", StoragesetIDParameter, value)
	}
	return int64(id), nil
}

// parseInt parses a decimal integer parameter value.
func parseInt(value string) (*int64, error) {
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("%q is not an integer", value)
	}
	return &n, nil
}

// numericID checks that an owner user or group is a numeric id, the only form PanAPI accepts.
func numericID(value string) (string, error) {
	if _, err := strconv.ParseUint(value, 10, 32); err != nil {
		return "", fmt.Errorf("%q is not a numeric id; PanAPI accepts a numeric uid or gid only", value)
	}
	return value, nil
}

// parseStripeUnit converts a stripe unit such as "64k" or "1M" to bytes.
func parseStripeUnit(value string) (*int64, error) {
	if len(value) < 2 {
		return nil, fmt.Errorf("%q is not a size such as 64K or 1M", value)
	}

	var multiplier int64
	switch value[len(value)-1] {
	case 'k', 'K':
		multiplier = 1 << 10
	case 'm', 'M':
		multiplier = 1 << 20
	default:
		return nil, fmt.Errorf("%q is not a size such as 64K or 1M", value)
	}

	n, err := strconv.ParseInt(value[:len(value)-1], 10, 64)
	if err != nil || n <= 0 {
		return nil, fmt.Errorf("%q is not a size such as 64K or 1M", value)
	}

	bytes := n * multiplier
	return &bytes, nil
}
