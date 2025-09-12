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

// Package utils provides utility functions for unit conversions.
package utils

const bytesPerGB float64 = 1073741824

// GBToBytes converts gigabytes to bytes.
//
// Parameters:
//
//	in - The size in gigabytes.
//
// Returns:
//
//	int64 - The size in bytes.
func GBToBytes(in float64) int64 {
	return int64(in * bytesPerGB)
}

// BytesToGB converts bytes to gigabytes.
//
// Parameters:
//
//	in - The size in bytes.
//
// Returns:
//
//	float64 - The size in gigabytes.
func BytesToGB(in int64) float64 {
	return float64(in) / bytesPerGB
}
