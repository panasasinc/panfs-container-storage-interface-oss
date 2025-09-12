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
	"github.com/panasasinc/panfs-container-storage-interface/pkg/utils"
)

// TestValidateVolumeCapacity tests the validateVolumeCapacity function.
// It verifies correct error handling for various capacity and quota scenarios.
func TestValidateVolumeCapacity(t *testing.T) {
	tests := []struct {
		capacity *csi.CapacityRange
		vol      *utils.Volume
		wantErr  bool
	}{
		// Test case 1: required bytes exceeds soft quota bytes
		{
			capacity: &csi.CapacityRange{RequiredBytes: 51 * utils.GBToBytes(1)},
			vol:      &utils.Volume{Soft: 50},
			wantErr:  true,
		},
		// Test case 2: required bytes equal to soft quota bytes
		{
			capacity: &csi.CapacityRange{RequiredBytes: 50 * utils.GBToBytes(1)},
			vol:      &utils.Volume{Soft: 50},
			wantErr:  false,
		},
		// Test case 3: required bytes less then soft quota bytes
		{
			capacity: &csi.CapacityRange{RequiredBytes: 49 * utils.GBToBytes(1)},
			vol:      &utils.Volume{Soft: 50},
			wantErr:  false,
		},
		// Test case 4: limit bytes not equal to hard quota bytes
		{
			capacity: &csi.CapacityRange{LimitBytes: 100},
			vol:      &utils.Volume{Hard: 50},
			wantErr:  true,
		},
		// Test case 5: required and soft bytes match
		{
			capacity: &csi.CapacityRange{RequiredBytes: 53687091200},
			vol:      &utils.Volume{Soft: 50},
			wantErr:  false,
		},
		// Test case 6: limit and hard bytes match
		{
			capacity: &csi.CapacityRange{LimitBytes: 53687091200},
			vol:      &utils.Volume{Hard: 50},
			wantErr:  false,
		},
		// Test case 7: capacity range is nil
		{
			capacity: nil,
			vol:      &utils.Volume{Soft: 50, Hard: 100},
			wantErr:  false,
		},
	}

	for i, tt := range tests {
		err := validateVolumeCapacity(tt.capacity, tt.vol)
		if (err != nil) != tt.wantErr {
			t.Errorf("Test case %d: unexpected error status, got %v, wantErr %v", i+1, err, tt.wantErr)
		}
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
			name: "missing bladeset parameter",
			request: &csi.CreateVolumeRequest{
				Name: "test",
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: 10,
				},
				VolumeCapabilities: []*csi.VolumeCapability{{}},
				Parameters: map[string]string{
					bladeSet: "",
				},
			},
			err: fmt.Errorf("%s must be provided", bladeSet),
		},
		{
			name: "missing volservice parameter",
			request: &csi.CreateVolumeRequest{
				Name: "test",
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: 10,
				},
				VolumeCapabilities: []*csi.VolumeCapability{{}},
				Parameters: map[string]string{
					volService: "",
				},
			},
			err: fmt.Errorf("%s must be provided", volService),
		},
		{
			name: "missing layout parameter",
			request: &csi.CreateVolumeRequest{
				Name: "test",
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: 10,
				},
				VolumeCapabilities: []*csi.VolumeCapability{{}},
				Parameters: map[string]string{
					layout: "",
				},
			},
			err: fmt.Errorf("%s must be one of: %v", layout, layoutList),
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
					maxWidth: "q1",
				},
			},
			err: fmt.Errorf("%s is not integer", maxWidth),
		},
		{
			name: "missing maxwidth parameter",
			request: &csi.CreateVolumeRequest{
				Name: "test",
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: 10,
				},
				VolumeCapabilities: []*csi.VolumeCapability{{}},
				Parameters: map[string]string{
					maxWidth: "",
				},
			},
			err: fmt.Errorf("%s is not integer", maxWidth),
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
					maxWidth: "0",
				},
			},
			err: fmt.Errorf("%s must be greater then 0", maxWidth),
		},
		{
			// todo: add more cases
			name: "missing stripeunit parameter",
			request: &csi.CreateVolumeRequest{
				Name: "test",
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: 10,
				},
				VolumeCapabilities: []*csi.VolumeCapability{{}},
				Parameters: map[string]string{
					stripeUnit: "",
				},
			},
			err: fmt.Errorf("%s is not valid", stripeUnit),
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
					rgWidth: "",
				},
			},
			err: fmt.Errorf("%s is not integer", rgWidth),
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
					rgWidth: "2",
				},
			},
			err: fmt.Errorf("%s must be between 3 and 20 (inclusive)", rgWidth),
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
					rgDepth: "q",
				},
			},
			err: fmt.Errorf("%s is not integer", rgDepth),
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
					rgDepth: "0",
				},
			},
			err: fmt.Errorf("%s must be greater then 0", rgDepth),
		},
		{
			name: "missing user parameter",
			request: &csi.CreateVolumeRequest{
				Name: "test",
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: 10,
				},
				VolumeCapabilities: []*csi.VolumeCapability{{}},
				Parameters: map[string]string{
					user: "",
				},
			},
			err: fmt.Errorf("%s must be provided", user),
		},
		{
			name: "missing group parameter",
			request: &csi.CreateVolumeRequest{
				Name: "test",
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: 10,
				},
				VolumeCapabilities: []*csi.VolumeCapability{{}},
				Parameters: map[string]string{
					group: "",
				},
			},
			err: fmt.Errorf("%s must be provided", group),
		},
		{
			name: "missing uperm parameter",
			request: &csi.CreateVolumeRequest{
				Name: "test",
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: 10,
				},
				VolumeCapabilities: []*csi.VolumeCapability{{}},
				Parameters: map[string]string{
					uPerm: "",
				},
			},
			err: fmt.Errorf("%s must be one of: %v", uPerm, permList),
		},
		{
			name: "missing gperm parameter",
			request: &csi.CreateVolumeRequest{
				Name: "test",
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: 10,
				},
				VolumeCapabilities: []*csi.VolumeCapability{{}},
				Parameters: map[string]string{
					gPerm: "",
				},
			},
			err: fmt.Errorf("%s must be one of: %v", gPerm, permList),
		},
		{
			name: "missing operm parameter",
			request: &csi.CreateVolumeRequest{
				Name: "test",
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: 10,
				},
				VolumeCapabilities: []*csi.VolumeCapability{{}},
				Parameters: map[string]string{
					oPerm: "",
				},
			},
			err: fmt.Errorf("%s must be one of: %v", oPerm, permList),
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
				bladeSet:   "Set-1",
				volService: "vol_service_id",
				layout:     "raid10+",
				maxWidth:   "3",
				stripeUnit: "16K",
				rgWidth:    "9",
				rgDepth:    "7",
				user:       "user_name",
				group:      "group_name",
				uPerm:      "read-only",
				gPerm:      "write-only",
				oPerm:      "none",
			},
			Secrets: map[string]string{
				realmIP:  "10.10.10.10",
				sshUser:  "user",
				password: "password",
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
