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
	"errors"
	"fmt"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/panasasinc/panfs-container-storage-interface-oss/pkg/pancli"
	"github.com/panasasinc/panfs-container-storage-interface-oss/pkg/utils"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	//lint:ignore U1000 This variable is intentionally kept for future use and should be ignored by the linter
	volumeSupportedAccessModes []csi.VolumeCapability_AccessMode_Mode = []csi.VolumeCapability_AccessMode_Mode{
		csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
		csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY,
		csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY,
		csi.VolumeCapability_AccessMode_MULTI_NODE_SINGLE_WRITER,
		csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
		csi.VolumeCapability_AccessMode_SINGLE_NODE_SINGLE_WRITER,
		csi.VolumeCapability_AccessMode_SINGLE_NODE_MULTI_WRITER,
	}

	controllerCapabilities = []csi.ControllerServiceCapability_RPC_Type{
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
		csi.ControllerServiceCapability_RPC_EXPAND_VOLUME,
		csi.ControllerServiceCapability_RPC_SINGLE_NODE_MULTI_WRITER,
	}
)

// Error definition strings
var (
	InvalidRequestErrorStr               = "Invalid request"
	InvalidRequestSecretsErrorStr        = "Invalid request secrets"
	InvalidCapacityRangeErrorStr         = "Invalid capacity range"
	EmptyVolumeIDErrorStr                = "Volume id must not be empty"
	VolumeNotFoundErrorStr               = "Volume not found"
	VolumeCapabilitiesUnsuportedErrorStr = "Volume capabilities are not supported"
	VolumeCapabilitiesDoNotMatchErrorStr = "Requested volume capabilities do not match existing volume capabilities"
	UnexpectedErrorInternalStr           = "Unexpected internal error"
)

