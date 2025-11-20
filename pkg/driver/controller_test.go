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
	"github.com/panasasinc/panfs-container-storage-interface-oss/pkg/driver/mock"
	"github.com/panasasinc/panfs-container-storage-interface-oss/pkg/pancli"
	"github.com/panasasinc/panfs-container-storage-interface-oss/pkg/utils"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog/v2"
)

var (
	defaultSecrets = map[string]string{
		utils.RealmConnectionContext.Username:       "user",
		utils.RealmConnectionContext.Password:       "pass",
		utils.RealmConnectionContext.RealmAddress:   "realm",
		utils.RealmConnectionContext.KMIPConfigData: "# some data",
	}
	validVolumeName = "validVolumeName"
	emptyVolumeName = ""
	GB10Bytes       = utils.GBToBytes(10)
)

// TestControllerExpandVolume tests the ControllerExpandVolume method of the Driver struct.
func TestControllerExpandVolume(t *testing.T) {
	ctrl := gomock.NewController(t)
	pancliMock := mock.NewMockStorageProviderClient(ctrl)
	driver := &Driver{
		Version:  "testing",
		Name:     DefaultDriverName,
		endpoint: "unix:///tmp/csi.sock",
		host:     "localhost",
		panfs:    pancliMock,
	}

	testCases := []struct {
		name             string
		req              *csi.ControllerExpandVolumeRequest
		expectedResponse *csi.ControllerExpandVolumeResponse
		expectedError    error
		mockFunc         func()
	}{
		{
			"Success",
			&csi.ControllerExpandVolumeRequest{
				VolumeId:      validVolumeName,
				CapacityRange: &csi.CapacityRange{RequiredBytes: GB10Bytes},
				Secrets:       defaultSecrets,
			},
			&csi.ControllerExpandVolumeResponse{
				CapacityBytes:         GB10Bytes,
				NodeExpansionRequired: false,
			},
			nil,
			func() {
				pancliMock.EXPECT().ExpandVolume(validVolumeName, GB10Bytes, defaultSecrets).Return(nil)
			},
		},
		{
			"EmptyVolumeIdError",
			&csi.ControllerExpandVolumeRequest{
				VolumeId:      emptyVolumeName,
				CapacityRange: &csi.CapacityRange{RequiredBytes: GB10Bytes},
				Secrets:       defaultSecrets,
			},
			nil,
			status.Error(codes.InvalidArgument, "volume id must be provided"),
			func() {
				pancliMock.EXPECT().ExpandVolume(gomock.Any(), gomock.Any(), defaultSecrets).Times(0)
			},
		},
		{
			"EmptyCapacityRangeError",
			&csi.ControllerExpandVolumeRequest{
				VolumeId:      validVolumeName,
				CapacityRange: nil,
				Secrets:       defaultSecrets,
			},
			nil,
			status.Error(codes.InvalidArgument, "volume capacity range must be provided"),
			func() {
				pancliMock.EXPECT().ExpandVolume(gomock.Any(), gomock.Any(), defaultSecrets).Times(0)
			},
		},
		{
			"EmptySecretsError",
			&csi.ControllerExpandVolumeRequest{
				VolumeId:      validVolumeName,
				CapacityRange: &csi.CapacityRange{RequiredBytes: GB10Bytes},
				Secrets:       nil,
			},
			nil,
			status.Error(codes.InvalidArgument, "secrets must be provided"),
			func() {
				pancliMock.EXPECT().ExpandVolume(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
			},
		},
		{
			"ExpandNonExistingVolumeError",
			&csi.ControllerExpandVolumeRequest{
				VolumeId:      validVolumeName,
				CapacityRange: &csi.CapacityRange{RequiredBytes: GB10Bytes},
				Secrets:       defaultSecrets,
			},
			nil,
			status.Error(codes.NotFound, VolumeNotFoundErrorStr),
			func() {
				pancliMock.EXPECT().ExpandVolume(validVolumeName, GB10Bytes, defaultSecrets).Return(pancli.ErrorNotFound)
			},
		},
		{
			"ExpandFailedPancliError",
			&csi.ControllerExpandVolumeRequest{
				VolumeId:      validVolumeName,
				CapacityRange: &csi.CapacityRange{RequiredBytes: GB10Bytes},
				Secrets:       defaultSecrets,
			},
			nil,
			status.Error(codes.Internal, UnexpectedErrorInternalStr),
			func() {
				pancliMock.EXPECT().ExpandVolume(validVolumeName, GB10Bytes, defaultSecrets).Return(pancli.ErrorInternal)
			},
		},
		{
			"RequiredLessThan0",
			&csi.ControllerExpandVolumeRequest{
				VolumeId:      validVolumeName,
				CapacityRange: &csi.CapacityRange{RequiredBytes: -100},
				Secrets:       defaultSecrets,
			},
			nil,
			status.Error(codes.InvalidArgument, InvalidCapacityRangeErrorStr),
			func() {
				pancliMock.EXPECT().ExpandVolume(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.mockFunc != nil {
				tc.mockFunc()
			}
			response, err := driver.ControllerExpandVolume(
				t.Context(),
				tc.req,
			)

			assert.Equal(t, tc.expectedResponse, response)
			assert.ErrorIs(t, err, tc.expectedError)
		})
	}
}

// TestControllerCreateVolume tests the CreateVolume method of the Driver struct.
func TestControllerCreateVolume(t *testing.T) {
	ctrl := gomock.NewController(t)
	pancliMock := mock.NewMockStorageProviderClient(ctrl)
	driver := &Driver{
		Version:  "testing",
		Name:     DefaultDriverName,
		endpoint: "unix:///tmp/csi.sock",
		host:     "localhost",
		panfs:    pancliMock,
		log:      klog.NewKlogr(),
	}

	testCases := []struct {
		name             string
		req              *csi.CreateVolumeRequest
		expectedResponse *csi.CreateVolumeResponse
		expectedError    error
		mockFunc         func()
	}{
		{
			"CreateVolumeSuccess",
			&csi.CreateVolumeRequest{
				Name:          validVolumeName,
				CapacityRange: &csi.CapacityRange{RequiredBytes: GB10Bytes},
				Parameters:    map[string]string{},
				Secrets:       defaultSecrets,
				VolumeCapabilities: []*csi.VolumeCapability{
					{
						AccessType: &csi.VolumeCapability_Mount{
							Mount: &csi.VolumeCapability_MountVolume{},
						},
					},
				},
			},
			&csi.CreateVolumeResponse{
				Volume: &csi.Volume{
					VolumeId:      validVolumeName,
					CapacityBytes: GB10Bytes,
					VolumeContext: map[string]string{},
				},
			},
			nil,
			func() {
				pancliMock.EXPECT().CreateVolume(validVolumeName, gomock.Any(), defaultSecrets).Times(1).Return(
					&utils.Volume{
						Name: utils.VolumeName(validVolumeName),
						Soft: 10.00,
					},
					nil)
			},
		},
		{
			"CreateEncryptedVolumeSuccess",
			&csi.CreateVolumeRequest{
				Name: validVolumeName,
				Parameters: map[string]string{
					utils.VolumeProvisioningContext.Encryption.Key: "on",
				},
				Secrets: defaultSecrets,
				VolumeCapabilities: []*csi.VolumeCapability{
					{
						AccessType: &csi.VolumeCapability_Mount{
							Mount: &csi.VolumeCapability_MountVolume{},
						},
					},
				},
			},
			&csi.CreateVolumeResponse{
				Volume: &csi.Volume{
					VolumeId: validVolumeName,
					VolumeContext: map[string]string{
						utils.VolumeProvisioningContext.Encryption.Key: "on",
					},
				},
			},
			nil,
			func() {
				pancliMock.EXPECT().CreateVolume(validVolumeName, gomock.Any(), defaultSecrets).Times(1).Return(
					&utils.Volume{
						Name:       utils.VolumeName(validVolumeName),
						Encryption: "on",
					},
					nil)
			},
		},
		{
			"VolumeExistsCapabilitiesMatch",
			&csi.CreateVolumeRequest{
				Name:          validVolumeName,
				CapacityRange: &csi.CapacityRange{RequiredBytes: GB10Bytes},
				Parameters:    map[string]string{},
				Secrets:       defaultSecrets,
				VolumeCapabilities: []*csi.VolumeCapability{
					{
						AccessType: &csi.VolumeCapability_Mount{
							Mount: &csi.VolumeCapability_MountVolume{},
						},
					},
				},
			},
			&csi.CreateVolumeResponse{
				Volume: &csi.Volume{
					VolumeId:      validVolumeName,
					CapacityBytes: GB10Bytes,
					VolumeContext: map[string]string{},
				},
			},
			nil,
			func() {
				pancliMock.EXPECT().CreateVolume(validVolumeName, gomock.Any(), defaultSecrets).Times(1).Return(
					&utils.Volume{
						Name: utils.VolumeName(validVolumeName),
						Soft: 10.00,
					},
					pancli.ErrorAlreadyExist,
				)
				pancliMock.EXPECT().GetVolume(validVolumeName, defaultSecrets).Times(1).Return(
					&utils.Volume{
						Name: utils.VolumeName(validVolumeName),
						Soft: 10.00,
					},
					nil,
				)
			},
		},
		{
			"VolumeExistsCapacityDoesNotMatchError",
			&csi.CreateVolumeRequest{
				Name:          validVolumeName,
				CapacityRange: &csi.CapacityRange{RequiredBytes: GB10Bytes},
				Parameters:    map[string]string{},
				Secrets:       defaultSecrets,
				VolumeCapabilities: []*csi.VolumeCapability{
					{
						AccessType: &csi.VolumeCapability_Mount{
							Mount: &csi.VolumeCapability_MountVolume{},
						},
					},
				},
			},
			nil,
			status.Error(codes.AlreadyExists, "Volume capacity does not match: requiredBytes bytes (10737418240) exceeds soft quota bytes (9663676416)"),
			func() {
				pancliMock.EXPECT().CreateVolume(validVolumeName, gomock.Any(), defaultSecrets).Times(1).Return(
					nil,
					pancli.ErrorAlreadyExist,
				)
				pancliMock.EXPECT().GetVolume(validVolumeName, defaultSecrets).Times(1).Return(
					&utils.Volume{
						Name: utils.VolumeName(validVolumeName),
						Soft: 9.00, // Different soft quota to simulate capabilities mismatch
					},
					nil,
				)
			},
		},
		{
			"UnsupportedVolumeCapabilitiesError",
			&csi.CreateVolumeRequest{
				Name:          validVolumeName,
				CapacityRange: &csi.CapacityRange{RequiredBytes: GB10Bytes},
				Parameters:    map[string]string{},
				Secrets:       nil,
				VolumeCapabilities: []*csi.VolumeCapability{
					{
						AccessType: &csi.VolumeCapability_Block{
							Block: &csi.VolumeCapability_BlockVolume{},
						},
					},
				},
			},
			nil,
			status.Error(
				codes.InvalidArgument,
				fmt.Sprintf(
					"unsupported volume capability: %s",
					&csi.VolumeCapability{AccessType: &csi.VolumeCapability_Block{Block: &csi.VolumeCapability_BlockVolume{}}})),
			func() {
				pancliMock.EXPECT().CreateVolume(validVolumeName, gomock.Any(), nil).Times(0)
			},
		},
		{
			"EmptySecretsError",
			&csi.CreateVolumeRequest{
				Name:          validVolumeName,
				CapacityRange: &csi.CapacityRange{RequiredBytes: GB10Bytes},
				Parameters:    map[string]string{},
				Secrets:       nil,
				VolumeCapabilities: []*csi.VolumeCapability{
					{
						AccessType: &csi.VolumeCapability_Mount{
							Mount: &csi.VolumeCapability_MountVolume{},
						},
					},
				},
			},
			nil,
			status.Error(codes.InvalidArgument, "secrets must be provided"),
			func() {
				pancliMock.EXPECT().CreateVolume(validVolumeName, gomock.Any(), nil).Times(0)
			},
		},
		{
			"CreateVolumeInvalidVolumeNameError",
			&csi.CreateVolumeRequest{
				Name:          emptyVolumeName,
				CapacityRange: &csi.CapacityRange{RequiredBytes: GB10Bytes},
				Parameters:    map[string]string{},
				Secrets:       defaultSecrets,
				VolumeCapabilities: []*csi.VolumeCapability{
					{
						AccessType: &csi.VolumeCapability_Mount{
							Mount: &csi.VolumeCapability_MountVolume{},
						},
					},
				},
			},
			nil,
			status.Error(codes.InvalidArgument, "name must be provided"),
			func() {
				pancliMock.EXPECT().CreateVolume(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
			},
		},
		{
			"CreateVolumeInvalidCapacityRangeError",
			&csi.CreateVolumeRequest{
				Name:          validVolumeName,
				CapacityRange: &csi.CapacityRange{RequiredBytes: -100},
				Parameters:    map[string]string{},
				Secrets:       defaultSecrets,
				VolumeCapabilities: []*csi.VolumeCapability{
					{
						AccessType: &csi.VolumeCapability_Mount{
							Mount: &csi.VolumeCapability_MountVolume{},
						},
					},
				},
			},
			nil,
			status.Error(codes.InvalidArgument, fmt.Sprintf("required_bytes (%d) cannot be less than zero", -100)),
			func() {
				pancliMock.EXPECT().CreateVolume(validVolumeName, gomock.Any(), gomock.Any()).Times(0)
			},
		},
		{
			"CreateVolumeInvalidVolumeCapabilitiesError",
			&csi.CreateVolumeRequest{
				Name:               validVolumeName,
				CapacityRange:      &csi.CapacityRange{RequiredBytes: GB10Bytes},
				Parameters:         map[string]string{},
				Secrets:            defaultSecrets,
				VolumeCapabilities: []*csi.VolumeCapability{},
			},
			nil,
			status.Error(codes.InvalidArgument, "volume_capabilities must be provided"),
			func() {
				pancliMock.EXPECT().CreateVolume(validVolumeName, gomock.Any(), gomock.Any()).Times(0)
			},
		},
		{
			"FailedToCreateVolumePancliError",
			&csi.CreateVolumeRequest{
				Name:          validVolumeName,
				CapacityRange: &csi.CapacityRange{RequiredBytes: GB10Bytes},
				Parameters:    map[string]string{},
				Secrets:       defaultSecrets,
				VolumeCapabilities: []*csi.VolumeCapability{
					{
						AccessType: &csi.VolumeCapability_Mount{
							Mount: &csi.VolumeCapability_MountVolume{},
						},
					},
				},
			},
			nil,
			status.Error(codes.Internal, UnexpectedErrorInternalStr),
			func() {
				pancliMock.EXPECT().CreateVolume(validVolumeName, gomock.Any(), gomock.Any()).Times(1).Return(
					nil,
					pancli.ErrorInternal,
				)
			},
		},
		{
			"VolumeExistsFailedToGetDetails",
			&csi.CreateVolumeRequest{
				Name:          validVolumeName,
				CapacityRange: &csi.CapacityRange{RequiredBytes: GB10Bytes},
				Parameters:    map[string]string{},
				Secrets:       defaultSecrets,
				VolumeCapabilities: []*csi.VolumeCapability{
					{
						AccessType: &csi.VolumeCapability_Mount{
							Mount: &csi.VolumeCapability_MountVolume{},
						},
					},
				},
			},
			nil,
			status.Error(codes.Internal, UnexpectedErrorInternalStr),
			func() {
				pancliMock.EXPECT().CreateVolume(validVolumeName, gomock.Any(), gomock.Any()).Times(1).Return(
					nil,
					pancli.ErrorAlreadyExist,
				)
				pancliMock.EXPECT().GetVolume(validVolumeName, defaultSecrets).Times(1).Return(
					nil,
					pancli.ErrorInternal,
				)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.mockFunc != nil {
				tc.mockFunc()
			}
			response, err := driver.CreateVolume(t.Context(), tc.req)
			assert.Equal(t, tc.expectedResponse, response)
			assert.ErrorIs(t, err, tc.expectedError)
		})
	}
}

// TestControllerDeleteVolume tests the DeleteVolume method of the Driver struct.
func TestControllerDeleteVolume(t *testing.T) {
	ctrl := gomock.NewController(t)
	pancliMock := mock.NewMockStorageProviderClient(ctrl)
	driver := &Driver{
		Version:  "testing",
		Name:     DefaultDriverName,
		endpoint: "unix:///tmp/csi.sock",
		host:     "localhost",
		panfs:    pancliMock,
	}

	testCases := []struct {
		name             string
		req              *csi.DeleteVolumeRequest
		expectedResponse *csi.DeleteVolumeResponse
		expectedError    error
		mockFunc         func()
	}{
		{
			name: "DeleteVolumeSuccess",
			req: &csi.DeleteVolumeRequest{
				VolumeId: validVolumeName,
				Secrets:  defaultSecrets,
			},
			expectedResponse: &csi.DeleteVolumeResponse{},
			expectedError:    nil,
			mockFunc: func() {
				pancliMock.EXPECT().DeleteVolume(validVolumeName, defaultSecrets).Return(nil)
			},
		},
		{
			name: "VolumeNotFoundReturnsOK",
			req: &csi.DeleteVolumeRequest{
				VolumeId: validVolumeName,
				Secrets:  defaultSecrets,
			},
			expectedResponse: &csi.DeleteVolumeResponse{},
			expectedError:    nil,
			mockFunc: func() {
				pancliMock.EXPECT().DeleteVolume(validVolumeName, defaultSecrets).Return(pancli.ErrorNotFound)
			},
		},
		{
			name: "PancliDeleteVolumeError",
			req: &csi.DeleteVolumeRequest{
				VolumeId: validVolumeName,
				Secrets:  defaultSecrets,
			},
			expectedResponse: nil,
			expectedError:    status.Error(codes.Internal, UnexpectedErrorInternalStr),
			mockFunc: func() {
				pancliMock.EXPECT().DeleteVolume(validVolumeName, defaultSecrets).Return(pancli.ErrorInternal)
			},
		},
		{
			name: "EmptyVolumeIdError",
			req: &csi.DeleteVolumeRequest{
				VolumeId: emptyVolumeName,
				Secrets:  defaultSecrets,
			},
			expectedResponse: nil,
			expectedError:    status.Error(codes.InvalidArgument, "volume id must be provided"),
			mockFunc: func() {
				pancliMock.EXPECT().DeleteVolume(gomock.Any(), gomock.Any()).Times(0)
			},
		},
		{
			name: "EmptySecretsError",
			req: &csi.DeleteVolumeRequest{
				VolumeId: validVolumeName,
				Secrets:  nil,
			},
			expectedResponse: nil,
			expectedError:    status.Error(codes.InvalidArgument, "secrets must be provided"),
			mockFunc: func() {
				pancliMock.EXPECT().DeleteVolume(gomock.Any(), gomock.Any()).Times(0)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.mockFunc != nil {
				tc.mockFunc()
			}
			response, err := driver.DeleteVolume(t.Context(), tc.req)
			assert.Equal(t, tc.expectedResponse, response)
			assert.ErrorIs(t, err, tc.expectedError)
		})
	}
}

func TestUnimplementedControllerMethods(t *testing.T) {
	driver := &Driver{
		Version:  "testing",
		Name:     DefaultDriverName,
		endpoint: "unix:///tmp/csi.sock",
		host:     "localhost",
		panfs:    nil,
	}

	t.Run("ControllerPublishVolume_Unimplemented", func(t *testing.T) {
		resp, err := driver.ControllerPublishVolume(t.Context(), &csi.ControllerPublishVolumeRequest{})
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, status.Error(codes.Unimplemented, ""))
	})

	t.Run("ControllerUnpublishVolume_Unimplemented", func(t *testing.T) {
		resp, err := driver.ControllerUnpublishVolume(t.Context(), &csi.ControllerUnpublishVolumeRequest{})
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, status.Error(codes.Unimplemented, ""))
	})

	t.Run("ListVolumes_Unimplemented", func(t *testing.T) {
		resp, err := driver.ListVolumes(t.Context(), &csi.ListVolumesRequest{})
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, status.Error(codes.Unimplemented, ""))
	})

	t.Run("ControllerGetVolume_Unimplemented", func(t *testing.T) {
		resp, err := driver.ControllerGetVolume(t.Context(), &csi.ControllerGetVolumeRequest{})
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, status.Error(codes.Unimplemented, ""))
	})

	t.Run("GetCapacity_Unimplemented", func(t *testing.T) {
		resp, err := driver.GetCapacity(t.Context(), &csi.GetCapacityRequest{})
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, status.Error(codes.Unimplemented, ""))
	})

	t.Run("CreateSnapshot_Unimplemented", func(t *testing.T) {
		resp, err := driver.CreateSnapshot(t.Context(), &csi.CreateSnapshotRequest{})
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, status.Error(codes.Unimplemented, ""))
	})

	t.Run("DeleteSnapshot_Unimplemented", func(t *testing.T) {
		resp, err := driver.DeleteSnapshot(t.Context(), &csi.DeleteSnapshotRequest{})
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, status.Error(codes.Unimplemented, ""))
	})

	t.Run("ListSnapshots_Unimplemented", func(t *testing.T) {
		resp, err := driver.ListSnapshots(t.Context(), &csi.ListSnapshotsRequest{})
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, status.Error(codes.Unimplemented, ""))
	})
}

