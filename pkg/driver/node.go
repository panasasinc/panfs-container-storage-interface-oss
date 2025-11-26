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
	"os"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/panasasinc/panfs-container-storage-interface-oss/pkg/utils"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	// NodeLabelKey is the Kubernetes node label key used to indicate the readiness of the PanFS CSI driver on the node.
	NodeLabelKey = "node.kubernetes.io/csi-driver.panfs.ready"
)

var (
	// IsNodeLabelSet tracks whether the node label has been set to avoid redundant updates.
	IsNodeLabelSet = false
)

// Mockable OS functions
var (
	osMkdirAll = os.MkdirAll
	osChmod    = os.Chmod
	osRemove   = os.Remove
)

// NodeStageVolume handles the CSI NodeStageVolume request.
// Logs the request and returns an unimplemented error.
//
// Parameters:
//
//	ctx - The context for the request.
//	in  - The NodeStageVolumeRequest containing volume details.
//
// Returns:
//
//	*csi.NodeStageVolumeResponse - Always nil.
//	error - Always returns codes.Unimplemented.
func (d *Driver) NodeStageVolume(ctx context.Context, in *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	llog := d.log.WithValues("method", "NodeStageVolume")
	llog.V(2).Info("NodeStageVolume called",
		"volume_id", in.VolumeId,
		"publish_context", in.PublishContext,
		"staging_target_path", in.StagingTargetPath,
		"volume_capability", in.VolumeCapability,
		"volume_context", in.VolumeContext)

	return nil, status.Error(codes.Unimplemented, "")
}

// NodeUnstageVolume handles the CSI NodeUnstageVolume request.
// Logs the request and returns an unimplemented error.
//
// Parameters:
//
//	ctx - The context for the request.
//	in  - The NodeUnstageVolumeRequest containing volume details.
//
// Returns:
//
//	*csi.NodeUnstageVolumeResponse - Always nil.
//	error - Always returns codes.Unimplemented.
func (d *Driver) NodeUnstageVolume(ctx context.Context, in *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	llog := d.log.WithValues("method", "NodeUnstageVolume")
	llog.V(2).Info("NodeUnstageVolume called", "volume_id", in.VolumeId, "staging_path", in.StagingTargetPath)
	return nil, status.Error(codes.Unimplemented, "")
}

