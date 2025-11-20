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
	"encoding/xml"
	"strings"
)

// VolumeName is a struct to handle volume name field from pancli pasxml volume(s) output
type VolumeName string

// UnmarshalXML implements Unmarshaler interface to involve custom handler for VolumeName field.
// This handler removes leading slash from volume name: /home -> home. CO requests volumes without leading slash
// so for PanFS CSI driver volume '/home' should be equal to 'home'
func (v *VolumeName) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var content string
	if err := d.DecodeElement(&content, &start); err != nil {
		return err
	}
	if strings.Index(content, "/") == 0 {
		content = content[1:]
	}
	*v = VolumeName(content)
	return nil
}

// VolumeList represents the XML structure returned by the `pancli` command for listing volumes.
type VolumeList struct {
	XMLName       xml.Name `xml:"pasxml"`
	Version       string   `xml:"version,attr"`
	Volumes       []Volume `xml:"volumes>volume"`
	SupportedUrls struct {
		Urls []string `xml:"url"`
	} `xml:"supportedUrls"`
}

// Bladeset represents a bladeset in the PanFS system.
type Bladeset struct {
	XMLName xml.Name `xml:"bladesetName"`
	ID      string   `xml:"id,attr"`
	Name    string   `xml:",chardata"`
}

// Volume represents a single volume in the PanFS system.
type Volume struct {
	XMLName    xml.Name   `xml:"volume"`
	ID         string     `xml:"id,attr"`
	Name       VolumeName `xml:"name"`
	State      string     `xml:"state"`
	Soft       float64    `xml:"softQuotaGB"`
	Hard       float64    `xml:"hardQuotaGB"`
	Bset       Bladeset   `xml:"bladesetName"`
	Encryption string     `xml:"encryption"`
}

// GetSoftQuotaBytes returns the soft quota in bytes.
func (v *Volume) GetSoftQuotaBytes() int64 {
	return GBToBytes(v.Soft)
}

// GetHardQuotaBytes returns the hard quota in bytes.
func (v *Volume) GetHardQuotaBytes() int64 {
	return GBToBytes(v.Hard)
}

// GetEncryptionMode returns the encryption mode of the volume.
func (vol *Volume) GetEncryptionMode() string {
	if vol.Encryption != "" {
		return vol.Encryption
	}
	return "off"
}

// MarshalVolumeToPasXML marshals the Volume struct into XML format compatible with PanFS pasxml output.
//
// Returns:
//
//	[]byte - The marshaled XML byte slice.
//	error  - Error if marshaling fails.
func (v *Volume) MarshalVolumeToPasXML() ([]byte, error) {
	list := VolumeList{
		Version: "6.0.0",
		Volumes: []Volume{*v},
		SupportedUrls: struct {
			Urls []string `xml:"url"`
		}{
			Urls: []string{},
		},
	}

	return xml.MarshalIndent(list, "", "    ")
}

// ParseListVolumes parses the XML output from the `pancli` command for listing volumes.
//
// Parameters:
//
//	volumes - The XML byte slice containing the volume list.
//
// Returns:
//
//	*VolumeList - The parsed VolumeList structure.
//	error       - Error if parsing fails.
func ParseListVolumes(volumes []byte) (*VolumeList, error) {
	var res VolumeList

	err := xml.Unmarshal(volumes, &res)
	if err != nil {
		return nil, err
	}
	return &res, nil
}
