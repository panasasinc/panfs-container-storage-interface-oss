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

import "testing"

const tolerance = 0.00000001

// TestGBToBytes tests the GBToBytes function.
func TestGBToBytes(t *testing.T) {
	testCases := []struct {
		input    float64
		expected int64
	}{
		{0, 0},
		{1, 1073741824},
		{0.5, 536870912},
		{2.5, 2684354560},
	}

	for _, testCase := range testCases {
		actual := GBToBytes(testCase.input)
		if actual != testCase.expected {
			t.Errorf("GBToBytes(%f) = %d; expected %d", testCase.input, actual, testCase.expected)
		}

		// Check that converting back to GB is accurate to within the tolerance.
		actualGB := float64(actual) / bytesPerGB
		if actualGB-testCase.input > tolerance || testCase.input-actualGB > tolerance {
			t.Errorf("Conversion not accurate for input %f: GBToBytes(%f) = %d, BytesToGB(%d) = %f", testCase.input, testCase.input, actual, actual, actualGB)
		}
	}
}

// TestBytesToGB tests the BytesToGB function.
func TestBytesToGB(t *testing.T) {
	testCases := []struct {
		input    int64
		expected float64
	}{
		{0, 0},
		{1073741824, 1},
		{536870912, 0.5},
		{2684354560, 2.5},
	}

	for _, testCase := range testCases {
		actual := BytesToGB(testCase.input)
		if actual != testCase.expected {
			t.Errorf("BytesToGB(%d) = %f; expected %f", testCase.input, actual, testCase.expected)
		}

		// Check that converting back to bytes is accurate to within the tolerance.
		actualBytes := actual * bytesPerGB
		if actualBytes-float64(testCase.input) > tolerance || float64(testCase.input)-actualBytes > tolerance {
			t.Errorf("Conversion not accurate for input %d: BytesToGB(%d) = %f, GBToBytes(%f) = %f", testCase.input, testCase.input, actual, actual, actualBytes)
		}
	}
}
