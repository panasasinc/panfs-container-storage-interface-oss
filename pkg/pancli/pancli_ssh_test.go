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

package pancli

import (
	"encoding/xml"
	"fmt"
	"testing"

	"github.com/panasasinc/panfs-container-storage-interface-oss/pkg/pancli/mock"
	"github.com/panasasinc/panfs-container-storage-interface-oss/pkg/utils"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

const (
	validVolumeName = "validVolumeName"
)

var (
	// Dummy secrets for testing only. Do not use real credentials.
	defaultSecrets = map[string]string{
		utils.RealmConnectionContext.Username:     "testuser",
		utils.RealmConnectionContext.Password:     "testpass",
		utils.RealmConnectionContext.RealmAddress: "testrealm",
	}

	// validVolumeResponse represents a valid volume response for testing
	validVolumeResponse = &utils.Volume{
		XMLName: xml.Name{Local: "volume"},
		Name:    validVolumeName,
		ID:      "371",
		State:   "Online",
		Soft:    0.00,
		Hard:    0.00,
		Bset: utils.Bladeset{
			XMLName: xml.Name{Local: "bladesetName"},
			ID:      "1",
			Name:    "Set 1",
		},
		Encryption: "off",
	}
)

func TestCreateVolume(t *testing.T) {
	ctrl := gomock.NewController(t)
	runnerMock := mock.NewMockSSHRunner(ctrl)

	testCases := []struct {
		name        string
		volName     string
		params      VolumeCreateParams
		expectedErr error
		response    *utils.Volume
		mockFunc    func()
	}{
		{
			"VolumeCreated",
			validVolumeName,
			VolumeCreateParams{
				utils.VolumeProvisioningContext.BladeSet.Key: "Set 1",
			},
			nil,
			validVolumeResponse,
			func() {
				// expect create volume command
				runnerMock.EXPECT().RunCommand(
					gomock.Any(),
					"volume", "create", validVolumeName, `bladeset "Set 1"`,
				).Times(1).Return([]byte{}, nil)

				genPasXML, _ := validVolumeResponse.MarshalVolumeToPasXML()
				// then get volume details
				runnerMock.EXPECT().RunCommand(
					gomock.Any(),
					"pasxml", "volumes", "volume", validVolumeName,
				).Times(1).Return(genPasXML, nil)
			},
		},
		{
			"CreateVolumeError",
			validVolumeName,
			VolumeCreateParams{},
			fmt.Errorf("create failed"),
			nil,
			func() {
				// expect create volume command to fail
				runnerMock.EXPECT().RunCommand(
					gomock.Any(),
					"volume", "create", validVolumeName,
				).Times(1).Return(nil, fmt.Errorf("create failed"))
				// no need to call get volume details
				runnerMock.EXPECT().RunCommand(
					gomock.Any(),
					gomock.Any(),
				).Times(0)
			},
		},
		{
			"VolumeAlreadyExists",
			validVolumeName,
			VolumeCreateParams{},
			fmt.Errorf("%w: %s", ErrorAlreadyExist, validVolumeName),
			nil,
			func() {
				// expect create volume command to fail with already exists error
				runnerMock.EXPECT().RunCommand(
					gomock.Any(),
					"volume", "create", validVolumeName,
				).Times(1).Return(nil, fmt.Errorf("%w: %s", ErrorAlreadyExist, validVolumeName))
				// no need to call get volume details
				runnerMock.EXPECT().RunCommand(
					gomock.Any(),
					gomock.Any(),
				).Times(0)
			},
		},
		{
			"CreatedButFailedToGetDetails",
			validVolumeName,
			VolumeCreateParams{
				utils.VolumeProvisioningContext.BladeSet.Key: "Set 1",
			},
			fmt.Errorf("xml syntax error"),
			nil,
			func() {
				// expect create volume command
				runnerMock.EXPECT().RunCommand(
					gomock.Any(),
					"volume", "create", validVolumeName, `bladeset "Set 1"`,
				).Times(1).Return([]byte{}, nil)
				// then get volume details
				runnerMock.EXPECT().RunCommand(
					gomock.Any(),
					"pasxml", "volumes", "volume", validVolumeName,
				).Times(1).Return([]byte("<invalid xml>"), fmt.Errorf("xml syntax error"))
			},
		},
		{
			"CreatedEncryptedVolume",
			validVolumeName,
			VolumeCreateParams{
				utils.VolumeProvisioningContext.Encryption.Key: "on",
			},
			nil,
			&utils.Volume{
				XMLName: xml.Name{Local: "volume"},
				Name:    validVolumeName,
				ID:      "371",
				State:   "Online",
				Bset: utils.Bladeset{
					XMLName: xml.Name{Local: "bladesetName"},
				},
				Encryption: "on",
			},
			func() {
				// expect create volume command
				runnerMock.EXPECT().RunCommand(
					gomock.Any(),
					"volume", "create", validVolumeName, "encryption on",
				).Times(1).Return([]byte{}, nil)

				genPasXML, _ := (&utils.Volume{
					ID:         "371",
					Name:       "validVolumeName",
					State:      "Online",
					Encryption: "on",
				}).MarshalVolumeToPasXML()

				// then get volume details
				runnerMock.EXPECT().RunCommand(
					gomock.Any(),
					"pasxml", "volumes", "volume", validVolumeName,
				).Times(1).Return(genPasXML, nil)
			},
		},
		{
			"CreatedEncryptedVolumeButFailedToGetDetails",
			validVolumeName,
			VolumeCreateParams{
				utils.VolumeProvisioningContext.Encryption.Key: "on",
			},
			fmt.Errorf("xml syntax error"),
			nil,
			func() {
				// expect create volume command
				runnerMock.EXPECT().RunCommand(
					gomock.Any(),
					"volume", "create", validVolumeName, "encryption on",
				).Times(1).Return([]byte{}, nil)
				// then get volume details
				runnerMock.EXPECT().RunCommand(
					gomock.Any(),
					"pasxml", "volumes", "volume", validVolumeName,
				).Times(1).Return([]byte("<invalid xml>"), fmt.Errorf("xml syntax error"))
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.mockFunc != nil {
				tc.mockFunc()
			}
			panfs := PancliSSHClient{
				runnerMock,
			}
			vol, err := panfs.CreateVolume(tc.volName, &tc.params, defaultSecrets)
			if tc.expectedErr != nil {
				assert.EqualError(t, err, tc.expectedErr.Error(), "unexpected error for test case: %s", tc.name)
			} else {
				assert.NoError(t, err, "expected no error for test case: %s", tc.name)
			}
			assert.Equal(t, tc.response, vol)
		})
	}
}

func TestDeleteVolume(t *testing.T) {
	ctrl := gomock.NewController(t)
	runnerMock := mock.NewMockSSHRunner(ctrl)

	testCases := []struct {
		name        string
		volName     string
		expectedErr error
		mockFunc    func()
	}{
		{
			"VolumeDeleted",
			validVolumeName,
			nil,
			func() {
				// expect delete volume command
				runnerMock.EXPECT().RunCommand(
					gomock.Any(),
					"volume", "delete", "-f", validVolumeName,
				).Times(1).Return([]byte{}, nil)
			},
		},
		{
			"DeleteVolumeError",
			validVolumeName,
			fmt.Errorf("delete failed"),
			func() {
				// expect delete volume command to fail
				runnerMock.EXPECT().RunCommand(
					gomock.Any(),
					"volume", "delete", "-f", validVolumeName,
				).Times(1).Return(nil, fmt.Errorf("delete failed"))
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.mockFunc != nil {
				tc.mockFunc()
			}
			panfs := PancliSSHClient{
				runnerMock,
			}
			err := panfs.DeleteVolume(tc.volName, defaultSecrets)
			if tc.expectedErr != nil {
				assert.EqualError(t, err, tc.expectedErr.Error(), "unexpected error for test case: %s", tc.name)
			} else {
				assert.NoError(t, err, "expected no error for test case: %s", tc.name)
			}
		})
	}
}

func TestGetOptionalParameters(t *testing.T) {
	tests := []struct {
		name   string
		params VolumeCreateParams
		want   []string
	}{
		{
			name:   "AllEmpty",
			params: VolumeCreateParams{},
			want:   []string{},
		},
		{
			name: "BladeSetOnly",
			params: VolumeCreateParams{
				utils.VolumeProvisioningContext.BladeSet.Key: "Set 1",
			},
			want: []string{`bladeset "Set 1"`},
		},
		{
			name: "VolServiceAndEfsa",
			params: VolumeCreateParams{
				utils.VolumeProvisioningContext.VolService.Key: "0x01",
				utils.VolumeProvisioningContext.Efsa.Key:       "retry",
			},
			want: []string{"volservice 0x01", "efsa retry"},
		},
		{
			name: "SoftAndHard",
			params: VolumeCreateParams{
				utils.VolumeProvisioningContext.Soft.Key: "1073741824", // 1GB
				utils.VolumeProvisioningContext.Hard.Key: "2147483648", // 2GB
			},
			want: []string{"soft 1.00", "hard 2.00"},
		},
		{
			name: "AllRAIDParams",
			params: VolumeCreateParams{
				utils.VolumeProvisioningContext.Layout.Key:     "RAID6",
				utils.VolumeProvisioningContext.MaxWidth.Key:   "10",
				utils.VolumeProvisioningContext.StripeUnit.Key: "64K",
				utils.VolumeProvisioningContext.RgWidth.Key:    "8",
				utils.VolumeProvisioningContext.RgDepth.Key:    "2",
			},
			want: []string{"layout RAID6", "maxwidth 10", "stripeunit 64K", "rgwidth 8", "rgdepth 2"},
		},
		{
			name: "OwnerGroupPerms",
			params: VolumeCreateParams{
				utils.VolumeProvisioningContext.User.Key:  "alice",
				utils.VolumeProvisioningContext.Group.Key: "staff",
				utils.VolumeProvisioningContext.UPerm.Key: "rwx",
				utils.VolumeProvisioningContext.GPerm.Key: "r-x",
				utils.VolumeProvisioningContext.OPerm.Key: "r--",
			},
			want: []string{`user "alice"`, `group "staff"`, "uperm rwx", "gperm r-x", "operm r--"},
		},
		{
			name: "DescriptionAndRecoveryPriority",
			params: VolumeCreateParams{
				utils.VolumeProvisioningContext.Description.Key:      "test volume",
				utils.VolumeProvisioningContext.RecoveryPriority.Key: "42",
			},
			want: []string{`description "test volume"`, "recoverypriority 42"},
		},
		{
			name: "EncryptionRequested",
			params: VolumeCreateParams{
				utils.VolumeProvisioningContext.Encryption.Key: "on",
			},
			want: []string{"encryption on"},
		},
		{
			name: "AllFields",
			params: VolumeCreateParams{
				utils.VolumeProvisioningContext.BladeSet.Key:         "Set 2",
				utils.VolumeProvisioningContext.RecoveryPriority.Key: "99",
				utils.VolumeProvisioningContext.Efsa.Key:             "file-unavailable",
				utils.VolumeProvisioningContext.Soft.Key:             "3221225472", // 3GB
				utils.VolumeProvisioningContext.Hard.Key:             "4294967296", // 4GB
				utils.VolumeProvisioningContext.VolService.Key:       "0x02",
				utils.VolumeProvisioningContext.Layout.Key:           "RAID5",
				utils.VolumeProvisioningContext.MaxWidth.Key:         "12",
				utils.VolumeProvisioningContext.StripeUnit.Key:       "128K",
				utils.VolumeProvisioningContext.RgWidth.Key:          "6",
				utils.VolumeProvisioningContext.RgDepth.Key:          "3",
				utils.VolumeProvisioningContext.User.Key:             "bob",
				utils.VolumeProvisioningContext.Group.Key:            "users",
				utils.VolumeProvisioningContext.UPerm.Key:            "rw-",
				utils.VolumeProvisioningContext.GPerm.Key:            "r--",
				utils.VolumeProvisioningContext.OPerm.Key:            "---",
				utils.VolumeProvisioningContext.Description.Key:      "full test",
			},
			want: []string{
				`bladeset "Set 2"`,
				"volservice 0x02",
				"soft 3.00",
				"hard 4.00",
				"efsa file-unavailable",
				`description "full test"`,
				"recoverypriority 99",
				"layout RAID5",
				"maxwidth 12",
				"stripeunit 128K",
				"rgwidth 6",
				"rgdepth 3",
				`user "bob"`,
				`group "users"`,
				"uperm rw-",
				"gperm r--",
				"operm ---",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := getOptionalParameters(&tc.params)
			assert.ElementsMatch(t, tc.want, got)
		})
	}
}