// TestControllerGetCapabilities tests the ControllerGetCapabilities method of the Driver struct.
func TestControllerGetCapabilities(t *testing.T) {
	driver := &Driver{
		Version:  "testing",
		Name:     DefaultDriverName,
		endpoint: "unix:///tmp/csi.sock",
		host:     "localhost",
		panfs:    nil,
	}

	expectedCapabilities := []*csi.ControllerServiceCapability{
		{
			Type: &csi.ControllerServiceCapability_Rpc{
				Rpc: &csi.ControllerServiceCapability_RPC{
					Type: csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
				},
			},
		},
		{
			Type: &csi.ControllerServiceCapability_Rpc{
				Rpc: &csi.ControllerServiceCapability_RPC{
					Type: csi.ControllerServiceCapability_RPC_EXPAND_VOLUME,
				},
			},
		},
		{
			Type: &csi.ControllerServiceCapability_Rpc{
				Rpc: &csi.ControllerServiceCapability_RPC{
					Type: csi.ControllerServiceCapability_RPC_SINGLE_NODE_MULTI_WRITER,
				},
			},
		},
	}

	resp, err := driver.ControllerGetCapabilities(t.Context(), &csi.ControllerGetCapabilitiesRequest{})
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, expectedCapabilities, resp.Capabilities)
}

