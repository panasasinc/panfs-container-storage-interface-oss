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
	"os"
	"regexp"
	"testing"

	"slices"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/panasasinc/panfs-container-storage-interface-oss/pkg/driver/mock"
	"github.com/panasasinc/panfs-container-storage-interface-oss/pkg/utils"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	emptyVolumeID          = ""
	validStagingPath       = "/tmp/stage/vol-123"
	validPublishTargetPath = "/tmp/publish/path"
	invalidStagingPath     = ""
	invalidPublishPath     = ""
)

// TODO: move this test to the mounter_test.go
// func TestNodeStageVolumeMountPointAlreadyExists(t *testing.T) {
// 	ctrl := gomock.NewController(t)
// 	mockMounter := mock.NewMockPanMounter(ctrl)
// 	// mounter := mount.NewFakeMounter(nil)
// 	driver := &Driver{
// 		version:  "testing",
// 		name:     DefaultDriverName,
// 		endpoint: "unix:///tmp/csi.sock",
// 		host:     "localhost",
// 		mounter:  mounter,
// 		mounterV2: mockMounter,
// 		panfs:    nil, // node service is not using PanFS so it's OK to pass nil
// 	}

// 	mounter.MountPoints = []mount.MountPoint{
// 		{
// 			Path: validStagingPath,
// 		},
// 	}

// 	req := &csi.NodeStageVolumeRequest{
// 		VolumeId:          validVolumeName,
// 		StagingTargetPath: validStagingPath,
// 		VolumeCapability: &csi.VolumeCapability{
// 			AccessType: &csi.VolumeCapability_Mount{
// 				Mount: &csi.VolumeCapability_MountVolume{},
// 			},
// 		},
// 		Secrets: defaultSecrets,
// 	}
// 	mockMounter.EXPECT()
// 	resp, err := driver.NodeStageVolume(t.Context(), req)
// 	assert.NoError(t, err)
// 	assert.NotNil(t, resp)

// 	assert.Len(t, mounter.MountPoints, 1)
// 	assert.Equal(t, validStagingPath, mounter.MountPoints[0].Path)
// }

