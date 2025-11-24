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

import "strings"

// VendorPrefix for PanFS CSI Driver
const VendorPrefix = "panfs.csi.vdura.com/"

// ContextParameterData defines structure for context parameter data
type ContextParameterData struct {
	PasXMLKey string
	Arg       string
}

// GetKey returns context parameter key with vendor prefix
func (c ContextParameterData) GetKey() string {
	return VendorPrefix + strings.Fields(c.Arg)[0]
}

// VolumeProvisioningContextData holds supported volume provisioning context parameters
type VolumeProvisioningContextData struct {
	Description      ContextParameterData
	BladeSet         ContextParameterData
	RecoveryPriority ContextParameterData
	Efsa             ContextParameterData
	VolService       ContextParameterData
	Layout           ContextParameterData
	MaxWidth         ContextParameterData
	StripeUnit       ContextParameterData
	RgWidth          ContextParameterData
	RgDepth          ContextParameterData
	User             ContextParameterData
	Group            ContextParameterData
	UPerm            ContextParameterData
	GPerm            ContextParameterData
	OPerm            ContextParameterData
	Encryption       ContextParameterData
	Soft             ContextParameterData
	Hard             ContextParameterData
}

// Supported Volume Parameters Keys
var VolumeProvisioningContext = VolumeProvisioningContextData{
	Description:      ContextParameterData{PasXMLKey: "description", Arg: `description "%s"`},
	BladeSet:         ContextParameterData{PasXMLKey: "bladesetName/Name", Arg: `bladeset "%s"`},
	RecoveryPriority: ContextParameterData{PasXMLKey: "recoverypriority", Arg: "recoverypriority %s"},
	Efsa:             ContextParameterData{PasXMLKey: "efsa", Arg: "efsa %s"},
	VolService:       ContextParameterData{PasXMLKey: "volservice", Arg: "volservice %s"},
	Layout:           ContextParameterData{PasXMLKey: "layout", Arg: "layout %s"},
	MaxWidth:         ContextParameterData{PasXMLKey: "maxwidth", Arg: "maxwidth %s"},
	StripeUnit:       ContextParameterData{PasXMLKey: "stripeunit", Arg: "stripeunit %s"},
	RgWidth:          ContextParameterData{PasXMLKey: "rgwidth", Arg: "rgwidth %s"},
	RgDepth:          ContextParameterData{PasXMLKey: "rgdepth", Arg: "rgdepth %s"},
	User:             ContextParameterData{PasXMLKey: "user", Arg: `user "%s"`},
	Group:            ContextParameterData{PasXMLKey: "group", Arg: `group "%s"`},
	UPerm:            ContextParameterData{PasXMLKey: "uperm", Arg: "uperm %s"},
	GPerm:            ContextParameterData{PasXMLKey: "gperm", Arg: "gperm %s"},
	OPerm:            ContextParameterData{PasXMLKey: "operm", Arg: "operm %s"},
	Encryption:       ContextParameterData{PasXMLKey: "encryption", Arg: "encryption %s"},
	Soft:             ContextParameterData{PasXMLKey: "hardQuotaGB", Arg: "soft %v"},
	Hard:             ContextParameterData{PasXMLKey: "softQuotaGB", Arg: "hard %v"},
}

// Realm Connection Parameters Keys
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
