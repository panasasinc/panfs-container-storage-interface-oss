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
	"github.com/panasasinc/panfs-container-storage-interface-oss/pkg/panapi"
	"github.com/panasasinc/panfs-container-storage-interface-oss/pkg/utils"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	defaultSecrets = map[string]string{
		utils.RealmConnectionContext.Token:          "token",
		utils.RealmConnectionContext.API:            "realm",
		utils.RealmConnectionContext.KMIPConfigData: "# some data",
	}
	validVolumeName = "validVolumeName"
	// validVolumeID is the CSI volume ID of validVolumeName: its PanAPI volume id in decimal form.
	validVolumeID       = "7"
	validVolumeIDNumber = int64(7)
	emptyVolumeName     = ""
	GB10Bytes           = utils.GBToBytes(10)
)

// TestControllerExpandVolume tests the ControllerExpandVolume method of the Driver struct.
func TestControllerExpandVolume(t *testing.T) {
	ctrl := gomock.NewController(t)
	panapiMock := mock.NewMockStorageProviderClient(ctrl)
	driver := &Driver{
		Version:  "testing",
		Name:     DefaultDriverName,
		endpoint: "unix:///tmp/csi.sock",
		host:     "localhost",
		panfs:    panapiMock,
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
				VolumeId:      validVolumeID,
				CapacityRange: &csi.CapacityRange{RequiredBytes: GB10Bytes},
				Secrets:       defaultSecrets,
			},
			&csi.ControllerExpandVolumeResponse{
				CapacityBytes:         GB10Bytes,
				NodeExpansionRequired: false,
			},
			nil,
			func() {
				panapiMock.EXPECT().ExpandVolume(gomock.Any(), validVolumeID, GB10Bytes, defaultSecrets).Return(nil)
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
				panapiMock.EXPECT().ExpandVolume(gomock.Any(), gomock.Any(), gomock.Any(), defaultSecrets).Times(0)
			},
		},
		{
			"EmptyCapacityRangeError",
			&csi.ControllerExpandVolumeRequest{
				VolumeId:      validVolumeID,
				CapacityRange: nil,
				Secrets:       defaultSecrets,
			},
			nil,
			status.Error(codes.InvalidArgument, "volume capacity range must be provided"),
			func() {
				panapiMock.EXPECT().ExpandVolume(gomock.Any(), gomock.Any(), gomock.Any(), defaultSecrets).Times(0)
			},
		},
		{
			"EmptySecretsError",
			&csi.ControllerExpandVolumeRequest{
				VolumeId:      validVolumeID,
				CapacityRange: &csi.CapacityRange{RequiredBytes: GB10Bytes},
				Secrets:       nil,
			},
			nil,
			status.Error(codes.InvalidArgument, "secrets must be provided"),
			func() {
				panapiMock.EXPECT().ExpandVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
			},
		},
		{
			"ExpandNonExistingVolumeError",
			&csi.ControllerExpandVolumeRequest{
				VolumeId:      validVolumeID,
				CapacityRange: &csi.CapacityRange{RequiredBytes: GB10Bytes},
				Secrets:       defaultSecrets,
			},
			nil,
			status.Error(codes.NotFound, VolumeNotFoundErrorStr),
			func() {
				panapiMock.EXPECT().ExpandVolume(gomock.Any(), validVolumeID, GB10Bytes, defaultSecrets).Return(panapi.ErrorNotFound)
			},
		},
		{
			"ExpandFailedPancliError",
			&csi.ControllerExpandVolumeRequest{
				VolumeId:      validVolumeID,
				CapacityRange: &csi.CapacityRange{RequiredBytes: GB10Bytes},
				Secrets:       defaultSecrets,
			},
			nil,
			status.Error(codes.Internal, UnexpectedErrorInternalStr),
			func() {
				panapiMock.EXPECT().ExpandVolume(gomock.Any(), validVolumeID, GB10Bytes, defaultSecrets).Return(panapi.ErrorInternal)
			},
		},
		{
			"RequiredLessThan0",
			&csi.ControllerExpandVolumeRequest{
				VolumeId:      validVolumeID,
				CapacityRange: &csi.CapacityRange{RequiredBytes: -100},
				Secrets:       defaultSecrets,
			},
			nil,
			status.Error(codes.InvalidArgument, InvalidCapacityRangeErrorStr),
			func() {
				panapiMock.EXPECT().ExpandVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
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

// mountCapabilities is the volume capability list of a plain mount volume.
func mountCapabilities() []*csi.VolumeCapability {
	return []*csi.VolumeCapability{
		{AccessType: &csi.VolumeCapability_Mount{Mount: &csi.VolumeCapability_MountVolume{}}},
	}
}

// testVolume returns a PanAPI volume named validVolumeName with the given quotas.
func testVolume(soft, hard int64) *panapi.Volume {
	return &panapi.Volume{
		ID:             validVolumeIDNumber,
		Pathname:       "/" + validVolumeName,
		SoftQuotaBytes: &soft,
		HardQuotaBytes: &hard,
	}
}

// TestControllerCreateVolume tests the CreateVolume method of the Driver struct.
func TestControllerCreateVolume(t *testing.T) {
	ctrl := gomock.NewController(t)
	panapiMock := mock.NewMockStorageProviderClient(ctrl)
	driver := &Driver{
		Version:  "testing",
		Name:     DefaultDriverName,
		endpoint: "unix:///tmp/csi.sock",
		host:     "localhost",
		panfs:    panapiMock,
	}

	const GB9Bytes, GB20Bytes = int64(9) << 30, int64(20) << 30

	request := func(mutate func(*csi.CreateVolumeRequest)) *csi.CreateVolumeRequest {
		req := &csi.CreateVolumeRequest{
			Name:               validVolumeName,
			CapacityRange:      &csi.CapacityRange{RequiredBytes: GB10Bytes},
			Parameters:         map[string]string{panapi.StoragesetIDParameter: "1"},
			Secrets:            defaultSecrets,
			VolumeCapabilities: mountCapabilities(),
		}
		if mutate != nil {
			mutate(req)
		}
		return req
	}
	response := func(capacity int64) *csi.CreateVolumeResponse {
		return &csi.CreateVolumeResponse{
			Volume: &csi.Volume{
				VolumeId:      validVolumeID,
				CapacityBytes: capacity,
				VolumeContext: map[string]string{utils.VolumeNameContextKey: validVolumeName},
			},
		}
	}
	lookup := func(vol *panapi.Volume, err error) *gomock.Call {
		return panapiMock.EXPECT().GetVolumeByName(gomock.Any(), validVolumeName, defaultSecrets).Times(1).Return(vol, err)
	}
	create := func(soft, hard int64, vol *panapi.Volume, err error) *gomock.Call {
		return panapiMock.EXPECT().CreateVolume(gomock.Any(), panapi.CreateVolumeParams{
			Name:           validVolumeName,
			Parameters:     map[string]string{panapi.StoragesetIDParameter: "1"},
			SoftQuotaBytes: soft,
			HardQuotaBytes: hard,
		}, defaultSecrets).Times(1).Return(vol, err)
	}
	noBackendCalls := func() {
		panapiMock.EXPECT().GetVolumeByName(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
		panapiMock.EXPECT().CreateVolume(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
	}
	invalidParameter := fmt.Errorf("%w: parameter user: not numeric", panapi.ErrorInvalidArgument)

	testCases := []struct {
		name             string
		req              *csi.CreateVolumeRequest
		expectedResponse *csi.CreateVolumeResponse
		expectedError    error
		mockFunc         func()
	}{
		{
			"CreateVolumeSuccessHardQuotaEqualsSize",
			request(nil),
			response(GB10Bytes),
			nil,
			func() {
				gomock.InOrder(
					lookup(nil, panapi.ErrorNotFound),
					create(GB10Bytes, GB10Bytes, testVolume(GB10Bytes, GB10Bytes), nil),
				)
			},
		},
		{
			"CreateVolumeExplicitLimitIsHardQuota",
			request(func(r *csi.CreateVolumeRequest) {
				r.CapacityRange = &csi.CapacityRange{RequiredBytes: GB10Bytes, LimitBytes: GB20Bytes}
			}),
			response(GB10Bytes),
			nil,
			func() {
				gomock.InOrder(
					lookup(nil, panapi.ErrorNotFound),
					create(GB10Bytes, GB20Bytes, testVolume(GB10Bytes, GB20Bytes), nil),
				)
			},
		},
		{
			"VolumeExistsIsReturnedWithoutCreate",
			request(nil),
			response(GB10Bytes),
			nil,
			func() {
				lookup(testVolume(GB10Bytes, GB10Bytes), nil)
				panapiMock.EXPECT().CreateVolume(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
			},
		},
		{
			"VolumeExistsCapacityDoesNotMatchError",
			request(nil),
			nil,
			status.Error(codes.AlreadyExists, "Volume capacity does not match: requiredBytes bytes (10737418240) exceeds soft quota bytes (9663676416)"),
			func() {
				lookup(testVolume(GB9Bytes, GB9Bytes), nil)
			},
		},
		{
			"ConflictOnCreateThenVolumeFound",
			request(nil),
			response(GB10Bytes),
			nil,
			func() {
				gomock.InOrder(
					lookup(nil, panapi.ErrorNotFound),
					create(GB10Bytes, GB10Bytes, nil, panapi.ErrorAlreadyExist),
					lookup(testVolume(GB10Bytes, GB10Bytes), nil),
				)
			},
		},
		{
			"ConflictOnCreateButNoSuchVolumeError",
			request(nil),
			nil,
			status.Error(codes.Internal, UnexpectedErrorInternalStr),
			func() {
				gomock.InOrder(
					lookup(nil, panapi.ErrorNotFound),
					create(GB10Bytes, GB10Bytes, nil, panapi.ErrorAlreadyExist),
					lookup(nil, panapi.ErrorNotFound),
				)
			},
		},
		{
			"LookupUnavailableError",
			request(nil),
			nil,
			status.Error(codes.Unavailable, UnexpectedErrorInternalStr),
			func() {
				lookup(nil, panapi.ErrorUnavailable)
			},
		},
		{
			"CreateUnauthenticatedError",
			request(nil),
			nil,
			status.Error(codes.Unauthenticated, UnexpectedErrorInternalStr),
			func() {
				gomock.InOrder(
					lookup(nil, panapi.ErrorNotFound),
					create(GB10Bytes, GB10Bytes, nil, panapi.ErrorUnauthenticated),
				)
			},
		},
		{
			"CreateInvalidArgumentKeepsMessage",
			request(nil),
			nil,
			status.Error(codes.InvalidArgument, invalidParameter.Error()),
			func() {
				gomock.InOrder(
					lookup(nil, panapi.ErrorNotFound),
					create(GB10Bytes, GB10Bytes, nil, invalidParameter),
				)
			},
		},
		{
			"CreateInternalError",
			request(nil),
			nil,
			status.Error(codes.Internal, UnexpectedErrorInternalStr),
			func() {
				gomock.InOrder(
					lookup(nil, panapi.ErrorNotFound),
					create(GB10Bytes, GB10Bytes, nil, panapi.ErrorInternal),
				)
			},
		},
		{
			"UnsupportedVolumeCapabilitiesError",
			request(func(r *csi.CreateVolumeRequest) {
				r.VolumeCapabilities = []*csi.VolumeCapability{
					{AccessType: &csi.VolumeCapability_Block{Block: &csi.VolumeCapability_BlockVolume{}}},
				}
			}),
			nil,
			status.Error(
				codes.InvalidArgument,
				fmt.Sprintf(
					"unsupported volume capability: %s",
					&csi.VolumeCapability{AccessType: &csi.VolumeCapability_Block{Block: &csi.VolumeCapability_BlockVolume{}}})),
			noBackendCalls,
		},
		{
			"EmptySecretsError",
			request(func(r *csi.CreateVolumeRequest) { r.Secrets = nil }),
			nil,
			status.Error(codes.InvalidArgument, "secrets must be provided"),
			noBackendCalls,
		},
		{
			"MissingTokenError",
			request(func(r *csi.CreateVolumeRequest) {
				r.Secrets = map[string]string{utils.RealmConnectionContext.API: "realm"}
			}),
			nil,
			status.Error(codes.InvalidArgument, "missing token in secrets"),
			noBackendCalls,
		},
		{
			"SSHCredentialsInSecretsError",
			request(func(r *csi.CreateVolumeRequest) {
				r.Secrets = map[string]string{
					utils.RealmConnectionContext.API:      "realm",
					utils.RealmConnectionContext.Token:    "token",
					utils.RealmConnectionContext.Password: "pass",
				}
			}),
			nil,
			status.Error(codes.InvalidArgument,
				"password in secrets is not supported: the driver uses a PanAPI token (token), not SSH credentials"),
			noBackendCalls,
		},
		{
			"CreateVolumeInvalidVolumeNameError",
			request(func(r *csi.CreateVolumeRequest) { r.Name = emptyVolumeName }),
			nil,
			status.Error(codes.InvalidArgument, "name must be provided"),
			noBackendCalls,
		},
		{
			"CreateVolumeInvalidCapacityRangeError",
			request(func(r *csi.CreateVolumeRequest) { r.CapacityRange = &csi.CapacityRange{RequiredBytes: -100} }),
			nil,
			status.Error(codes.InvalidArgument, fmt.Sprintf("required_bytes (%d) cannot be less than zero", -100)),
			noBackendCalls,
		},
		{
			"CreateVolumeInvalidVolumeCapabilitiesError",
			request(func(r *csi.CreateVolumeRequest) { r.VolumeCapabilities = []*csi.VolumeCapability{} }),
			nil,
			status.Error(codes.InvalidArgument, "volume_capabilities must be provided"),
			noBackendCalls,
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
	panapiMock := mock.NewMockStorageProviderClient(ctrl)
	driver := &Driver{
		Version:  "testing",
		Name:     DefaultDriverName,
		endpoint: "unix:///tmp/csi.sock",
		host:     "localhost",
		panfs:    panapiMock,
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
				VolumeId: validVolumeID,
				Secrets:  defaultSecrets,
			},
			expectedResponse: &csi.DeleteVolumeResponse{},
			expectedError:    nil,
			mockFunc: func() {
				panapiMock.EXPECT().DeleteVolume(gomock.Any(), validVolumeID, defaultSecrets).Return(nil)
			},
		},
		{
			name: "VolumeNotFoundReturnsOK",
			req: &csi.DeleteVolumeRequest{
				VolumeId: validVolumeID,
				Secrets:  defaultSecrets,
			},
			expectedResponse: &csi.DeleteVolumeResponse{},
			expectedError:    nil,
			mockFunc: func() {
				panapiMock.EXPECT().DeleteVolume(gomock.Any(), validVolumeID, defaultSecrets).Return(panapi.ErrorNotFound)
			},
		},
		{
			name: "PancliDeleteVolumeError",
			req: &csi.DeleteVolumeRequest{
				VolumeId: validVolumeID,
				Secrets:  defaultSecrets,
			},
			expectedResponse: nil,
			expectedError:    status.Error(codes.Internal, UnexpectedErrorInternalStr),
			mockFunc: func() {
				panapiMock.EXPECT().DeleteVolume(gomock.Any(), validVolumeID, defaultSecrets).Return(panapi.ErrorInternal)
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
				panapiMock.EXPECT().DeleteVolume(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
			},
		},
		{
			name: "EmptySecretsError",
			req: &csi.DeleteVolumeRequest{
				VolumeId: validVolumeID,
				Secrets:  nil,
			},
			expectedResponse: nil,
			expectedError:    status.Error(codes.InvalidArgument, "secrets must be provided"),
			mockFunc: func() {
				panapiMock.EXPECT().DeleteVolume(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
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
	panapiMock := mock.NewMockStorageProviderClient(ctrl)
	driver := &Driver{
		Version:  "testing",
		Name:     DefaultDriverName,
		endpoint: "unix:///tmp/csi.sock",
		host:     "localhost",
		panfs:    panapiMock,
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
				VolumeId:           validVolumeID,
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
				panapiMock.EXPECT().GetVolume(gomock.Any(), validVolumeID, defaultSecrets).Return(testVolume(GB10Bytes, GB10Bytes), nil)
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
				panapiMock.EXPECT().GetVolume(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
			},
		},
		{
			name: "EmptyVolumeCapabilitiesError",
			req: &csi.ValidateVolumeCapabilitiesRequest{
				VolumeId:           validVolumeID,
				VolumeCapabilities: []*csi.VolumeCapability{},
				Secrets:            defaultSecrets,
			},
			expectedResponse: nil,
			expectedError:    status.Error(codes.InvalidArgument, "volume capabilities must be provided"),
			mockFunc: func() {
				panapiMock.EXPECT().GetVolume(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
			},
		},
		{
			name: "EmptySecretsError",
			req: &csi.ValidateVolumeCapabilitiesRequest{
				VolumeId:           validVolumeID,
				VolumeCapabilities: []*csi.VolumeCapability{mountCap},
				Secrets:            nil,
			},
			expectedResponse: nil,
			expectedError:    status.Error(codes.InvalidArgument, "secrets must be provided"),
			mockFunc: func() {
				panapiMock.EXPECT().GetVolume(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
			},
		},
		{
			name: "UnsupportedVolumeCapabilitiesError",
			req: &csi.ValidateVolumeCapabilitiesRequest{
				VolumeId:           validVolumeID,
				VolumeCapabilities: []*csi.VolumeCapability{blockCap},
				Secrets:            defaultSecrets,
			},
			expectedResponse: nil,
			expectedError:    status.Error(codes.InvalidArgument, VolumeCapabilitiesDoNotMatchErrorStr),
			mockFunc: func() {
				panapiMock.EXPECT().GetVolume(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
			},
		},
		{
			name: "VolumeNotFoundError",
			req: &csi.ValidateVolumeCapabilitiesRequest{
				VolumeId:           validVolumeID,
				VolumeCapabilities: []*csi.VolumeCapability{mountCap},
				Secrets:            defaultSecrets,
			},
			expectedResponse: nil,
			expectedError:    status.Error(codes.NotFound, VolumeNotFoundErrorStr),
			mockFunc: func() {
				panapiMock.EXPECT().GetVolume(gomock.Any(), validVolumeID, defaultSecrets).Return(nil, panapi.ErrorNotFound)
			},
		},
		{
			name: "InternalErrorFromPancli",
			req: &csi.ValidateVolumeCapabilitiesRequest{
				VolumeId:           validVolumeID,
				VolumeCapabilities: []*csi.VolumeCapability{mountCap},
				Secrets:            defaultSecrets,
			},
			expectedResponse: nil,
			expectedError:    status.Error(codes.Internal, UnexpectedErrorInternalStr),
			mockFunc: func() {
				panapiMock.EXPECT().GetVolume(gomock.Any(), validVolumeID, defaultSecrets).Return(nil, panapi.ErrorInternal)
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