// TestNodePublishVolume tests the NodePublishVolume method of the Driver.
// It covers scenarios for successful publish, error cases, unsupported capabilities, ephemeral volumes, and mount options.
func TestNodePublishVolume(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockMounter := mock.NewMockPanMounter(ctrl)
	driver := &Driver{
		Version:   "testing",
		Name:      DefaultDriverName,
		endpoint:  "unix:///tmp/csi.sock",
		host:      "localhost",
		mounterV2: mockMounter,
		panfs:     nil, // node service is not using PanFS so it's OK to pass nil
	}

	bindMountCalledZeroTimes := func() {
		mockMounter.EXPECT().Mount(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
	}

	testCases := []struct {
		name          string
		req           *csi.NodePublishVolumeRequest
		expectedResp  *csi.NodePublishVolumeResponse
		expectedError error
		mockFunc      func()
	}{
		{
			"Successfully published",
			&csi.NodePublishVolumeRequest{
				VolumeId:          validVolumeName,
				StagingTargetPath: "",
				TargetPath:        validPublishTargetPath,
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{
							MountFlags: []string{},
						},
					},
				},
				Secrets: defaultSecrets,
			},
			&csi.NodePublishVolumeResponse{},
			nil,
			func() {
				mockMounter.EXPECT().Mount(
					fmt.Sprintf("panfs://%s/%s", defaultSecrets[utils.RealmConnectionContext.RealmAddress], validVolumeName),
					validPublishTargetPath,
					[]string{}).Times(1)
			},
		},
		{
			"Empty volume id",
			&csi.NodePublishVolumeRequest{
				VolumeId:          emptyVolumeName,
				StagingTargetPath: validStagingPath,
				TargetPath:        validPublishTargetPath,
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{
							MountFlags: []string{},
						},
					},
				},
				Secrets: defaultSecrets,
			},
			nil,
			status.Error(codes.InvalidArgument, "Volume id must be provided"),
			bindMountCalledZeroTimes,
		},
		{
			"Publish failure",
			&csi.NodePublishVolumeRequest{
				VolumeId:          validVolumeName,
				StagingTargetPath: "",
				TargetPath:        validPublishTargetPath,
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{
							MountFlags: []string{"noatime"},
						},
					},
				},
				Secrets: defaultSecrets,
			},
			nil,
			status.Error(codes.Internal, "Failed to publish volume: mounter error"),
			func() {
				mockMounter.EXPECT().Mount(
					fmt.Sprintf("panfs://%s/%s", defaultSecrets[utils.RealmConnectionContext.RealmAddress], validVolumeName),
					validPublishTargetPath,
					[]string{"noatime"}).Return(fmt.Errorf("mounter error")).Times(1)
			},
		},
		{
			"Empty staging target path",
			&csi.NodePublishVolumeRequest{
				VolumeId:          validVolumeName,
				StagingTargetPath: invalidStagingPath,
				TargetPath:        validPublishTargetPath,
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{
							MountFlags: []string{},
						},
					},
				},
				Secrets: defaultSecrets,
			},
			&csi.NodePublishVolumeResponse{},
			nil,
			func() {
				mockMounter.EXPECT().Mount(
					fmt.Sprintf("panfs://%s/%s", defaultSecrets[utils.RealmConnectionContext.RealmAddress], validVolumeName),
					validPublishTargetPath,
					[]string{}).Times(1)
			},
		},
		{
			"Empty publish target path",
			&csi.NodePublishVolumeRequest{
				VolumeId:          validVolumeName,
				StagingTargetPath: validStagingPath,
				TargetPath:        invalidPublishPath,
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{
							MountFlags: []string{},
						},
					},
				},
				Secrets: defaultSecrets,
			},
			nil,
			status.Error(codes.InvalidArgument, "Target Path must be provided"),
			bindMountCalledZeroTimes,
		},
		{
			"Empty volume capability",
			&csi.NodePublishVolumeRequest{
				VolumeId:          validVolumeName,
				StagingTargetPath: validStagingPath,
				TargetPath:        validPublishTargetPath,
				VolumeCapability:  nil,
				Secrets:           defaultSecrets,
			},
			nil,
			status.Error(codes.InvalidArgument, "Volume Capability must be provided"),
			bindMountCalledZeroTimes,
		},
		{
			"Not supported volume capability: block",
			&csi.NodePublishVolumeRequest{
				VolumeId:          validVolumeName,
				StagingTargetPath: validStagingPath,
				TargetPath:        validPublishTargetPath,
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Block{
						Block: &csi.VolumeCapability_BlockVolume{},
					},
				},
				Secrets: defaultSecrets,
			},
			nil,
			status.Error(codes.FailedPrecondition, "unsupported volume capability provided"),
			bindMountCalledZeroTimes,
		},
		{
			"Ephemeral unsupported volume",
			&csi.NodePublishVolumeRequest{
				VolumeId:          validVolumeName,
				StagingTargetPath: validStagingPath,
				TargetPath:        validPublishTargetPath,
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{
							MountFlags: []string{},
						},
					},
				},
				Secrets: defaultSecrets,
				VolumeContext: map[string]string{
					EphemeralK8SVolumeContext: "true",
				},
			},
			nil,
			status.Error(codes.FailedPrecondition, "Ephemeral volumes are not supported by this driver"),
			bindMountCalledZeroTimes,
		},
		{
			"Mount options with read-only flag",
			&csi.NodePublishVolumeRequest{
				VolumeId:          validVolumeName,
				StagingTargetPath: validStagingPath,
				TargetPath:        validPublishTargetPath,
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{
							MountFlags: []string{"noatime"},
						},
					},
				},
				Secrets:  defaultSecrets,
				Readonly: true,
			},
			&csi.NodePublishVolumeResponse{},
			nil,
			func() {
				mockMounter.EXPECT().Mount(
					fmt.Sprintf("panfs://%s/%s", defaultSecrets[utils.RealmConnectionContext.RealmAddress], validVolumeName),
					validPublishTargetPath,
					[]string{"noatime", "ro"}).Times(1)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.mockFunc()
			resp, err := driver.NodePublishVolume(t.Context(), tc.req)
			assert.Equal(t, tc.expectedResp, resp, "Unexpected response got from NodePublishVolume: %v, expected: %v", resp, tc.expectedResp)
			assert.Equal(t, tc.expectedError, err, "Unexpected error got from NodePublishVolume: %v, expected: %v", err, tc.expectedError)
		})
	}
}

