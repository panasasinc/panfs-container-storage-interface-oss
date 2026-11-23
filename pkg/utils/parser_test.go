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

package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const listVolumesXML = `<?xml version="1.0" encoding="UTF-8"?>
<pasxml version="6.0.0">
    <volumes>
        <volume id="42">
            <name>/vol1</name>
            <state>Online</state>
            <softQuotaGB>1.5</softQuotaGB>
            <hardQuotaGB>2</hardQuotaGB>
            <storagesetName id="7">Set 1</storagesetName>
            <encryption>aes-xts-256</encryption>
        </volume>
        <volume id="43">
            <name>vol2</name>
            <state>Offline</state>
            <softQuotaGB>0</softQuotaGB>
            <hardQuotaGB>0</hardQuotaGB>
            <storagesetName id="7">Set 1</storagesetName>
        </volume>
    </volumes>
</pasxml>`

// TestParseListVolumes tests parsing of a pancli volume list in pasxml format.
func TestParseListVolumes(t *testing.T) {
	list, err := ParseListVolumes([]byte(listVolumesXML))
	require.NoError(t, err)
	require.Len(t, list.Volumes, 2)
	assert.Equal(t, "6.0.0", list.Version)

	first := list.Volumes[0]
	assert.Equal(t, "42", first.ID)
	assert.Equal(t, VolumeName("vol1"), first.Name, "the leading slash must be removed")
	assert.Equal(t, "Online", first.State)
	assert.Equal(t, "7", first.StorageSet.ID)
	assert.Equal(t, "Set 1", first.StorageSet.Name)
	assert.Equal(t, int64(1610612736), first.GetSoftQuotaBytes())
	assert.Equal(t, int64(2147483648), first.GetHardQuotaBytes())
	assert.Equal(t, "aes-xts-256", first.GetEncryptionMode())

	second := list.Volumes[1]
	assert.Equal(t, VolumeName("vol2"), second.Name, "a name without a leading slash must stay as it is")
	assert.Equal(t, int64(0), second.GetSoftQuotaBytes())
	assert.Equal(t, int64(0), second.GetHardQuotaBytes())
	assert.Empty(t, second.GetEncryptionMode())
}

// TestParseListVolumesEmpty tests parsing of a list that contains no volumes.
func TestParseListVolumesEmpty(t *testing.T) {
	list, err := ParseListVolumes([]byte(`<pasxml version="6.0.0"><volumes></volumes></pasxml>`))
	require.NoError(t, err)
	assert.Empty(t, list.Volumes)
}

// TestParseListVolumesInvalid tests that malformed input returns an error.
func TestParseListVolumesInvalid(t *testing.T) {
	testCases := []struct {
		name  string
		input string
	}{
		{"empty input", ""},
		{"not xml", "volume vol1 not found"},
		{"truncated xml", `<pasxml version="6.0.0"><volumes><volume id="1">`},
		{"quota is not a number", `<pasxml><volumes><volume><softQuotaGB>big</softQuotaGB></volume></volumes></pasxml>`},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			list, err := ParseListVolumes([]byte(testCase.input))
			require.Error(t, err)
			assert.Nil(t, list)
		})
	}
}

// TestMarshalVolumeToPasXML tests that a marshalled volume parses back to the same values.
func TestMarshalVolumeToPasXML(t *testing.T) {
	volume := Volume{
		ID:         "42",
		Name:       "vol1",
		State:      "Online",
		Soft:       1.5,
		Hard:       2,
		StorageSet: Storageset{ID: "7", Name: "Set 1"},
		Encryption: "aes-xts-256",
	}

	out, err := volume.MarshalVolumeToPasXML()
	require.NoError(t, err)

	list, err := ParseListVolumes(out)
	require.NoError(t, err)
	require.Len(t, list.Volumes, 1)
	assert.Equal(t, "6.0.0", list.Version)

	parsed := list.Volumes[0]
	assert.Equal(t, volume.ID, parsed.ID)
	assert.Equal(t, volume.Name, parsed.Name)
	assert.Equal(t, volume.State, parsed.State)
	assert.InDelta(t, volume.Soft, parsed.Soft, tolerance)
	assert.InDelta(t, volume.Hard, parsed.Hard, tolerance)
	assert.Equal(t, volume.StorageSet.ID, parsed.StorageSet.ID)
	assert.Equal(t, volume.StorageSet.Name, parsed.StorageSet.Name)
	assert.Equal(t, volume.Encryption, parsed.Encryption)
}

// TestVolumeContext tests that the volume context carries the encryption mode only when it is set.
func TestVolumeContext(t *testing.T) {
	encrypted := Volume{Encryption: "aes-xts-256"}
	assert.Equal(t, map[string]string{VendorPrefix + "encryption": "aes-xts-256"}, encrypted.VolumeContext())

	plain := Volume{}
	assert.Empty(t, plain.VolumeContext())
}
