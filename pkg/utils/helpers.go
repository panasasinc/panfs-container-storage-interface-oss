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

import "slices"

// in checks if the value is in the provided list of strings.
// Parameters:
//
//	value - The string value to check.
//	list  - Variadic list of strings to search.
//
// Returns:
//
//	bool - True if value is in list, false otherwise.
func In(value string, list ...string) bool {
	return slices.Contains(list, value)
}