// fakeFileWriter is a mock implementation of utils.FileWriter for testing
type fakeFileWriter struct {
	writeCalled bool
	writeData   []byte
	writeErr    error
	closeCalled bool
	closeErr    error
	name        string
}

// Write simulates writing data to the file
func (f *fakeFileWriter) Write(b []byte) (int, error) {
	f.writeCalled = true
	f.writeData = b
	return len(b), f.writeErr
}

// Close simulates closing the file
func (f *fakeFileWriter) Close() error {
	f.closeCalled = true
	return f.closeErr
}

// Name returns the name of the file
func (f *fakeFileWriter) Name() string {
	return f.name
}

// fakeTempFileFactory is a mock implementation of a temp file factory for testing
type fakeTempFileFactory struct {
	file FileWriter
}

// CreateTemp simulates creating a temporary file
func (f *fakeTempFileFactory) CreateTemp(dir, pattern string) (FileWriter, error) {
	return f.file, nil
}

// osCreateTemp is a wrapper around os.CreateTemp to match the utils.FileWriter interface
var osCreateTemp = func(dir, pattern string) (FileWriter, error) {
	f, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return nil, err
	}
	return &osFileWrapper{f}, nil
}

// mountOptsRegexpMatcher is a custom matcher for mount options using regular expressions
type mountOptsRegexpMatcher struct {
	pattern *regexp.Regexp
}

// Matches checks if the provided mount options match the regular expression
func (m mountOptsRegexpMatcher) Matches(x interface{}) bool {
	opts, ok := x.([]string)
	if !ok || len(opts) == 0 {
		return false
	}
	return slices.ContainsFunc(opts, m.pattern.MatchString)
}

// String returns a description of the matcher
func (m mountOptsRegexpMatcher) String() string {
	return "matches mount options regexp"
}

// errorTempFileFactory simulates an error during temp file creation
type errorTempFileFactory struct{}

// CreateTemp always returns an error
func (f *errorTempFileFactory) CreateTemp(dir, pattern string) (FileWriter, error) {
	return nil, fmt.Errorf("create temp error")
}

