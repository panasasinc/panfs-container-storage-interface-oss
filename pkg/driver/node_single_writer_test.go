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
	"errors"
	"fmt"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/panasasinc/panfs-container-storage-interface-oss/pkg/driver/mock"
	"github.com/panasasinc/panfs-container-storage-interface-oss/pkg/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/mount-utils"
)

// publishRequestWithAccessMode builds a valid NodePublishVolume request with the given access mode.
func publishRequestWithAccessMode(mode csi.VolumeCapability_AccessMode_Mode, targetPath string) *csi.NodePublishVolumeRequest {
	return &csi.NodePublishVolumeRequest{
		VolumeId:   validVolumeName,
		TargetPath: targetPath,
		VolumeCapability: &csi.VolumeCapability{
			AccessType: &csi.VolumeCapability_Mount{
				Mount: &csi.VolumeCapability_MountVolume{MountFlags: []string{}},
			},
			AccessMode: &csi.VolumeCapability_AccessMode{Mode: mode},
		},
		Secrets:       defaultSecrets,
		VolumeContext: map[string]string{utils.VolumeNameContextKey: validVolumeName},
	}
}

// TestNodePublishVolumeSingleNodeSingleWriter tests that a SINGLE_NODE_SINGLE_WRITER volume
// can be published at one target path only.
func TestNodePublishVolumeSingleNodeSingleWriter(t *testing.T) {
	source := fmt.Sprintf("panfs://%s/%s", defaultSecrets[utils.RealmConnectionContext.API], validVolumeName)
	otherTargetPath := "/tmp/publish/other_path"
	singleWriter := csi.VolumeCapability_AccessMode_SINGLE_NODE_SINGLE_WRITER

	newDriver := func(t *testing.T) (*Driver, *mock.MockPanMounter) {
		mockMounter := mock.NewMockPanMounter(gomock.NewController(t))
		return &Driver{
			Version:   "testing",
			Name:      DefaultDriverName,
			endpoint:  "unix:///tmp/csi.sock",
			host:      "localhost",
			mounterV2: mockMounter,
		}, mockMounter
	}

	t.Run("published when the volume is not mounted", func(t *testing.T) {
		driver, mockMounter := newDriver(t)
		mockMounter.EXPECT().MountedTargets(source).Return(nil, nil).Times(1)
		mockMounter.EXPECT().Mount(source, validPublishTargetPath, []string{}).Return(nil).Times(1)

		resp, err := driver.NodePublishVolume(t.Context(), publishRequestWithAccessMode(singleWriter, validPublishTargetPath))
		require.NoError(t, err)
		assert.Equal(t, &csi.NodePublishVolumeResponse{}, resp)
	})

	t.Run("idempotent when the volume is mounted at the same target path", func(t *testing.T) {
		driver, mockMounter := newDriver(t)
		mockMounter.EXPECT().MountedTargets(source).Return([]string{validPublishTargetPath + "/"}, nil).Times(1)
		mockMounter.EXPECT().Mount(source, validPublishTargetPath, []string{}).Return(nil).Times(1)

		resp, err := driver.NodePublishVolume(t.Context(), publishRequestWithAccessMode(singleWriter, validPublishTargetPath))
		require.NoError(t, err)
		assert.Equal(t, &csi.NodePublishVolumeResponse{}, resp)
	})

	t.Run("rejected when the volume is mounted at another target path", func(t *testing.T) {
		driver, mockMounter := newDriver(t)
		mockMounter.EXPECT().MountedTargets(source).Return([]string{validPublishTargetPath}, nil).Times(1)
		mockMounter.EXPECT().Mount(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

		resp, err := driver.NodePublishVolume(t.Context(), publishRequestWithAccessMode(singleWriter, otherTargetPath))
		assert.Nil(t, resp)
		assert.Equal(t, codes.FailedPrecondition, status.Code(err))
	})

	t.Run("internal error when the mount table cannot be read", func(t *testing.T) {
		driver, mockMounter := newDriver(t)
		mockMounter.EXPECT().MountedTargets(source).Return(nil, errors.New("cannot read mounts")).Times(1)
		mockMounter.EXPECT().Mount(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

		resp, err := driver.NodePublishVolume(t.Context(), publishRequestWithAccessMode(singleWriter, validPublishTargetPath))
		assert.Nil(t, resp)
		assert.Equal(t, codes.Internal, status.Code(err))
	})

	t.Run("other access modes are not checked", func(t *testing.T) {
		for _, mode := range []csi.VolumeCapability_AccessMode_Mode{
			csi.VolumeCapability_AccessMode_SINGLE_NODE_MULTI_WRITER,
			csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
		} {
			driver, mockMounter := newDriver(t)
			mockMounter.EXPECT().MountedTargets(gomock.Any()).Times(0)
			mockMounter.EXPECT().Mount(source, otherTargetPath, []string{}).Return(nil).Times(1)

			_, err := driver.NodePublishVolume(t.Context(), publishRequestWithAccessMode(mode, otherTargetPath))
			require.NoError(t, err, "access mode %s", mode)
		}
	})
}

// TestMountedTargets tests that mountedTargets returns only the mount points of the given source.
func TestMountedTargets(t *testing.T) {
	fakeMounter := mount.NewFakeMounter([]mount.MountPoint{
		{Device: "panfs://realm/vol1", Path: "/mnt/a"},
		{Device: "panfs://realm/vol2", Path: "/mnt/b"},
		{Device: "panfs://realm/vol1", Path: "/mnt/c"},
	})

	targets, err := mountedTargets(fakeMounter, "panfs://realm/vol1")
	require.NoError(t, err)
	assert.Equal(t, []string{"/mnt/a", "/mnt/c"}, targets)

	targets, err = mountedTargets(fakeMounter, "panfs://realm/unknown")
	require.NoError(t, err)
	assert.Empty(t, targets)
}
