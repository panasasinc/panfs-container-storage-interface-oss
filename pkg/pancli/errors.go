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
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

var (
	// ErrorNotImplemented is returned when an operation is not implemented.
	ErrorNotImplemented = errors.New("operation is not implemented")
	// ErrorAlreadyExist is returned when a volume already exists.
	ErrorAlreadyExist = errors.New("volume already exists")
	// ErrorNotFound is returned when a requested entity was not found.
	ErrorNotFound = errors.New("requested entity was not found")
	// ErrorInvalidArgument is returned when an invalid argument was specified.
	ErrorInvalidArgument = errors.New("an invalid argument was specified")
	// ErrorUnauthenticated is returned when authentication credentials are invalid.
	ErrorUnauthenticated = errors.New("request does not have valid authentication credentials for the operation")
	// ErrorUnavailable is returned when a connection was refused or terminated.
	ErrorUnavailable = errors.New("connection was refused or terminated")
	// ErrorInternal is returned for internal server errors.
	ErrorInternal = errors.New("internal server error")
)

// parseErrorString parses an error string and returns a corresponding error value.
// Matches known error patterns and returns specific error types, or nil for success.
//
// Parameters:
//
//	errorStr - The error string to parse.
//
// Returns:
//
//	error - The parsed error value, or nil if no error.
func parseErrorString(errorStr string) error {
	s := strings.ToLower(errorStr)
	switch {
	case strings.Contains(s, "already exists"):
		return fmt.Errorf("%w: %s", ErrorAlreadyExist, errorStr)
	case strings.Contains(s, "no volume with name"):
		return fmt.Errorf("%w: %s", ErrorNotFound, errorStr)
	case strings.Contains(s, "successfully"):
		return nil
	case strings.Contains(s, "<volumes>"):
		return nil
	case strings.Contains(s, "do not exist"):
		return fmt.Errorf("%w: %s", ErrorNotFound, errorStr)
	//	internal errors
	case strings.Contains(s, "must be one of"), strings.Contains(s, "invalid string"):
		// Normalize NBSP -> space, remove newlines
		reNBSP := regexp.MustCompile("\u00A0")
		clean := reNBSP.ReplaceAllString(errorStr, " ")

		// Collapse whitespace, remove newlines
		clean = strings.Join(strings.Fields(clean), " ")

		// Remove help line at the end
		reHelpLine := regexp.MustCompile(`\s*Use the command "[^"]*" to get more help\.?\s*$`)
		clean = reHelpLine.ReplaceAllString(clean, "")

		// Remove trailing ", -f."
		reTrailingF := regexp.MustCompile(`,\s*-f\.$`)
		clean = reTrailingF.ReplaceAllString(clean, "")

		// Trim spaces
		clean = strings.TrimSpace(clean)

		if clean == "" {
			return fmt.Errorf("%w: %s", ErrorInvalidArgument, errorStr)
		}

		// Capitalize first rune
		runes := []rune(clean)
		runes[0] = unicode.ToUpper(runes[0])
		clean = string(runes)

		return fmt.Errorf("%w: %s", ErrorInvalidArgument, clean)
	case strings.Contains(s, "should be"):
		return fmt.Errorf("%w: %s", ErrorInvalidArgument, errorStr)
	case strings.Contains(s, "status 255"):
		return fmt.Errorf("%w: %s", ErrorUnavailable, errorStr)
	default:
		return fmt.Errorf("%w: %s", ErrorInternal, errorStr)
	}
}