// TestNodePublishVolume_EncryptedVolume tests the NodePublishVolume method for encrypted volumes,
// specifically focusing on KMIP configuration file handling and error scenarios.
func TestNodePublishVolume_EncryptedVolume(t *testing.T) {
	t.Run("KMIP config file creation fails", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockMounter := mock.NewMockPanMounter(ctrl)

		driver := &Driver{
			Version:         "testing",
			Name:            DefaultDriverName,
			endpoint:        "unix:///tmp/csi.sock",
			host:            "localhost",
			mounterV2:       mockMounter, // will be set in sub-tests
			panfs:           nil,
			tempFileFactory: &errorTempFileFactory{},
		}

		mockMounter.EXPECT().Mount(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

		// Save original functions to restore after test
		origMkdirAll := osMkdirAll
		origCreateTemp := osCreateTemp
		origChmod := osChmod
		origRemove := osRemove

		defer func() {
			osMkdirAll = origMkdirAll
			osCreateTemp = origCreateTemp
			osChmod = origChmod
			osRemove = origRemove
		}()

		osMkdirAll = func(path string, perm os.FileMode) error { return nil }
		osCreateTemp = func(dir, pattern string) (FileWriter, error) { return nil, fmt.Errorf("create temp error") }
		osChmod = func(name string, mode os.FileMode) error { return nil }
		osRemove = func(name string) error { return nil }

		req := &csi.NodePublishVolumeRequest{
			VolumeId:   validVolumeName,
			TargetPath: validPublishTargetPath,
			VolumeCapability: &csi.VolumeCapability{
				AccessType: &csi.VolumeCapability_Mount{
					Mount: &csi.VolumeCapability_MountVolume{},
				},
			},
			Secrets: map[string]string{
				utils.RealmConnectionContext.RealmAddress:   "realm",
				utils.RealmConnectionContext.Username:       "user",
				utils.RealmConnectionContext.Password:       "password",
				utils.RealmConnectionContext.KMIPConfigData: "some-kmip-data",
			},
			VolumeContext: map[string]string{
				utils.VolumeParameters.GetSCKey("encryption"): "on",
			},
		}
		resp, err := driver.NodePublishVolume(t.Context(), req)
		assert.Nil(t, resp)
		assert.EqualError(t, err, "rpc error: code = Internal desc = Failed to create KMIP config file: create temp error")
	})

	t.Run("KMIP config file chmod fails", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockMounter := mock.NewMockPanMounter(ctrl)

		driver := &Driver{
			Version:   "testing",
			Name:      DefaultDriverName,
			endpoint:  "unix:///tmp/csi.sock",
			host:      "localhost",
			mounterV2: mockMounter, // will be set in sub-tests
			panfs:     nil,
			tempFileFactory: &fakeTempFileFactory{
				file: &fakeFileWriter{
					name: "/var/tmp/kmip/config_test.conf",
				},
			},
		}

		// Mount should NOT be called if chmod fails
		mockMounter.EXPECT().Mount(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

		req := &csi.NodePublishVolumeRequest{
			VolumeId:   validVolumeName,
			TargetPath: validPublishTargetPath,
			VolumeCapability: &csi.VolumeCapability{
				AccessType: &csi.VolumeCapability_Mount{
					Mount: &csi.VolumeCapability_MountVolume{},
				},
			},
			Secrets: map[string]string{
				utils.RealmConnectionContext.RealmAddress:   "realm",
				utils.RealmConnectionContext.Username:       "user",
				utils.RealmConnectionContext.Password:       "password",
				utils.RealmConnectionContext.KMIPConfigData: "some-kmip-data",
			},
			VolumeContext: map[string]string{
				utils.VolumeParameters.GetSCKey("encryption"): "on",
			},
		}

		// Save and restore original osChmod
		origChmod := osChmod
		defer func() { osChmod = origChmod }()
		osChmod = func(name string, mode os.FileMode) error {
			return fmt.Errorf("chmod error")
		}

		resp, err := driver.NodePublishVolume(t.Context(), req)
		assert.Nil(t, resp)
		assert.EqualError(t, err, "rpc error: code = Internal desc = Failed to set '0700' permissions on KMIP config file: chmod error")
	})

	t.Run("Missing/Empty KMIP secret", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockMounter := mock.NewMockPanMounter(ctrl)

		driver := &Driver{
			Version:   "testing",
			Name:      DefaultDriverName,
			endpoint:  "unix:///tmp/csi.sock",
			host:      "localhost",
			mounterV2: mockMounter, // will be set in sub-tests
			panfs:     nil,
			tempFileFactory: &fakeTempFileFactory{
				file: &fakeFileWriter{
					name: "/var/tmp/kmip/config_test.conf",
				},
			},
		}

		// Mount should NOT be called if KMIP secret is missing
		mockMounter.EXPECT().Mount(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

		req := &csi.NodePublishVolumeRequest{
			VolumeId:   validVolumeName,
			TargetPath: validPublishTargetPath,
			VolumeCapability: &csi.VolumeCapability{
				AccessType: &csi.VolumeCapability_Mount{
					Mount: &csi.VolumeCapability_MountVolume{},
				},
			},
			Secrets: map[string]string{
				utils.RealmConnectionContext.RealmAddress:   "realm",
				utils.RealmConnectionContext.Username:       "user",
				utils.RealmConnectionContext.Password:       "password",
				utils.RealmConnectionContext.KMIPConfigData: "",
			},
			VolumeContext: map[string]string{
				utils.VolumeParameters.GetSCKey("encryption"): "on",
			},
		}

		// Save and restore original osChmod
		origChmod := osChmod
		defer func() { osChmod = origChmod }()
		osChmod = func(name string, mode os.FileMode) error { return nil }

		resp, err := driver.NodePublishVolume(t.Context(), req)
		assert.Nil(t, resp)
		assert.EqualError(t, err, "rpc error: code = InvalidArgument desc = KMIP secret must be provided for encrypted volumes")
	})

	t.Run("KMIP config file write fails", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockMounter := mock.NewMockPanMounter(ctrl)

		driver := &Driver{
			Version:   "testing",
			Name:      DefaultDriverName,
			endpoint:  "unix:///tmp/csi.sock",
			host:      "localhost",
			mounterV2: mockMounter,
			panfs:     nil,
			tempFileFactory: &fakeTempFileFactory{
				file: &fakeFileWriter{
					name:     "/var/tmp/kmip/config_test.conf",
					writeErr: fmt.Errorf("write error"),
				},
			},
		}

		// Mount should NOT be called if write fails
		mockMounter.EXPECT().Mount(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

		req := &csi.NodePublishVolumeRequest{
			VolumeId:   validVolumeName,
			TargetPath: validPublishTargetPath,
			VolumeCapability: &csi.VolumeCapability{
				AccessType: &csi.VolumeCapability_Mount{
					Mount: &csi.VolumeCapability_MountVolume{},
				},
			},
			Secrets: map[string]string{
				utils.RealmConnectionContext.RealmAddress:   "realm",
				utils.RealmConnectionContext.Username:       "user",
				utils.RealmConnectionContext.Password:       "password",
				utils.RealmConnectionContext.KMIPConfigData: "some data",
			},
			VolumeContext: map[string]string{
				utils.VolumeParameters.GetSCKey("encryption"): "on",
			},
		}

		// Save and restore original osChmod
		origChmod := osChmod
		defer func() { osChmod = origChmod }()
		osChmod = func(name string, mode os.FileMode) error { return nil }

		resp, err := driver.NodePublishVolume(t.Context(), req)
		assert.Nil(t, resp)
		assert.EqualError(t, err, "rpc error: code = Internal desc = Failed to write KMIP config data to temporary file: write error")
	})

	t.Run("Mount called with KMIP config file option", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockMounter := mock.NewMockPanMounter(ctrl)

		driver := &Driver{
			Version:   "testing",
			Name:      DefaultDriverName,
			endpoint:  "unix:///tmp/csi.sock",
			host:      "localhost",
			mounterV2: mockMounter,
			panfs:     nil,
			tempFileFactory: &fakeTempFileFactory{
				file: &fakeFileWriter{
					name: "/var/tmp/kmip/config_test.conf",
				},
			},
		}

		// Expect Mount to be called with the KMIP config file option
		mockMounter.EXPECT().Mount(
			"panfs://realm/validVolumeName",
			validPublishTargetPath,
			mountOptsRegexpMatcher{pattern: regexp.MustCompile(`kmip-config-file=/var/tmp/kmip/config_test.conf`)},
		).Return(nil).Times(1)

		req := &csi.NodePublishVolumeRequest{
			VolumeId:   validVolumeName,
			TargetPath: validPublishTargetPath,
			VolumeCapability: &csi.VolumeCapability{
				AccessType: &csi.VolumeCapability_Mount{
					Mount: &csi.VolumeCapability_MountVolume{},
				},
			},
			Secrets: map[string]string{
				utils.RealmConnectionContext.RealmAddress:   "realm",
				utils.RealmConnectionContext.Username:       "user",
				utils.RealmConnectionContext.Password:       "password",
				utils.RealmConnectionContext.KMIPConfigData: "some data",
			},
			VolumeContext: map[string]string{
				utils.VolumeParameters.GetSCKey("encryption"): "on",
			},
		}

		// Save and restore original osChmod
		origChmod := osChmod
		defer func() { osChmod = origChmod }()
		osChmod = func(name string, mode os.FileMode) error { return nil }

		resp, err := driver.NodePublishVolume(t.Context(), req)
		assert.NoError(t, err)
		assert.NotNil(t, resp)
	})
}