// NodePublishVolume handles the CSI NodePublishVolume request.
// Publishes the volume to the target path, validates input, and performs mount operations.
// Returns error for invalid input, unsupported capabilities, or mount failures.
//
// Parameters:
//
//	ctx - The context for the request.
//	in  - The NodePublishVolumeRequest containing volume, target, and capability details.
//
// Returns:
//
//	*csi.NodePublishVolumeResponse - The response on success.
//	error - Returns error for invalid input, unsupported capability, or mount failure.
func (d *Driver) NodePublishVolume(ctx context.Context, in *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	llog := d.log.WithValues("method", "NodePublishVolume")
	llog.V(2).Info("NodePublishVolume called",
		"volume_id", in.VolumeId,
		"publish_context", in.PublishContext,
		"staging_target_path", in.StagingTargetPath,
		"target_path", in.TargetPath,
		"volume_capability", in.VolumeCapability,
		"readonly", in.Readonly,
		"volume_context", in.VolumeContext)

	volumeID := in.GetVolumeId()
	if volumeID == "" {
		llog.Error(fmt.Errorf("volume id must not be empty"), InvalidRequestErrorStr)
		return nil, status.Error(codes.InvalidArgument, "Volume id must be provided")
	}

	secrets := in.GetSecrets()
	if err := validateReqSecrets(secrets); err != nil {
		llog.Error(err, InvalidRequestSecretsErrorStr)
		return nil, status.Error(codes.InvalidArgument, InvalidRequestSecretsErrorStr)
	}

	publishTargetPath := in.GetTargetPath()
	if publishTargetPath == "" {
		llog.Error(fmt.Errorf("target path must not be empty"), InvalidRequestErrorStr)
		return nil, status.Error(codes.InvalidArgument, "Target Path must be provided")
	}

	volumeCapability := in.GetVolumeCapability()
	if volumeCapability == nil {
		llog.Error(fmt.Errorf("volume capability must not be empty"), InvalidRequestErrorStr)
		return nil, status.Error(codes.InvalidArgument, "Volume Capability must be provided")
	}

	if !d.isSupportedCapability(in.GetVolumeCapability()) {
		llog.Error(fmt.Errorf("unsupported volume capability"), "unsupported volume capability provided",
			"volume_capability", in.GetVolumeCapability())
		return nil, status.Error(codes.FailedPrecondition, "unsupported volume capability provided")
	}

	if isEphemeral, ok := in.VolumeContext[EphemeralK8SVolumeContext]; ok && isEphemeral == "true" {
		llog.Error(fmt.Errorf("ephemeral volumes are not supported by this driver"), "Unsupported ephemeral volume requested")
		return nil, status.Error(codes.FailedPrecondition, "Ephemeral volumes are not supported by this driver")
	}

	mountOptions := volumeCapability.GetMount().GetMountFlags()
	if in.GetReadonly() {
		mountOptions = append(mountOptions, "ro")
	}

	if encryptionVal, ok := in.VolumeContext[utils.VolumeParameters.GetPanKey("encryption")]; ok && encryptionVal == "on" {
		// Create a temporary KMIP Config file
		if err := osMkdirAll("/var/tmp/kmip/", 0o700); err != nil {
			llog.Error(err, "failed to create temp directory for KMIP config file")
			return nil, status.Error(codes.Internal, "Failed to create temp directory for KMIP config file: "+err.Error())
		}

		kmipConfigFile, err := d.tempFileFactory.CreateTemp("/var/tmp/kmip/", "config_*.conf")
		if err != nil {
			llog.Error(err, "failed to create temporary KMIP config file for mounting")
			return nil, status.Error(codes.Internal, "Failed to create KMIP config file: "+err.Error())
		}

		// Cleanup the temp file after mount operation, checking errors
		defer func() {
			if err := kmipConfigFile.Close(); err != nil {
				llog.Error(err, "failed to close KMIP config file")
			}
		}()

		defer func() {
			if err := osRemove(kmipConfigFile.Name()); err != nil {
				llog.Error(err, "failed to remove KMIP config file")
			}
		}()

		// Set file permissions to 0700
		err = osChmod(kmipConfigFile.Name(), 0o600)
		if err != nil {
			llog.Error(err, "failed to set '0700' permissions on KMIP config file")
			return nil, status.Error(codes.Internal, "Failed to set '0700' permissions on KMIP config file: "+err.Error())
		}

		if in.Secrets[utils.RealmConnectionContext.KMIPConfigData] == "" {
			llog.Error(fmt.Errorf("%s key is empty", utils.RealmConnectionContext.KMIPConfigData), "KMIP secret must be provided for encrypted volumes")
			return nil, status.Error(codes.InvalidArgument, "KMIP secret must be provided for encrypted volumes")
		}

		data := []byte(in.Secrets[utils.RealmConnectionContext.KMIPConfigData])
		if _, err := kmipConfigFile.Write(data); err != nil {
			llog.Error(err, "failed to write KMIP config data to temporary file")
			return nil, status.Error(codes.Internal, "Failed to write KMIP config data to temporary file: "+err.Error())
		}

		mountOptions = append(mountOptions, fmt.Sprintf("kmip-config-file=%s", kmipConfigFile.Name()))
	}

	if err := d.mounterV2.Mount(fmt.Sprintf("panfs://%s/%s", in.GetSecrets()[utils.RealmConnectionContext.RealmAddress], volumeID), publishTargetPath, mountOptions); err != nil {
		llog.Error(fmt.Errorf("failed to publish volume"), UnexpectedErrorInternalStr,
			"volume_id", volumeID,
			"publish_target_path", publishTargetPath,
			"mount_options", mountOptions)
		return nil, status.Error(codes.Internal, "Failed to publish volume: "+err.Error())
	}

	llog.Info("successfully published volume",
		"volume_id", volumeID,
		"publish_path", publishTargetPath)
	return &csi.NodePublishVolumeResponse{}, nil
}

