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

package pancli

import (
	"errors"
	"testing"
)

// TestParseOutput tests the parseErrorString function.
// It verifies correct error mapping for various error string inputs.
func TestParseOutput(t *testing.T) {
	testCases := []struct {
		input    string
		expected error
	}{
		{
			input:    "Volume already exists",
			expected: ErrorAlreadyExist,
		},
		{
			input:    "No volume with name 'test'",
			expected: ErrorNotFound,
		},
		{
			input:    "Invalid string argument: 'test'",
			expected: ErrorInvalidArgument,
		},
		{
			input:    "Invalid argument: size should be greater than 0",
			expected: ErrorInvalidArgument,
		},
		{
			input:    "Command failed with status 255",
			expected: ErrorUnavailable,
		},
		{
			input:    "Some random error message",
			expected: ErrorInternal,
		},
		{
			input:    "successfully",
			expected: nil,
		},
		{
			input:    "<version>123\n</version>\n<volumes>foo\n</volumes>",
			expected: nil,
		},
	}

	for _, testCase := range testCases {
		actual := parseErrorString(testCase.input)

		if !errors.Is(actual, testCase.expected) {
			t.Errorf("Expected error: %v but got: %v", testCase.expected, actual)
		}
	}
}
