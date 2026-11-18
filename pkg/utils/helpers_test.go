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

// TestIn tests the In function.
func TestIn(t *testing.T) {
	testCases := []struct {
		name     string
		value    string
		list     []string
		expected bool
	}{
		{"present", "b", []string{"a", "b", "c"}, true},
		{"absent", "d", []string{"a", "b", "c"}, false},
		{"empty list", "a", nil, false},
		{"empty value present", "", []string{"a", ""}, true},
		{"case sensitive", "A", []string{"a"}, false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			assert.Equal(t, testCase.expected, In(testCase.value, testCase.list...))
		})
	}
}
