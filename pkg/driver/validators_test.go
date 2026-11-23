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
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/panasasinc/panfs-container-storage-interface-oss/pkg/panapi"
	"github.com/panasasinc/panfs-container-storage-interface-oss/pkg/utils"
)

// TestValidateVolumeCapacity tests the validateVolumeCapacity function.
// It verifies correct error handling for various capacity and quota scenarios.
func TestValidateVolumeCapacity(t *testing.T) {
	const gb = int64(1) << 30
	volume := func(soft, hard int64) *panapi.Volume {
		return &panapi.Volume{SoftQuotaBytes: &soft, HardQuotaBytes: &hard}
	}

	tests := []struct {
		name     string
		capacity *csi.CapacityRange
		vol      *panapi.Volume
		wantErr  bool
	}{
		{"required exceeds soft quota", &csi.CapacityRange{RequiredBytes: 51 * gb}, volume(50*gb, 0), true},
		{"required exceeds soft quota by one byte", &csi.CapacityRange{RequiredBytes: 50*gb + 1}, volume(50*gb, 0), true},
		{"required equals soft quota", &csi.CapacityRange{RequiredBytes: 50 * gb}, volume(50*gb, 0), false},
		{"required below soft quota", &csi.CapacityRange{RequiredBytes: 49 * gb}, volume(50*gb, 0), false},
		{"limit differs from hard quota", &csi.CapacityRange{LimitBytes: 100}, volume(0, 50*gb), true},
		{"limit equals hard quota", &csi.CapacityRange{LimitBytes: 50 * gb}, volume(0, 50*gb), false},
		{"no capacity range", nil, volume(50*gb, 100*gb), false},
		{"volume without quotas", &csi.CapacityRange{}, &panapi.Volume{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateVolumeCapacity(tt.capacity, tt.vol)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateVolumeCapacity(%v, %+v) error = %v, wantErr %v", tt.capacity, tt.vol, err, tt.wantErr)
			}
		})
	}
}

// TestValidateSecrets tests validateNodeSecrets and validateControllerSecrets.
func TestValidateSecrets(t *testing.T) {
	keys := utils.RealmConnectionContext

	tests := []struct {
		name              string
		secrets           map[string]string
		wantNodeErr       bool
		wantControllerErr bool
	}{
		{"nil secrets", nil, true, true},
		{"empty secrets", map[string]string{}, true, true},
		{"realm only", map[string]string{keys.API: "realm"}, false, true},
		{"empty realm", map[string]string{keys.API: "", keys.Token: "token"}, true, true},
		{"realm and token", map[string]string{keys.API: "realm", keys.Token: "token"}, false, false},
		{"empty token", map[string]string{keys.API: "realm", keys.Token: ""}, false, true},
		{
			"all PanAPI keys",
			map[string]string{
				keys.API: "https://realm:8443", keys.Token: "token",
				keys.CACert: "pem",
			},
			false, false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateNodeSecrets(tt.secrets); (err != nil) != tt.wantNodeErr {
				t.Errorf("validateNodeSecrets(%v) error = %v, wantErr %v", tt.secrets, err, tt.wantNodeErr)
			}
			if err := validateControllerSecrets(tt.secrets); (err != nil) != tt.wantControllerErr {
				t.Errorf("validateControllerSecrets(%v) error = %v, wantErr %v", tt.secrets, err, tt.wantControllerErr)
			}
		})
	}
}

