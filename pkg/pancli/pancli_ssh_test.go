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
		"user":     "testuser",
		"password": "testpass",
		"realm_ip": "testrealm",
	}
	getValidVolumePasxmlResponse = `<pasxml version="6.0.0">
  <system>
    <name>virtual-realm.local.com</name>
    <IPV4>realm.ip.address</IPV4>
    <alertLevel>warning</alertLevel>
    <state>online</state>
  </system>
  <time>2025-06-26T13:10:49Z</time>
  <volumes>
      <volume id="371">
      <name>/validVolumeName</name>
      <bladesetName id="1">Set 1</bladesetName>
      <state>Online</state>
      <raid>Object RAID6+</raid>
      <director>ASD-1,1</director>
      <volservice>0x0400000000000008(FM)</volservice>
      <objectId>I-xD0200000000000008-xG00000000-xU0000000000000000</objectId>
      <recoveryPriority>50</recoveryPriority>
      <efsaMode>retry</efsaMode>
      <spaceUsedGB>0</spaceUsedGB>
      <spaceAvailableGB>95.00</spaceAvailableGB>
      <hardQuotaGB>0</hardQuotaGB>
      <softQuotaGB>0</softQuotaGB>
      <userQuotaPolicy inherit="1">disabled</userQuotaPolicy>
    </volume></volumes>
</pasxml>`
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
				BladeSet: "Set 1",
			},
			nil,
			validVolumeResponse,
			func() {
				// expect create volume command
				runnerMock.EXPECT().RunCommand(
					gomock.Any(),
					"volume", "create", validVolumeName, "bladeset \"Set 1\"",
				).Times(1).Return([]byte{}, nil)
				// then get volume details
				runnerMock.EXPECT().RunCommand(
					gomock.Any(),
					"pasxml", "volumes", "volume", validVolumeName,
				).Times(1).Return([]byte(getValidVolumePasxmlResponse), nil)
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
				BladeSet: "Set 1",
			},
			fmt.Errorf("xml syntax error"),
			nil,
			func() {
				// expect create volume command
				runnerMock.EXPECT().RunCommand(
					gomock.Any(),
					"volume", "create", validVolumeName, "bladeset \"Set 1\"",
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
				BladeSet: "Set 1",
			},
			want: []string{`bladeset "Set 1"`},
		},
		{
			name: "VolServiceAndEfsa",
			params: VolumeCreateParams{
				VolService: "0x01",
				Efsa:       "retry",
			},
			want: []string{"volservice 0x01", "efsa", "retry"},
		},
		{
			name: "SoftAndHard",
			params: VolumeCreateParams{
				Soft: 1073741824, // 1GB
				Hard: 2147483648, // 2GB
			},
			want: []string{"soft", "1.00", "hard", "2.00"},
		},
		{
			name: "AllRAIDParams",
			params: VolumeCreateParams{
				Layout:     "RAID6",
				MaxWidth:   "10",
				StripeUnit: "64K",
				RgWidth:    "8",
				RgDepth:    "2",
			},
			want: []string{"layout RAID6", "maxwidth 10", "stripeunit 64K", "rgwidth 8", "rgdepth 2"},
		},
		{
			name: "OwnerGroupPerms",
			params: VolumeCreateParams{
				User:  "alice",
				Group: "staff",
				UPerm: "rwx",
				GPerm: "r-x",
				OPerm: "r--",
			},
			want: []string{"user alice", "group staff", "uperm rwx", "gperm r-x", "operm r--"},
		},
		{
			name: "DescriptionAndRecoveryPriority",
			params: VolumeCreateParams{
				Description:      "test volume",
				Recoverypriority: "42",
			},
			want: []string{"description", "test volume", "recoverypriority", "42"},
		},
		{
			name: "AllFields",
			params: VolumeCreateParams{
				BladeSet:         "Set 2",
				Recoverypriority: "99",
				Efsa:             "file-unavailable",
				Soft:             3221225472, // 3GB
				Hard:             4294967296, // 4GB
				VolService:       "0x02",
				Layout:           "RAID5",
				MaxWidth:         "12",
				StripeUnit:       "128K",
				RgWidth:          "6",
				RgDepth:          "3",
				User:             "bob",
				Group:            "users",
				UPerm:            "rw-",
				GPerm:            "r--",
				OPerm:            "---",
				Description:      "full test",
			},
			want: []string{
				`bladeset "Set 2"`,
				"volservice 0x02",
				"soft", "3.00",
				"hard", "4.00",
				"efsa", "file-unavailable",
				"description", "full test",
				"recoverypriority", "99",
				"layout RAID5",
				"maxwidth 12",
				"stripeunit 128K",
				"rgwidth 6",
				"rgdepth 3",
				"user bob",
				"group users",
				"uperm rw-",
				"gperm r--",
				"operm ---",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := getOptionalParameters(&tc.params)
			assert.Equal(t, tc.want, got)
		})
	}
}