// TestValidateVolumeCapabilities tests the ValidateVolumeCapabilities method of the Driver struct.
func TestValidateVolumeCapabilities(t *testing.T) {
	ctrl := gomock.NewController(t)
	pancliMock := mock.NewMockStorageProviderClient(ctrl)
	driver := &Driver{
		Version:  "testing",
		Name:     DefaultDriverName,
		endpoint: "unix:///tmp/csi.sock",
		host:     "localhost",
		panfs:    pancliMock,
	}

	mountCap := &csi.VolumeCapability{
		AccessType: &csi.VolumeCapability_Mount{
			Mount: &csi.VolumeCapability_MountVolume{},
		},
	}
	blockCap := &csi.VolumeCapability{
		AccessType: &csi.VolumeCapability_Block{
			Block: &csi.VolumeCapability_BlockVolume{},
		},
	}

	testCases := []struct {
		name             string
		req              *csi.ValidateVolumeCapabilitiesRequest
		expectedResponse *csi.ValidateVolumeCapabilitiesResponse
		expectedError    error
		mockFunc         func()
	}{
		{
			name: "Success",
			req: &csi.ValidateVolumeCapabilitiesRequest{
				VolumeId:           validVolumeName,
				VolumeCapabilities: []*csi.VolumeCapability{mountCap},
				Secrets:            defaultSecrets,
			},
			expectedResponse: &csi.ValidateVolumeCapabilitiesResponse{
				Confirmed: &csi.ValidateVolumeCapabilitiesResponse_Confirmed{
					VolumeCapabilities: []*csi.VolumeCapability{mountCap},
				},
			},
			expectedError: nil,
			mockFunc: func() {
				pancliMock.EXPECT().GetVolume(validVolumeName, defaultSecrets).Return(&utils.Volume{Name: utils.VolumeName(validVolumeName)}, nil)
			},
		},
		{
			name: "EmptyVolumeIdError",
			req: &csi.ValidateVolumeCapabilitiesRequest{
				VolumeId:           "",
				VolumeCapabilities: []*csi.VolumeCapability{mountCap},
				Secrets:            defaultSecrets,
			},
			expectedResponse: nil,
			expectedError:    status.Error(codes.InvalidArgument, "volume id must not be empty"),
			mockFunc: func() {
				pancliMock.EXPECT().GetVolume(gomock.Any(), gomock.Any()).Times(0)
			},
		},
		{
			name: "EmptyVolumeCapabilitiesError",
			req: &csi.ValidateVolumeCapabilitiesRequest{
				VolumeId:           validVolumeName,
				VolumeCapabilities: []*csi.VolumeCapability{},
				Secrets:            defaultSecrets,
			},
			expectedResponse: nil,
			expectedError:    status.Error(codes.InvalidArgument, "volume capabilities must be provided"),
			mockFunc: func() {
				pancliMock.EXPECT().GetVolume(gomock.Any(), gomock.Any()).Times(0)
			},
		},
		{
			name: "EmptySecretsError",
			req: &csi.ValidateVolumeCapabilitiesRequest{
				VolumeId:           validVolumeName,
				VolumeCapabilities: []*csi.VolumeCapability{mountCap},
				Secrets:            nil,
			},
			expectedResponse: nil,
			expectedError:    status.Error(codes.InvalidArgument, "secrets must be provided"),
			mockFunc: func() {
				pancliMock.EXPECT().GetVolume(gomock.Any(), gomock.Any()).Times(0)
			},
		},
		{
			name: "UnsupportedVolumeCapabilitiesError",
			req: &csi.ValidateVolumeCapabilitiesRequest{
				VolumeId:           validVolumeName,
				VolumeCapabilities: []*csi.VolumeCapability{blockCap},
				Secrets:            defaultSecrets,
			},
			expectedResponse: nil,
			expectedError:    status.Error(codes.InvalidArgument, VolumeCapabilitiesDoNotMatchErrorStr),
			mockFunc: func() {
				pancliMock.EXPECT().GetVolume(gomock.Any(), gomock.Any()).Times(0)
			},
		},
		{
			name: "VolumeNotFoundError",
			req: &csi.ValidateVolumeCapabilitiesRequest{
				VolumeId:           validVolumeName,
				VolumeCapabilities: []*csi.VolumeCapability{mountCap},
				Secrets:            defaultSecrets,
			},
			expectedResponse: nil,
			expectedError:    status.Error(codes.NotFound, VolumeNotFoundErrorStr),
			mockFunc: func() {
				pancliMock.EXPECT().GetVolume(validVolumeName, defaultSecrets).Return(nil, pancli.ErrorNotFound)
			},
		},
		{
			name: "InternalErrorFromPancli",
			req: &csi.ValidateVolumeCapabilitiesRequest{
				VolumeId:           validVolumeName,
				VolumeCapabilities: []*csi.VolumeCapability{mountCap},
				Secrets:            defaultSecrets,
			},
			expectedResponse: nil,
			expectedError:    status.Error(codes.Internal, pancli.ErrorInternal.Error()),
			mockFunc: func() {
				pancliMock.EXPECT().GetVolume(validVolumeName, defaultSecrets).Return(nil, pancli.ErrorInternal)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.mockFunc != nil {
				tc.mockFunc()
			}
			response, err := driver.ValidateVolumeCapabilities(t.Context(), tc.req)
			assert.Equal(t, tc.expectedResponse, response)
			assert.ErrorIs(t, err, tc.expectedError)
		})
	}
}