// NodeUnpublishVolume handles the CSI NodeUnpublishVolume request.
// Unpublishes the volume from the target path, validates input, and performs unmount operations.
// Returns error for invalid input or unmount failures.
//
// Parameters:
//
//	ctx - The context for the request.
//	in  - The NodeUnpublishVolumeRequest containing volume and target details.
//
// Returns:
//
//	*csi.NodeUnpublishVolumeResponse - The response on success.
//	error - Returns error for invalid input or unmount failure.
func (d *Driver) NodeUnpublishVolume(ctx context.Context, in *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	llog := d.log.WithValues("method", "NodeUnpublishVolume")
	llog.V(2).Info("NodeUnpublishVolume called",
		"volume_id", in.VolumeId,
		"target_path", in.TargetPath)
	volumeID := in.GetVolumeId()
	if volumeID == "" {
		llog.Error(fmt.Errorf("volume id must not be empty"), InvalidRequestErrorStr)
		return nil, status.Error(codes.InvalidArgument, "Volume id must be provided")
	}

	publishTargetPath := in.GetTargetPath()
	if publishTargetPath == "" {
		llog.Error(fmt.Errorf("target path must not be empty"), "Target Path must be provided")
		return nil, status.Error(codes.InvalidArgument, "Target Path must be provided")
	}

	if err := d.mounterV2.Unmount(publishTargetPath); err != nil {
		llog.Error(err, "failed to unpublish volume", "volume_id", volumeID)
		return nil, status.Error(codes.Internal, "Failed to unpublish volume: "+err.Error())
	}

	llog.V(2).Info("Successfully unpublished volume",
		"volume_id", volumeID,
		"publish_path", publishTargetPath)
	return &csi.NodeUnpublishVolumeResponse{}, nil
}

// NodeGetCapabilities handles the CSI NodeGetCapabilities request.
// Returns the supported node service capabilities for the CSI driver.
//
// Parameters:
//
//	ctx - The context for the request.
//	in  - The NodeGetCapabilitiesRequest.
//
// Returns:
//
//	*csi.NodeGetCapabilitiesResponse - The response containing supported capabilities.
//	error - Always nil.
func (d *Driver) NodeGetCapabilities(ctx context.Context, in *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	d.log.V(2).Info("NodeGetCapabilities called")

	return &csi.NodeGetCapabilitiesResponse{
		Capabilities: []*csi.NodeServiceCapability{
			{
				Type: &csi.NodeServiceCapability_Rpc{
					Rpc: &csi.NodeServiceCapability_RPC{
						Type: csi.NodeServiceCapability_RPC_SINGLE_NODE_MULTI_WRITER,
					},
				},
			},
		},
	}, nil
}

// NodeExpandVolume handles the CSI NodeExpandVolume request.
// Logs the request and returns an unimplemented error.
//
// Parameters:
//
//	ctx - The context for the request.
//	in  - The NodeExpandVolumeRequest containing volume and capacity details.
//
// Returns:
//
//	*csi.NodeExpandVolumeResponse - Always nil.
//	error - Always returns codes.Unimplemented.
func (d *Driver) NodeExpandVolume(ctx context.Context, in *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	d.log.V(2).Info("NodeExpandVolume called",
		"volume_id", in.VolumeId,
		"volume_path", in.VolumePath,
		"capacity_range", in.CapacityRange,
		"staging_target_path", in.StagingTargetPath,
		"volume_capability", in.VolumeCapability)
	return nil, status.Error(codes.Unimplemented, "")
}

// NodeGetInfo handles the CSI NodeGetInfo request.
// Returns the node ID and maximum volumes per node.
//
// Parameters:
//
//	ctx - The context for the request.
//	in  - The NodeGetInfoRequest.
//
// Returns:
//
//	*csi.NodeGetInfoResponse - The response containing node info.
//	error - Always nil.
func (d *Driver) NodeGetInfo(ctx context.Context, in *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	d.log.V(2).Info("NodeGetInfo called")

	// Set the label when starting up
	nodeLabelValue := "true"
	if err := d.updateNodeLabel(NodeLabelKey, nodeLabelValue); err != nil {
		d.log.Error(err, "failed to set node label")
		return &csi.NodeGetInfoResponse{
			NodeId: d.host,
			AccessibleTopology: &csi.Topology{
				Segments: map[string]string{},
			},
			MaxVolumesPerNode: 0,
		}, nil
	}

	return &csi.NodeGetInfoResponse{
		NodeId: d.host,
		AccessibleTopology: &csi.Topology{
			Segments: map[string]string{
				NodeLabelKey: nodeLabelValue,
			},
		},
		MaxVolumesPerNode: 0,
	}, nil
}

// NodeGetVolumeStats handles the CSI NodeGetVolumeStats request.
// Logs the request and returns an unimplemented error.
//
// Parameters:
//
//	ctx - The context for the request.
//	in  - The NodeGetVolumeStatsRequest containing volume and path details.
//
// Returns:
//
//	*csi.NodeGetVolumeStatsResponse - Always nil.
//	error - Always returns codes.Unimplemented.
func (d *Driver) NodeGetVolumeStats(ctx context.Context, in *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	d.log.V(2).Info("NodeGetVolumeStats called",
		"volume_id", in.VolumeId,
		"volume_path", in.VolumePath,
		"staging_target_path", in.StagingTargetPath)
	return nil, status.Error(codes.Unimplemented, "")
}
