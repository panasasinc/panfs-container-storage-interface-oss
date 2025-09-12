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

package driver_test

import (
	"context"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/assert"

	"github.com/panasasinc/panfs-container-storage-interface/pkg/driver"
)

// TestDriver_GetPluginInfo tests the GetPluginInfo method of the Driver.
// It verifies correct plugin info is returned and error handling for missing driver name.
func TestDriver_GetPluginInfo(t *testing.T) {
	tests := []struct {
		name       string
		driverName string
		driverVer  string
		wantErr    bool
		wantResp   *csi.GetPluginInfoResponse
	}{
		{
			name:       "valid plugin info",
			driverName: "test-driver",
			driverVer:  "v1.0.0",
			wantErr:    false,
			wantResp: &csi.GetPluginInfoResponse{
				Name:          "test-driver",
				VendorVersion: "v1.0.0",
			},
		},
		{
			name:       "missing driver name",
			driverName: "",
			driverVer:  "v1.0.0",
			wantErr:    true,
			wantResp:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &driver.Driver{
				Name:    tt.driverName,
				Version: tt.driverVer,
			}
			resp, err := d.GetPluginInfo(context.Background(), &csi.GetPluginInfoRequest{})
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, resp)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantResp, resp)
			}
		})
	}
}

// TestDriver_GetPluginCapabilities tests the GetPluginCapabilities method of the Driver.
// It verifies that the default plugin capabilities are returned as expected.
func TestDriver_GetPluginCapabilities(t *testing.T) {
	tests := []struct {
		name     string
		driver   *driver.Driver
		wantCaps []*csi.PluginCapability
	}{
		{
			name:   "default capabilities",
			driver: &driver.Driver{},
			wantCaps: []*csi.PluginCapability{
				{
					Type: &csi.PluginCapability_Service_{
						Service: &csi.PluginCapability_Service{
							Type: csi.PluginCapability_Service_CONTROLLER_SERVICE,
						},
					},
				},
				{
					Type: &csi.PluginCapability_VolumeExpansion_{
						VolumeExpansion: &csi.PluginCapability_VolumeExpansion{
							Type: csi.PluginCapability_VolumeExpansion_ONLINE,
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := tt.driver.GetPluginCapabilities(context.Background(), &csi.GetPluginCapabilitiesRequest{})
			assert.NoError(t, err)
			assert.Equal(t, tt.wantCaps, resp.Capabilities)
		})
	}
}

// TestDriver_Probe tests the Probe method of the Driver.
// It verifies that the probe returns a healthy response and no error.
func TestDriver_Probe(t *testing.T) {
	tests := []struct {
		name   string
		driver *driver.Driver
	}{
		{
			name:   "probe healthy",
			driver: &driver.Driver{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := tt.driver.Probe(context.Background(), &csi.ProbeRequest{})
			assert.NoError(t, err)
			assert.NotNil(t, resp)
		})
	}
}