// TestValidateCreateVolumeRequest tests the validateCreateVolumeRequest function.
// It verifies validation logic for required fields, parameters, and error cases.
func TestValidateCreateVolumeRequest(t *testing.T) {
	tests := []struct {
		name    string
		request *csi.CreateVolumeRequest
		err     error
	}{
		{
			name: "empty name",
			request: &csi.CreateVolumeRequest{
				Name: "",
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: 5368709120,
					LimitBytes:    53687091200,
				},
				VolumeCapabilities: []*csi.VolumeCapability{},
			},
			err: fmt.Errorf("name must be provided"),
		},
		{
			name: "missing volume capabilities",
			request: &csi.CreateVolumeRequest{
				Name: "test",
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: 5368709120,
					LimitBytes:    53687091200,
				},
				VolumeCapabilities: nil,
			},
			err: fmt.Errorf("volume_capabilities must be provided"),
		},
		{
			name: "empty volume capabilities",
			request: &csi.CreateVolumeRequest{
				Name: "test",
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: 5368709120,
					LimitBytes:    53687091200,
				},
				VolumeCapabilities: []*csi.VolumeCapability{},
			},
			err: fmt.Errorf("volume_capabilities must be provided"),
		},
		{
			name: "negative required_bytes",
			request: &csi.CreateVolumeRequest{
				Name: "test",
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: -1,
					LimitBytes:    53687091200,
				},
				VolumeCapabilities: []*csi.VolumeCapability{{}},
			},
			err: fmt.Errorf("required_bytes (-1) cannot be less than zero"),
		},
		{
			name: "negative limit_bytes",
			request: &csi.CreateVolumeRequest{
				Name: "test",
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: 5368709120,
					LimitBytes:    -1,
				},
				VolumeCapabilities: []*csi.VolumeCapability{{}},
			},
			err: fmt.Errorf("limit_bytes (-1) cannot be less than zero"),
		},
		{
			name: "required_bytes greater than limit_bytes",
			request: &csi.CreateVolumeRequest{
				Name: "test",
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: 10,
					LimitBytes:    1,
				},
				VolumeCapabilities: []*csi.VolumeCapability{{}},
			},
			err: fmt.Errorf("required_bytes (10) should not be greater than limit_bytes (1)"),
		},
		{
			name: "missing storageset id parameter",
			request: &csi.CreateVolumeRequest{
				Name: "test",
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: 10,
				},
				VolumeCapabilities: []*csi.VolumeCapability{{}},
			},
			err: fmt.Errorf("parameter %s must be provided", panapi.StoragesetIDParameter),
		},
		{
			name: "storageset id parameter is a name",
			request: &csi.CreateVolumeRequest{
				Name: "test",
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: 10,
				},
				VolumeCapabilities: []*csi.VolumeCapability{{}},
				Parameters: map[string]string{
					panapi.StoragesetIDParameter: "Set 1",
				},
			},
			err: fmt.Errorf("parameter %s: %q is not a storageset id", panapi.StoragesetIDParameter, "Set 1"),
		},
		{
			name: "storageset parameter is not supported",
			request: &csi.CreateVolumeRequest{
				Name: "test",
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: 10,
				},
				VolumeCapabilities: []*csi.VolumeCapability{{}},
				Parameters: map[string]string{
					utils.VolumeParameters.GetSCKey("storageset"): "Set 1",
				},
			},
			err: fmt.Errorf("parameter %s is not supported; use %s with the numeric id of the storageset",
				utils.VolumeParameters.GetSCKey("storageset"), panapi.StoragesetIDParameter),
		},
		{
			name: "empty volservice parameter",
			request: &csi.CreateVolumeRequest{
				Name: "test",
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: 10,
				},
				VolumeCapabilities: []*csi.VolumeCapability{{}},
				Parameters: map[string]string{
					utils.VolumeParameters.GetSCKey("volservice"): "",
				},
			},
			err: fmt.Errorf("%s must be provided", utils.VolumeParameters.GetSCKey("volservice")),
		},
		{
			name: "invalid layout parameter",
			request: &csi.CreateVolumeRequest{
				Name: "test",
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: 10,
				},
				VolumeCapabilities: []*csi.VolumeCapability{{}},
				Parameters: map[string]string{
					utils.VolumeParameters.GetSCKey("layout"): "invalid",
				},
			},
			err: fmt.Errorf("%s must be one of: %v", utils.VolumeParameters.GetSCKey("layout"), layoutList),
		},
		{
			name: "invalid maxwidth parameter (alphanumeric)",
			request: &csi.CreateVolumeRequest{
				Name: "test",
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: 10,
				},
				VolumeCapabilities: []*csi.VolumeCapability{{}},
				Parameters: map[string]string{
					utils.VolumeParameters.GetSCKey("maxwidth"): "q1",
				},
			},
			err: fmt.Errorf("%s is not integer", utils.VolumeParameters.GetSCKey("maxwidth")),
		},
		{
			name: "invalid maxwidth parameter",
			request: &csi.CreateVolumeRequest{
				Name: "test",
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: 10,
				},
				VolumeCapabilities: []*csi.VolumeCapability{{}},
				Parameters: map[string]string{
					utils.VolumeParameters.GetSCKey("maxwidth"): "abc",
				},
			},
			err: fmt.Errorf("%s is not integer", utils.VolumeParameters.GetSCKey("maxwidth")),
		},
		{
			name: "invalid maxwidth parameter",
			request: &csi.CreateVolumeRequest{
				Name: "test",
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: 10,
				},
				VolumeCapabilities: []*csi.VolumeCapability{{}},
				Parameters: map[string]string{
					utils.VolumeParameters.GetSCKey("maxwidth"): "0",
				},
			},
			err: fmt.Errorf("%s must be greater then 0", utils.VolumeParameters.GetSCKey("maxwidth")),
		},
		{
			// todo: add more cases
			name: "invalid stripeunit parameter",
			request: &csi.CreateVolumeRequest{
				Name: "test",
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: 10,
				},
				VolumeCapabilities: []*csi.VolumeCapability{{}},
				Parameters: map[string]string{
					utils.VolumeParameters.GetSCKey("stripeunit"): "abc",
				},
			},
			err: fmt.Errorf("%s is not valid", utils.VolumeParameters.GetSCKey("stripeunit")),
		},
		{
			name: "invalid rgwidth parameter",
			request: &csi.CreateVolumeRequest{
				Name: "test",
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: 10,
				},
				VolumeCapabilities: []*csi.VolumeCapability{{}},
				Parameters: map[string]string{
					utils.VolumeParameters.GetSCKey("rgwidth"): "",
				},
			},
			err: fmt.Errorf("%s is not integer", utils.VolumeParameters.GetSCKey("rgwidth")),
		},
		{
			name: "rgwidth parameter is not in range",
			request: &csi.CreateVolumeRequest{
				Name: "test",
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: 10,
				},
				VolumeCapabilities: []*csi.VolumeCapability{{}},
				Parameters: map[string]string{
					utils.VolumeParameters.GetSCKey("rgwidth"): "2",
				},
			},
			err: fmt.Errorf("%s must be between 3 and 20 (inclusive)", utils.VolumeParameters.GetSCKey("rgwidth")),
		},
		{
			name: "invalid rgdepth parameter",
			request: &csi.CreateVolumeRequest{
				Name: "test",
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: 10,
				},
				VolumeCapabilities: []*csi.VolumeCapability{{}},
				Parameters: map[string]string{
					utils.VolumeParameters.GetSCKey("rgdepth"): "q",
				},
			},
			err: fmt.Errorf("%s is not integer", utils.VolumeParameters.GetSCKey("rgdepth")),
		},
		{
			name: "rgdepth parameter is less then minimum",
			request: &csi.CreateVolumeRequest{
				Name: "test",
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: 10,
				},
				VolumeCapabilities: []*csi.VolumeCapability{{}},
				Parameters: map[string]string{
					utils.VolumeParameters.GetSCKey("rgdepth"): "0",
				},
			},
			err: fmt.Errorf("%s must be greater then 0", utils.VolumeParameters.GetSCKey("rgdepth")),
		},
		{
			name: "empty user parameter",
			request: &csi.CreateVolumeRequest{
				Name: "test",
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: 10,
				},
				VolumeCapabilities: []*csi.VolumeCapability{{}},
				Parameters: map[string]string{
					utils.VolumeParameters.GetSCKey("user"): "",
				},
			},
			err: fmt.Errorf("%s must be provided", utils.VolumeParameters.GetSCKey("user")),
		},
		{
			name: "empty group parameter",
			request: &csi.CreateVolumeRequest{
				Name: "test",
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: 10,
				},
				VolumeCapabilities: []*csi.VolumeCapability{{}},
				Parameters: map[string]string{
					utils.VolumeParameters.GetSCKey("group"): "",
				},
			},
			err: fmt.Errorf("%s must be provided", utils.VolumeParameters.GetSCKey("group")),
		},
		{
			name: "invalid uperm parameter",
			request: &csi.CreateVolumeRequest{
				Name: "test",
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: 10,
				},
				VolumeCapabilities: []*csi.VolumeCapability{{}},
				Parameters: map[string]string{
					utils.VolumeParameters.GetSCKey("uperm"): "invalid",
				},
			},
			err: fmt.Errorf("%s must be one of: %v", utils.VolumeParameters.GetSCKey("uperm"), permList),
		},
		{
			name: "invalid gperm parameter",
			request: &csi.CreateVolumeRequest{
				Name: "test",
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: 10,
				},
				VolumeCapabilities: []*csi.VolumeCapability{{}},
				Parameters: map[string]string{
					utils.VolumeParameters.GetSCKey("gperm"): "invalid",
				},
			},
			err: fmt.Errorf("%s must be one of: %v", utils.VolumeParameters.GetSCKey("gperm"), permList),
		},
		{
			name: "invalid operm parameter",
			request: &csi.CreateVolumeRequest{
				Name: "test",
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: 10,
				},
				VolumeCapabilities: []*csi.VolumeCapability{{}},
				Parameters: map[string]string{
					utils.VolumeParameters.GetSCKey("operm"): "invalid",
				},
			},
			err: fmt.Errorf("%s must be one of: %v", utils.VolumeParameters.GetSCKey("operm"), permList),
		},
		{
			name: "encryption parameter is not supported",
			request: &csi.CreateVolumeRequest{
				Name: "test",
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: 10,
				},
				VolumeCapabilities: []*csi.VolumeCapability{{}},
				Parameters: map[string]string{
					utils.VolumeParameters.GetSCKey("encryption"): "off",
				},
			},
			err: fmt.Errorf("parameter %s is not supported; PanAPI has no encryption setting for a volume",
				utils.VolumeParameters.GetSCKey("encryption")),
		},
		{
			name: "volume content source not supported",
			request: &csi.CreateVolumeRequest{
				Name: "test",
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: 10,
				},
				VolumeCapabilities: []*csi.VolumeCapability{{}},
				VolumeContentSource: &csi.VolumeContentSource{
					Type: &csi.VolumeContentSource_Snapshot{
						Snapshot: &csi.VolumeContentSource_SnapshotSource{SnapshotId: "snap-123"},
					},
				},
			},
			err: fmt.Errorf("create volume request with content source is not supported"),
		},
		{
			name: "volume content source not supported with volume source",
			request: &csi.CreateVolumeRequest{
				Name: "test",
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: 10,
				},
				VolumeCapabilities: []*csi.VolumeCapability{{}},
				VolumeContentSource: &csi.VolumeContentSource{
					Type: &csi.VolumeContentSource_Volume{
						Volume: &csi.VolumeContentSource_VolumeSource{VolumeId: "vol-123"},
					},
				},
			},
			err: fmt.Errorf("create volume request with content source is not supported"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCreateVolumeRequest(tt.request)
			if err == nil || err.Error() != tt.err.Error() {
				t.Errorf("unexpected error: %v", err.Error())
			}
		})
	}

	t.Run("valid request", func(t *testing.T) {
		req := &csi.CreateVolumeRequest{
			Name: "test",
			CapacityRange: &csi.CapacityRange{
				RequiredBytes: 5368709120,
				LimitBytes:    53687091200,
			},
			VolumeCapabilities: []*csi.VolumeCapability{{}},
			Parameters: map[string]string{
				panapi.StoragesetIDParameter:                  "1",
				utils.VolumeParameters.GetSCKey("volservice"): "vol_service_id",
				utils.VolumeParameters.GetSCKey("layout"):     "raid10+",
				utils.VolumeParameters.GetSCKey("maxwidth"):   "3",
				utils.VolumeParameters.GetSCKey("stripeunit"): "16K",
				utils.VolumeParameters.GetSCKey("rgwidth"):    "9",
				utils.VolumeParameters.GetSCKey("rgdepth"):    "7",
				utils.VolumeParameters.GetSCKey("user"):       "user_name",
				utils.VolumeParameters.GetSCKey("group"):      "group_name",
				utils.VolumeParameters.GetSCKey("uperm"):      "read-only",
				utils.VolumeParameters.GetSCKey("gperm"):      "write-only",
				utils.VolumeParameters.GetSCKey("operm"):      "none",
			},
			Secrets: map[string]string{
				utils.RealmConnectionContext.API:   "10.11.12.13",
				utils.RealmConnectionContext.Token: "dummy",
			},
		}

		err := validateCreateVolumeRequest(req)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

// TestValidateStripeUnit tests the validateStripeUnit function.
// It verifies correct validation for various stripe unit formats and values.
func TestValidateStripeUnit(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"Valid 1K", "1K", false},
		{"Valid 16K", "16K", true},
		{"Valid 16k", "16k", true},
		{"Valid 32K", "32K", true},
		{"Valid 64K", "64K", true},
		{"Valid 128K", "128K", true},
		{"Valid 256K", "256K", true},
		{"Valid 512K", "512K", true},
		{"Valid 1M", "1M", true},
		{"Valid 1m", "1m", true},
		{"Valid 2M", "2M", true},
		{"Valid 4M", "4M", true},
		{"Invalid 0K", "0K", false},
		{"Invalid 5K", "5K", false},
		{"Invalid 10K", "10K", false},
		{"Invalid 15K", "15K", false},
		{"Invalid 17K", "17K", false},
		{"Invalid 100K", "100K", false},
		{"Invalid 4097K", "4097K", false},
		{"Invalid 5M", "5M", false},
		{"Invalid 10M", "10M", false},
		{"Invalid 100M", "100M", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := validateStripeUnit(tc.input)
			if result != tc.expected {
				t.Errorf("Expected %v, but got %v for input %v", tc.expected, result, tc.input)
			}
		})
	}
}
