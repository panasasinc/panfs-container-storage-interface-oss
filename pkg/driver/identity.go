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
	"context"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog/v2"

	"github.com/container-storage-interface/spec/lib/go/csi"
)

// List of supported plugin capabilities.
// Now we support the following capabilities:
// - Controller Service
// - Online Volume Expansion
var pluginCapabilities = []*csi.PluginCapability{
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
}

// GetPluginInfo returns the name and version of the CSI plugin.
//
// Parameters:
//   ctx - The context for the request.
//   in  - The GetPluginInfoRequest.
//
// Returns:
//   *csi.GetPluginInfoResponse - The response containing plugin name and version.
//   error - Returns an error if the driver name is not configured.
func (d *Driver) GetPluginInfo(ctx context.Context, in *csi.GetPluginInfoRequest) (*csi.GetPluginInfoResponse, error) {
	klog.V(2).Info("GetPluginInfo called")

	if d.Name == "" {
		klog.Error(fmt.Errorf("Driver name is not configured"))
		return nil, status.Error(codes.Unavailable, "Driver name not configured")
	}

	return &csi.GetPluginInfoResponse{
		Name:          d.Name,
		VendorVersion: d.Version,
	}, nil
}

// GetPluginCapabilities returns available capabilities of the plugin.
//
// Parameters:
//   ctx - The context for the request.
//   in  - The GetPluginCapabilitiesRequest.
//
// Returns:
//   *csi.GetPluginCapabilitiesResponse - The response containing supported plugin capabilities.
//   error - Always returns nil.
func (d *Driver) GetPluginCapabilities(ctx context.Context, in *csi.GetPluginCapabilitiesRequest) (*csi.GetPluginCapabilitiesResponse, error) {
	klog.V(2).Info("GetPluginCapabilities called")

	return &csi.GetPluginCapabilitiesResponse{
		Capabilities: pluginCapabilities,
	}, nil
}

// Probe returns the health and readiness of the plugin.
//
// Parameters:
//   ctx - The context for the request.
//   in  - The ProbeRequest.
//
// Returns:
//   *csi.ProbeResponse - The response indicating plugin readiness.
//   error - Always returns nil.
func (d *Driver) Probe(ctx context.Context, in *csi.ProbeRequest) (*csi.ProbeResponse, error) {
	klog.V(2).Info("Probe called")

	return &csi.ProbeResponse{}, nil
}
