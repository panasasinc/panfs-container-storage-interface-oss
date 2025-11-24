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
	"fmt"
	"reflect"
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
func (v *Volume) GetEncryptionMode() string {
	if v.Encryption != "" {
		return v.Encryption
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

// ToParams converts the Volume struct into a map of string key-value pairs like volume creation parameters.
//
// Returns:
//
//	map[string]string - The map of volume creation parameters.
func (v *Volume) VolumeContext() map[string]string {
	params := FlattenXMLStruct(v, true)

	// name is reflected in csi.Volume{VolumeId} field, so we don't need it in VolumeContext
	delete(params, "name")

	// Remove quota keys as they are managed separately, in csi.Volume{CapacityBytes}
	delete(params, VolumeProvisioningContext.Soft.PasXMLKey)
	delete(params, VolumeProvisioningContext.Hard.PasXMLKey)

	return params
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

// FlattenXMLStruct flattens a struct with XML tags into a map[string]string.
//
// Parameters:
//
//	v        - The struct to flatten.
//	skipEmpty - If true, fields with zero values will be skipped.
//
// Returns:
//
//	map[string]string - The flattened map representation of the struct.
func FlattenXMLStruct(v interface{}, skipEmpty bool) map[string]string {
	out := make(map[string]string)
	flattenXML(reflect.ValueOf(v), "", out, skipEmpty)
	return out
}

// flattenXML is a recursive helper function to flatten XML struct fields.
//
// Parameters:
//
//	val       - The reflect.Value of the current struct or field.
//	prefix    - The current key prefix for nested fields.
//	out       - The output map to store flattened key-value pairs.
//	skipEmpty - If true, fields with zero values will be skipped.
func flattenXML(val reflect.Value, prefix string, out map[string]string, skipEmpty bool) {
	if val.Kind() == reflect.Pointer {
		if val.IsNil() {
			return
		}
		val = val.Elem()
	}

	// Skip zero values if requested
	if skipEmpty && isZeroValue(val) {
		return
	}

	switch val.Kind() {

	// -------------------------------------
	// Primitive → map entry
	// -------------------------------------
	case reflect.String:
		if !skipEmpty || val.String() != "" {
			out[prefix] = val.String()
		}
		return

	case reflect.Bool:
		if !skipEmpty || val.Bool() {
			out[prefix] = fmt.Sprintf("%t", val.Bool())
		}
		return

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if !skipEmpty || val.Int() != 0 {
			out[prefix] = fmt.Sprintf("%d", val.Int())
		}
		return

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if !skipEmpty || val.Uint() != 0 {
			out[prefix] = fmt.Sprintf("%d", val.Uint())
		}
		return

	case reflect.Float32, reflect.Float64:
		if !skipEmpty || val.Float() != 0 {
			out[prefix] = fmt.Sprintf("%g", val.Float())
		}
		return

	// -------------------------------------
	// Slice
	// -------------------------------------
	case reflect.Slice:
		if skipEmpty && val.Len() == 0 {
			return
		}
		for i := 0; i < val.Len(); i++ {
			childKey := fmt.Sprintf("%s[%d]", prefix, i)
			flattenXML(val.Index(i), childKey, out, skipEmpty)
		}
		return

	// -------------------------------------
	// Struct
	// -------------------------------------
	case reflect.Struct:
		t := val.Type()

		for i := 0; i < val.NumField(); i++ {
			f := t.Field(i)
			fv := val.Field(i)

			if f.PkgPath != "" { // unexported
				continue
			}

			xmlTag := f.Tag.Get("xml")
			key := xmlTag

			if xmlTag == "" {
				key = f.Name
			}

			// Handle attributes: xml:"id,attr"
			if idx := strings.Index(xmlTag, ","); idx != -1 {
				key = xmlTag[:idx]
			}

			// Skip explicit "-"
			if key == "-" {
				continue
			}

			// Handle chardata: xml:",chardata"
			if key == "" && strings.Contains(xmlTag, "chardata") {
				key = f.Name
			}

			childPrefix := key
			if prefix != "" {
				childPrefix = prefix + "/" + childPrefix
			}

			flattenXML(fv, childPrefix, out, skipEmpty)
		}
	}
}

// isZeroValue checks if a reflect.Value is the zero value for its type.
//
// Parameters:
//
//	v - The reflect.Value to check.
//
// Returns:
//
//	bool - True if the value is zero, false otherwise.
func isZeroValue(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.String:
		return v.String() == ""
	case reflect.Bool:
		return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Pointer, reflect.Interface:
		return v.IsNil()
	case reflect.Slice, reflect.Array, reflect.Map:
		return v.Len() == 0
	case reflect.Struct:
		// Zero if all fields are zero
		for i := range v.NumField() {
			if !isZeroValue(v.Field(i)) {
				return false
			}
		}
		return true
	}
	return false
}
