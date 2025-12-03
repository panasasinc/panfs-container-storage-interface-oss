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

// VolumeParametersData defines structure for volume parameters data
type VolumeParametersData map[string]string

// VolumeParameters holds supported volume provisioning context parameters
var VolumeParameters = VolumeParametersData{
	"description": `description "%s"`,
	"bladeset":    `bladeset "%s"`,
	"recovery":    "recoverypriority %s",
	"efsa":        "efsa %s",
	"volservice":  "volservice %s",
	"layout":      "layout %s",
	"maxwidth":    "maxwidth %s",
	"stripeunit":  "stripeunit %s",
	"rgwidth":     "rgwidth %s",
	"rgdepth":     "rgdepth %s",
	"user":        `user "%s"`,
	"group":       `group "%s"`,
	"uperm":       "uperm %s",
	"gperm":       "gperm %s",
	"operm":       "operm %s",
	"encryption":  "encryption %s",
	"soft":        "soft %v", // softQuotaGB
	"hard":        "hard %v", // hardQuotaGB
}

// GetSCKey retrieves the storage class parameter key for a given context parameter key
func (c VolumeParametersData) GetSCKey(k string) string {
	short := strings.TrimPrefix(k, VendorPrefix)
	if _, ok := c[short]; ok {
		return VendorPrefix + short
	}

	return ""
}

// GetFmt retrieves the formatting string for a given context parameter key
func (c VolumeParametersData) GetFmt(k string) string {
	short := strings.TrimPrefix(c.GetSCKey(k), VendorPrefix)
	if value, ok := c[short]; ok {
		return value
	}
	return ""
}

// RealmConnectionContext holds supported realm connection context parameters
var RealmConnectionContext = struct {
	RealmAddress         string
	Username             string
	Password             string
	PrivateKey           string
	PrivateKeyPassphrase string
	KMIPConfigData       string
}{
	RealmAddress:         "realm_ip",
	Username:             "user",
	Password:             "password",
	PrivateKey:           "private_key",
	PrivateKeyPassphrase: "private_key_passphrase",
	KMIPConfigData:       "kmip_config_data",
}
