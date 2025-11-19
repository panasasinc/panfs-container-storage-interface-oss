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

// Vendor Prefix for PanFS CSI Driver
const VendorPrefix = "panfs.csi.vdura.com/"

type ContextParameterData struct {
	Key string
	Fmt string
}

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
	Description:      ContextParameterData{Key: VendorPrefix + "description", Fmt: `description "%s"`},
	BladeSet:         ContextParameterData{Key: VendorPrefix + "bladeset", Fmt: `bladeset "%s"`},
	RecoveryPriority: ContextParameterData{Key: VendorPrefix + "recoverypriority", Fmt: "recoverypriority %s"},
	Efsa:             ContextParameterData{Key: VendorPrefix + "efsa", Fmt: "efsa %s"},
	VolService:       ContextParameterData{Key: VendorPrefix + "volservice", Fmt: "volservice %s"},
	Layout:           ContextParameterData{Key: VendorPrefix + "layout", Fmt: "layout %s"},
	MaxWidth:         ContextParameterData{Key: VendorPrefix + "maxwidth", Fmt: "maxwidth %s"},
	StripeUnit:       ContextParameterData{Key: VendorPrefix + "stripeunit", Fmt: "stripeunit %s"},
	RgWidth:          ContextParameterData{Key: VendorPrefix + "rgwidth", Fmt: "rgwidth %s"},
	RgDepth:          ContextParameterData{Key: VendorPrefix + "rgdepth", Fmt: "rgdepth %s"},
	User:             ContextParameterData{Key: VendorPrefix + "user", Fmt: `user "%s"`},
	Group:            ContextParameterData{Key: VendorPrefix + "group", Fmt: `group "%s"`},
	UPerm:            ContextParameterData{Key: VendorPrefix + "uperm", Fmt: "uperm %s"},
	GPerm:            ContextParameterData{Key: VendorPrefix + "gperm", Fmt: "gperm %s"},
	OPerm:            ContextParameterData{Key: VendorPrefix + "operm", Fmt: "operm %s"},
	Encryption:       ContextParameterData{Key: VendorPrefix + "encryption", Fmt: "encryption %s"},
	Soft:             ContextParameterData{Key: "soft", Fmt: "soft %v"},
	Hard:             ContextParameterData{Key: "hard", Fmt: "hard %v"},
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