// TODO: move to the mounter
// func TestPublishVolumeAlreadyPublished(t *testing.T) {
// 	mounter := mount.NewFakeMounter(nil)
// 	driver := &Driver{
// 		version:  "testing",
// 		name:     DefaultDriverName,
// 		endpoint: "unix:///tmp/csi.sock",
// 		host:     "localhost",
// 		mounter:  mounter,
// 		panfs:    nil, // node service is not using PanFS so it's OK to pass nil
// 	}

// 	// precreate mount point
// 	mounter.MountPoints = append(mounter.MountPoints,
// 		mount.MountPoint{
// 			Device: "panfs://realm/volume",
// 			Path:   validPublishTargetPath,
// 			Type:   "panfs",
// 			Opts:   []string{"noatime"},
// 		})

// 	resp, err := driver.NodePublishVolume(t.Context(),
// 		&csi.NodePublishVolumeRequest{
// 			VolumeId:          validVolumeName,
// 			StagingTargetPath: validStagingPath,
// 			TargetPath:        validPublishTargetPath,
// 			VolumeCapability: &csi.VolumeCapability{
// 				AccessType: &csi.VolumeCapability_Mount{
// 					Mount: &csi.VolumeCapability_MountVolume{
// 						MountFlags: []string{},
// 					},
// 				},
// 			},
// 		})
// 	assert.NoError(t, err)
// 	expectedResp := &csi.NodePublishVolumeResponse{}
// 	assert.Equal(t, expectedResp, resp, "Expected response: %v, got: %v\n", expectedResp, resp)
// }

