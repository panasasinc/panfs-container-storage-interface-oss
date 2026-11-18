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
)

// TestGetSCKey tests that known parameters resolve to the vendor-prefixed StorageClass key.
func TestGetSCKey(t *testing.T) {
	testCases := []struct {
		name     string
		key      string
		expected string
	}{
		{"short key", "layout", VendorPrefix + "layout"},
		{"prefixed key", VendorPrefix + "layout", VendorPrefix + "layout"},
		{"unknown short key", "nosuchparam", ""},
		{"unknown prefixed key", VendorPrefix + "nosuchparam", ""},
		{"empty key", "", ""},
		{"prefix only", VendorPrefix, ""},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			assert.Equal(t, testCase.expected, VolumeParameters.GetSCKey(testCase.key))
		})
	}
}

// TestGetFmt tests that known parameters resolve to their pancli format string.
func TestGetFmt(t *testing.T) {
	testCases := []struct {
		name     string
		key      string
		expected string
	}{
		{"short key", "layout", "layout %s"},
		{"prefixed key", VendorPrefix + "storageset", `storageset "%s"`},
		{"key that differs from the pancli option", "recovery", "recoverypriority %s"},
		{"quota key", "soft", "soft %v"},
		{"unknown key", "nosuchparam", ""},
		{"empty key", "", ""},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			assert.Equal(t, testCase.expected, VolumeParameters.GetFmt(testCase.key))
		})
	}
}

// TestVolumeParametersResolve tests that every known parameter resolves in both lookups.
func TestVolumeParametersResolve(t *testing.T) {
	for key, format := range VolumeParameters {
		assert.Equal(t, VendorPrefix+key, VolumeParameters.GetSCKey(key))
		assert.Equal(t, format, VolumeParameters.GetFmt(VendorPrefix+key))
	}
}
