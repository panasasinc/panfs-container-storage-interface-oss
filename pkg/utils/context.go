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
	"strings"
)

// VendorPrefix for PanFS CSI Driver
const VendorPrefix = "panfs.csi.vdura.com/"

// VolumeNameContextKey is the volume context key that carries the volume name from the controller to the node.
// The CSI volume ID is the PanAPI volume id, while the node mounts the volume by name.
const VolumeNameContextKey = VendorPrefix + "volume-name"

// VolumeParametersData is the set of known StorageClass parameter names, without the vendor prefix.
type VolumeParametersData map[string]struct{}

// VolumeParameters holds the known StorageClass parameters.
// It also lists the parameters of earlier driver versions that the driver now rejects,
// so that a request carrying one of them gets a clear error.
var VolumeParameters = VolumeParametersData{
	// storageset_id is the numeric id of the storageset that holds new volumes; it is required.
	"storageset_id":    {},
	"recoverypriority": {},
	"efsa":             {},
	"volservice":       {},
	"layout":           {},
	"maxwidth":         {},
	"stripeunit":       {},
	"rgwidth":          {},
	"rgdepth":          {},
	"user":             {},
	"group":            {},
	"uperm":            {},
	"gperm":            {},
	"operm":            {},

	// Rejected parameters of earlier driver versions.
	"description": {},
	"storageset":  {},
	"recovery":    {},
	"encryption":  {},
}

// GetSCKey retrieves the storage class parameter key for a given context parameter key
func (c VolumeParametersData) GetSCKey(k string) string {
	short := strings.TrimPrefix(k, VendorPrefix)
	if _, ok := c[short]; ok {
		return VendorPrefix + short
	}

	return ""
}

// RealmConnectionContext holds supported realm connection context parameters
var RealmConnectionContext = struct {
	API    string
	Token  string
	CACert string
}{
	API:    "api",
	Token:  "token",
	CACert: "ca_cert",
}