// TestUnpublishVolume tests the NodeUnpublishVolume method of the Driver.
// It covers scenarios for successful unpublish, error cases, and unpublish failures.
func TestUnpublishVolume(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockMounter := mock.NewMockPanMounter(ctrl)
	driver := &Driver{
		Version:   "testing",
		Name:      DefaultDriverName,
		endpoint:  "unix:///tmp/csi.sock",
		host:      "localhost",
		mounterV2: mockMounter,
		panfs:     nil, // node service is not using PanFS so it's OK to pass nil
	}

	testCases := []struct {
		name          string
		req           *csi.NodeUnpublishVolumeRequest
		expectedResp  *csi.NodeUnpublishVolumeResponse
		expectedError error
		mockFunc      func()
	}{
		{
			"Successfully unpublished",
			&csi.NodeUnpublishVolumeRequest{
				VolumeId:   validVolumeName,
				TargetPath: validPublishTargetPath,
			},
			&csi.NodeUnpublishVolumeResponse{},
			nil,
			func() {
				mockMounter.EXPECT().Unmount(validPublishTargetPath).Times(1)
			},
		},
		{
			"Empty volume id",
			&csi.NodeUnpublishVolumeRequest{
				VolumeId:   emptyVolumeName,
				TargetPath: validPublishTargetPath,
			},
			nil,
			status.Error(codes.InvalidArgument, "Volume id must be provided"),
			func() {
				mockMounter.EXPECT().Unmount(gomock.Any()).Times(0)
			},
		},
		{
			"Empty target path",
			&csi.NodeUnpublishVolumeRequest{
				VolumeId:   validVolumeName,
				TargetPath: invalidPublishPath,
			},
			nil,
			status.Error(codes.InvalidArgument, "Target Path must be provided"),
			func() {
				mockMounter.EXPECT().Unmount(gomock.Any()).Times(0)
			},
		},
		{
			"Unpublish failure",
			&csi.NodeUnpublishVolumeRequest{
				VolumeId:   validVolumeName,
				TargetPath: validPublishTargetPath,
			},
			nil,
			status.Error(codes.Internal, "Failed to unpublish volume: mounter error"),
			func() {
				mockMounter.EXPECT().Unmount(
					validPublishTargetPath).Return(fmt.Errorf("mounter error")).Times(1)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.mockFunc()
			resp, err := driver.NodeUnpublishVolume(t.Context(), tc.req)

			assert.Equal(t, tc.expectedResp, resp)
			assert.Equal(t, tc.expectedError, err)
		})
	}
}