// CreateVolume handles the CSI CreateVolume request.
//
// Parameters:
//
//	ctx - The context for the request, used for cancellation and deadlines.
//	in  - The CreateVolumeRequest containing volume name, capacity, parameters, capabilities, and secrets.
//
// Returns:
//
//	*csi.CreateVolumeResponse - The response containing the created or existing volume details.
//	error - Returns an error if validation fails, capabilities are unsupported, secrets are invalid,
//	        or if volume creation encounters an internal error.
//
// Error Cases:
//   - codes.InvalidArgument: If the request, capabilities, or secrets are invalid.
//   - codes.Internal: For unexpected internal errors during volume creation or verification.
//   - codes.AlreadyExists: If the volume already exists but does not match requested capabilities.
func (d *Driver) CreateVolume(ctx context.Context, in *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	llog := d.log.WithValues("method", "CreateVolume")
	llog.V(2).Info("CreateVolume called",
		"volume_name", in.Name,
		"capacity_range", in.CapacityRange,
		"parameters", in.Parameters,
		"capabilities", in.VolumeCapabilities,
	)

	// basic validation create volume request for correctness
	// this will check required fields and format of the request
	if err := validateCreateVolumeRequest(in); err != nil {
		llog.Error(err, InvalidRequestErrorStr)
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	if err := d.validateVolumeCapabilities(in.GetVolumeCapabilities()); err != nil {
		llog.Error(err, VolumeCapabilitiesUnsuportedErrorStr, "capabilities", in.VolumeCapabilities)
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	secrets := in.GetSecrets()
	if err := validateReqSecrets(secrets); err != nil {
		llog.Error(err, InvalidRequestSecretsErrorStr)
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	volumeName := in.GetName()
	parameters := in.GetParameters()
	if parameters == nil {
		parameters = make(map[string]string)
	}

	// handle capacity range
	cr := in.GetCapacityRange()
	soft, hard := int64(0), int64(0)

	if cr != nil {
		soft = cr.GetRequiredBytes()
		hard = cr.GetLimitBytes()
	}

	parameters[utils.VolumeProvisioningContext.Soft.GetKey()] = fmt.Sprintf("%d", soft)
	parameters[utils.VolumeProvisioningContext.Hard.GetKey()] = fmt.Sprintf("%d", hard)

	vol, err := d.panfs.CreateVolume(volumeName, parameters, secrets)
	if err != nil {
		// if error happens and it is not ErrorAlreadyExist, we return error
		if !errors.Is(err, pancli.ErrorAlreadyExist) {
			d.log.Error(err, "failed to create volume", "volume_id", volumeName)
			return nil, status.Error(codes.Internal, UnexpectedErrorInternalStr)
		}

		// this is ErrorAlreadyExist error - need to check volume matches capabilities
		vol, err := d.panfs.GetVolume(volumeName, secrets)
		if err != nil || vol == nil {
			llog.Error(err, "volume already exists but failed to verify capabilities", "volume_id", volumeName)
			return nil, status.Error(codes.Internal, UnexpectedErrorInternalStr)
		}

		// if volume is not match requested capabilities
		if err := validateVolumeCapacity(in.GetCapacityRange(), vol); err != nil {
			llog.Error(err, "volume already exists, but the capacity does not match", "volume_id", volumeName)
			return nil, status.Error(codes.AlreadyExists, "Volume capacity does not match: "+err.Error())
		}

		// existing volume matches requested capabilities - return OK with existing volume info
		llog.Info("volume already exists", "volume_name", volumeName, "capacity", vol.GetSoftQuotaBytes(), "encryption", vol.GetEncryptionMode())
		return &csi.CreateVolumeResponse{
			Volume: &csi.Volume{
				CapacityBytes: vol.GetSoftQuotaBytes(),
				VolumeId:      volumeName,
				VolumeContext: vol.VolumeContext(),
			},
		}, nil
	}

	llog.Info("volume created", "volume_name", volumeName, "capacity", vol.GetSoftQuotaBytes(), "encryption", vol.GetEncryptionMode())

	requestedEncMode := parameters[utils.VolumeProvisioningContext.Encryption.GetKey()]
	if requestedEncMode == "" {
		requestedEncMode = "off"
	}

	if requestedEncMode != vol.GetEncryptionMode() {
		llog.Error(fmt.Errorf("volume encryption mode does not match the requested one"), "Volume creation error", "volume_name", volumeName, "requested_encryption", requestedEncMode, "actual_encryption", vol.GetEncryptionMode())
	}

	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			CapacityBytes: vol.GetSoftQuotaBytes(),
			VolumeId:      volumeName,
			VolumeContext: vol.VolumeContext(),
		},
	}, nil
}

// DeleteVolume handles the CSI DeleteVolume request.
//
// Parameters:
//
//	ctx - The context for the request.
//	in  - The DeleteVolumeRequest containing the volume ID and secrets.
//
// Returns:
//
//	*csi.DeleteVolumeResponse - The response indicating success or failure.
//	error - Returns an error if validation fails or deletion encounters an internal error.
//
// Error Cases:
//   - codes.InvalidArgument: If the volume ID or secrets are invalid.
//   - codes.Internal: For unexpected internal errors during volume deletion.
func (d *Driver) DeleteVolume(ctx context.Context, in *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	llog := d.log.WithValues("method", "DeleteVolume")
	llog.V(2).Info("DeleteVolume called", "volume_id", in.VolumeId)

	volumeID := in.GetVolumeId()
	if volumeID == "" {
		llog.Error(fmt.Errorf("volume id must be provided"), InvalidRequestErrorStr)
		return nil, status.Error(codes.InvalidArgument, "volume id must be provided")
	}

	secrets := in.GetSecrets()
	if err := validateReqSecrets(secrets); err != nil {
		llog.Error(err, InvalidRequestSecretsErrorStr)
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	err := d.panfs.DeleteVolume(volumeID, secrets)
	// If volume does not exist, we return OK status
	if err != nil && !errors.Is(err, pancli.ErrorNotFound) {
		llog.Error(err, "failed to delete volume", "volume_id", volumeID)
		return nil, status.Error(codes.Internal, UnexpectedErrorInternalStr)
	}
	llog.Info("volume deleted", "volume_id", volumeID)
	return &csi.DeleteVolumeResponse{}, nil
}

// ControllerPublishVolume handles the CSI ControllerPublishVolume request (unimplemented).
//
// Parameters:
//
//	ctx - The context for the request.
//	in  - The ControllerPublishVolumeRequest.
//
// Returns:
//
//	*csi.ControllerPublishVolumeResponse - Always nil.
//	error - Always returns codes.Unimplemented.
func (d *Driver) ControllerPublishVolume(ctx context.Context, in *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	d.log.V(2).Info("ControllerPublishVolume called")
	return nil, status.Error(codes.Unimplemented, "")
}

// ControllerUnpublishVolume handles the CSI ControllerUnpublishVolume request (unimplemented).
//
// Parameters:
//
//	ctx - The context for the request.
//	in  - The ControllerUnpublishVolumeRequest.
//
// Returns:
//
//	*csi.ControllerUnpublishVolumeResponse - Always nil.
//	error - Always returns codes.Unimplemented.
func (d *Driver) ControllerUnpublishVolume(ctx context.Context, in *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	d.log.V(2).Info("ControllerUnpublishVolume called")
	return nil, status.Error(codes.Unimplemented, "")
}

// validateVolumeCapabilities checks if all provided volume capabilities are supported.
//
// Parameters:
//
//	caps - Slice of VolumeCapability objects to validate.
//
// Returns:
//
//	error - Returns an error if any capability is unsupported.
func (d *Driver) validateVolumeCapabilities(caps []*csi.VolumeCapability) error {
	for _, capability := range caps {
		if !d.isSupportedCapability(capability) {
			return fmt.Errorf("unsupported volume capability: %s", capability)
		}
	}

	return nil
}

// isSupportedCapability checks if the provided volume capability is supported.
//
// Parameters:
//
//	capability - The VolumeCapability to check.
//
// Returns:
//
//	bool - True if supported, false otherwise.
func (d *Driver) isSupportedCapability(capability *csi.VolumeCapability) bool {
	// All access modes are supported so we can just skip them and check only access type
	_, ok := capability.GetAccessType().(*csi.VolumeCapability_Mount)
	return ok
}

// ValidateVolumeCapabilities handles the CSI ValidateVolumeCapabilities request.
//
// Parameters:
//
//	ctx - The context for the request.
//	in  - The ValidateVolumeCapabilitiesRequest containing volume ID, capabilities, parameters, and secrets.
//
// Returns:
//
//	*csi.ValidateVolumeCapabilitiesResponse - The response confirming capabilities if valid.
//	error - Returns an error if validation fails or volume is not found.
//
// Error Cases:
//   - codes.InvalidArgument: If the volume ID, capabilities, or secrets are invalid.
//   - codes.NotFound: If the volume does not exist.
//   - codes.Internal: For unexpected internal errors during validation.
func (d *Driver) ValidateVolumeCapabilities(ctx context.Context, in *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	llog := d.log.WithValues("method", "ValidateVolumeCapabilities")
	llog.V(2).Info("ValidateVolumeCapabilities called",
		"volume_id", in.VolumeId,
		"capabilities", in.VolumeCapabilities,
		"parameters", in.Parameters,
		"context", in.VolumeContext,
	)

	volumeID := in.GetVolumeId()
	if len(volumeID) == 0 {
		llog.Error(fmt.Errorf("volume id must not be empty"), InvalidRequestErrorStr)
		return nil, status.Error(codes.InvalidArgument, "volume id must not be empty")
	}

	capabilitiesRequested := in.GetVolumeCapabilities()
	if len(capabilitiesRequested) == 0 {
		llog.Error(fmt.Errorf("volume capabilities must be provided"), InvalidRequestErrorStr)
		return nil, status.Error(codes.InvalidArgument, "volume capabilities must be provided")
	}

	secrets := in.GetSecrets()
	if err := validateReqSecrets(secrets); err != nil {
		llog.Error(err, InvalidRequestSecretsErrorStr)
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	if err := d.validateVolumeCapabilities(capabilitiesRequested); err != nil {
		llog.Error(err, VolumeCapabilitiesDoNotMatchErrorStr, "volume_id", volumeID)
		return nil, status.Error(codes.InvalidArgument, VolumeCapabilitiesDoNotMatchErrorStr)
	}

	_, err := d.panfs.GetVolume(volumeID, secrets)
	if err != nil {
		switch {
		case errors.Is(err, pancli.ErrorNotFound):
			return nil, status.Error(codes.NotFound, VolumeNotFoundErrorStr)
		default:
			llog.Error(err, "failed to get volume", "volume_id", volumeID)
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	return &csi.ValidateVolumeCapabilitiesResponse{
		Confirmed: &csi.ValidateVolumeCapabilitiesResponse_Confirmed{
			VolumeCapabilities: capabilitiesRequested,
		},
	}, nil
}

// ListVolumes handles the CSI ListVolumes request (unimplemented).
//
// Parameters:
//
//	ctx - The context for the request.
//	in  - The ListVolumesRequest.
//
// Returns:
//
//	*csi.ListVolumesResponse - Always nil.
//	error - Always returns codes.Unimplemented.
func (d *Driver) ListVolumes(ctx context.Context, in *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {
	d.log.V(2).Info("ListVolumes called",
		"max_entries", in.MaxEntries,
		"starting_token", in.StartingToken,
	)
	return nil, status.Error(codes.Unimplemented, "")
}

// ControllerGetVolume handles the CSI ControllerGetVolume request (unimplemented).
//
// Parameters:
//
//	ctx - The context for the request.
//	in  - The ControllerGetVolumeRequest.
//
// Returns:
//
//	*csi.ControllerGetVolumeResponse - Always nil.
//	error - Always returns codes.Unimplemented.
func (d *Driver) ControllerGetVolume(ctx context.Context, in *csi.ControllerGetVolumeRequest) (*csi.ControllerGetVolumeResponse, error) {
	d.log.V(2).Info("ControllerGetVolume called",
		"volume_id", in.VolumeId,
	)
	return nil, status.Error(codes.Unimplemented, "")
}

// GetCapacity handles the CSI GetCapacity request (unimplemented).
//
// Parameters:
//
//	ctx - The context for the request.
//	in  - The GetCapacityRequest.
//
// Returns:
//
//	*csi.GetCapacityResponse - Always nil.
//	error - Always returns codes.Unimplemented.
func (d *Driver) GetCapacity(ctx context.Context, in *csi.GetCapacityRequest) (*csi.GetCapacityResponse, error) {
	d.log.V(2).Info("GetCapacity called",
		"volume_capabilities", in.VolumeCapabilities,
		"parameters", in.Parameters,
		"accessible_topology", in.AccessibleTopology,
	)
	return nil, status.Error(codes.Unimplemented, "")
}

// ControllerGetCapabilities handles the CSI ControllerGetCapabilities request.
//
// Parameters:
//
//	ctx - The context for the request.
//	in  - The ControllerGetCapabilitiesRequest.
//
// Returns:
//
//	*csi.ControllerGetCapabilitiesResponse - The response containing supported capabilities.
func (d *Driver) ControllerGetCapabilities(ctx context.Context, in *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	d.log.V(2).Info("ControllerGetCapabilities called")

	var supportedCapabilities []*csi.ControllerServiceCapability
	for _, capability := range controllerCapabilities {
		c := &csi.ControllerServiceCapability{
			Type: &csi.ControllerServiceCapability_Rpc{
				Rpc: &csi.ControllerServiceCapability_RPC{
					Type: capability,
				},
			},
		}
		supportedCapabilities = append(supportedCapabilities, c)
	}

	resp := &csi.ControllerGetCapabilitiesResponse{
		Capabilities: supportedCapabilities,
	}

	return resp, nil
}

// ControllerExpandVolume handles the CSI ControllerExpandVolume request.
//
// Parameters:
//
//	ctx - The context for the request.
//	in  - The ControllerExpandVolumeRequest containing volume ID, capacity range, capability, and secrets.
//
// Returns:
//
//	*csi.ControllerExpandVolumeResponse - The response with expanded volume details.
//	error - Returns an error if validation fails, volume not found, or expansion fails.
//
// Error Cases:
//   - codes.InvalidArgument: If the volume ID, capacity range, or secrets are invalid.
//   - codes.NotFound: If the volume does not exist.
//   - codes.Internal: For unexpected internal errors during expansion.
func (d *Driver) ControllerExpandVolume(ctx context.Context, in *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	llog := d.log.WithValues("method", "ControllerExpandVolume")
	llog.V(2).Info("ControllerExpandVolume called",
		"volume_id", in.VolumeId,
		"capacity_range", in.CapacityRange,
		"volume_capability", in.VolumeCapability,
	)

	volumeID := in.GetVolumeId()
	if len(volumeID) == 0 {
		llog.Error(fmt.Errorf("volume id must be provided"), InvalidRequestErrorStr)
		return nil, status.Error(codes.InvalidArgument, "volume id must be provided")
	}

	capacityRange := in.GetCapacityRange()
	if capacityRange == nil {
		d.log.Error(fmt.Errorf("volume capacity range must be provided"), InvalidCapacityRangeErrorStr)
		return nil, status.Error(codes.InvalidArgument, "volume capacity range must be provided")
	}

	secrets := in.GetSecrets()
	if err := validateReqSecrets(secrets); err != nil {
		llog.Error(err, InvalidRequestSecretsErrorStr)
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	if capacityRange.GetRequiredBytes() <= 0 {
		llog.Error(fmt.Errorf("invalid volume capacity range provided"), "required_bytes must be greater than zero",
			"required", capacityRange.GetRequiredBytes())
		return nil, status.Error(codes.InvalidArgument, InvalidCapacityRangeErrorStr)
	}

	err := d.expandVolume(volumeID, capacityRange, secrets)
	if err != nil {
		switch {
		case errors.Is(err, pancli.ErrorNotFound):
			llog.Error(err, VolumeNotFoundErrorStr, "volume_id", volumeID)
			return nil, status.Error(codes.NotFound, VolumeNotFoundErrorStr)
		default:
			llog.Error(err, "failed to expand volume capacity: "+err.Error(), "volume_id", volumeID)
			return nil, status.Error(codes.Internal, UnexpectedErrorInternalStr)
		}
	}

	requiredBytes := capacityRange.GetRequiredBytes()
	llog.Info("volume expanded successfully", "volume_id", volumeID, "volume_capacity", requiredBytes)
	// Return expanded volume capacity and indicate that volume expansion on the
	// node is not required
	return &csi.ControllerExpandVolumeResponse{
		CapacityBytes:         requiredBytes,
		NodeExpansionRequired: false,
	}, nil
}

// expandVolume performs the volume expansion operation.
//
// Parameters:
//
//	volumeID      - The ID of the volume to expand.
//	capacityRange - The requested capacity range.
//	secrets       - Secrets for authentication.
//
// Returns:
//
//	error - Returns an error if expansion fails.
func (d *Driver) expandVolume(volumeID string, capacityRange *csi.CapacityRange, secrets map[string]string) error {
	// validate required bytes
	requiredBytes := capacityRange.GetRequiredBytes()

	err := d.panfs.ExpandVolume(volumeID, requiredBytes, secrets)
	if err != nil {
		return err
	}
	return nil
}

// CreateSnapshot handles the CSI CreateSnapshot request (unimplemented).
//
// Parameters:
//
//	ctx - The context for the request.
//	in  - The CreateSnapshotRequest.
//
// Returns:
//
//	*csi.CreateSnapshotResponse - Always nil.
//	error - Always returns codes.Unimplemented.
func (d *Driver) CreateSnapshot(ctx context.Context, in *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
	d.log.V(2).Info("CreateSnapshot called",
		"source_volume_id", in.SourceVolumeId,
		"parameters", in.Parameters,
		"snapshot_name", in.Name)

	return nil, status.Error(codes.Unimplemented, "")
}

// DeleteSnapshot handles the CSI DeleteSnapshot request (unimplemented).
//
// Parameters:
//
//	ctx - The context for the request.
//	in  - The DeleteSnapshotRequest.
//
// Returns:
//
//	*csi.DeleteSnapshotResponse - Always nil.
//	error - Always returns codes.Unimplemented.
func (d *Driver) DeleteSnapshot(ctx context.Context, in *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	d.log.V(2).Info("DeleteSnapshot called", "snapshot_id", in.SnapshotId)
	return nil, status.Error(codes.Unimplemented, "")
}

// ListSnapshots handles the CSI ListSnapshots request (unimplemented).
//
// Parameters:
//
//	ctx - The context for the request.
//	in  - The ListSnapshotsRequest.
//
// Returns:
//
//	*csi.ListSnapshotsResponse - Always nil.
//	error - Always returns codes.Unimplemented.
func (d *Driver) ListSnapshots(ctx context.Context, in *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	d.log.V(2).Info("ListSnapshots called",
		"max_entries", in.MaxEntries,
		"starting_token", in.StartingToken,
		"snapshot_id", in.SnapshotId,
		"source_volume_id", in.SourceVolumeId)
	return nil, status.Error(codes.Unimplemented, "")
}