// TODO: move to the mounter tests
// TestUnpublishVolumeAlreadyUnpublished is a placeholder for testing already unpublished volumes.
func TestUnpublishVolumeAlreadyUnpublished(t *testing.T) {
	// ctrl := gomock.NewController(t)
	// mockMounter := mock.NewMockPanMounter(ctrl)
}

// TestNodeUnimplementedMethods tests unimplemented node methods to ensure they return the correct error codes.
func TestNodeUnimplementedMethods(t *testing.T) {
	driver := &Driver{
		Version:   "testing",
		Name:      DefaultDriverName,
		endpoint:  "unix:///tmp/csi.sock",
		host:      "localhost",
		mounterV2: nil,
		panfs:     nil,
	}

	t.Run("NodeExpandVolume returns Unimplemented", func(t *testing.T) {
		resp, err := driver.NodeExpandVolume(t.Context(), &csi.NodeExpandVolumeRequest{})
		assert.Nil(t, resp)
		st, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.Unimplemented, st.Code())
	})

	t.Run("NodeGetVolumeStats returns Unimplemented", func(t *testing.T) {
		resp, err := driver.NodeGetVolumeStats(t.Context(), &csi.NodeGetVolumeStatsRequest{})
		assert.Nil(t, resp)
		st, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.Unimplemented, st.Code())
	})
}

// TestNodeGetCapabilities tests the NodeGetCapabilities method of the Driver.
// It verifies that the correct node service capability is returned.
func TestNodeGetCapabilities(t *testing.T) {
	driver := &Driver{
		Version:   "testing",
		Name:      DefaultDriverName,
		endpoint:  "unix:///tmp/csi.sock",
		host:      "localhost",
		mounterV2: nil,
		panfs:     nil,
	}

	t.Run("NodeGetCapabilities returns correct capability", func(t *testing.T) {
		resp, err := driver.NodeGetCapabilities(t.Context(), &csi.NodeGetCapabilitiesRequest{})
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Equal(t, &csi.NodeGetCapabilitiesResponse{
			Capabilities: []*csi.NodeServiceCapability{
				{
					Type: &csi.NodeServiceCapability_Rpc{
						Rpc: &csi.NodeServiceCapability_RPC{
							Type: csi.NodeServiceCapability_RPC_SINGLE_NODE_MULTI_WRITER,
						},
					},
				},
			},
		},
			resp)
	})
}

// TestNodeGetInfo tests the NodeGetInfo method of the Driver.
// It verifies that the correct node info is returned.
func TestNodeGetInfo(t *testing.T) {
	expectedNodeID := "test-node-id"
	driver := &Driver{
		Version:   "testing",
		Name:      DefaultDriverName,
		endpoint:  "unix:///tmp/csi.sock",
		host:      expectedNodeID,
		mounterV2: nil,
		panfs:     nil,
	}

	t.Run("NodeGetInfo returns correct node info", func(t *testing.T) {
		resp, err := driver.NodeGetInfo(t.Context(), &csi.NodeGetInfoRequest{})
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Equal(t, expectedNodeID, resp.NodeId)
		assert.Equal(t, int64(0), resp.MaxVolumesPerNode)
	})
}
