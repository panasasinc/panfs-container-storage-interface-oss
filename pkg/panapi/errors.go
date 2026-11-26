// Copyright 2026 VDURA Inc.
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

package panapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

var (
	// ErrorAlreadyExist is returned when PanAPI reports a conflict on create.
	ErrorAlreadyExist = errors.New("volume already exists")
	// ErrorNotFound is returned when a requested entity was not found.
	ErrorNotFound = errors.New("requested entity was not found")
	// ErrorInvalidArgument is returned when an invalid argument was specified.
	ErrorInvalidArgument = errors.New("an invalid argument was specified")
	// ErrorUnauthenticated is returned when the API token is missing, invalid, expired or lacks permission.
	ErrorUnauthenticated = errors.New("request does not have valid authentication credentials for the operation")
	// ErrorUnavailable is returned when PanAPI cannot be reached or reports a backend outage.
	ErrorUnavailable = errors.New("connection was refused or terminated")
	// ErrorInternal is returned for every other PanAPI failure.
	ErrorInternal = errors.New("internal server error")
)

// errorFromResponse maps an HTTP status and a PanAPI error body to a sentinel error.
// PanAPI reports errors as {"detail": "<text>"}; validation errors carry a list in "detail".
func errorFromResponse(statusCode int, body []byte) error {
	detail := errorDetail(body)

	var sentinel error
	switch statusCode {
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		sentinel = ErrorInvalidArgument
	case http.StatusUnauthorized, http.StatusForbidden:
		sentinel = ErrorUnauthenticated
	case http.StatusNotFound:
		sentinel = ErrorNotFound
	case http.StatusConflict:
		sentinel = ErrorAlreadyExist
	case http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		sentinel = ErrorUnavailable
	default:
		sentinel = ErrorInternal
	}

	return fmt.Errorf("%w: HTTP %d: %s", sentinel, statusCode, detail)
}

// errorDetail extracts the "detail" field of a PanAPI error body as text.
func errorDetail(body []byte) string {
	var parsed struct {
		Detail json.RawMessage `json:"detail"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil || len(parsed.Detail) == 0 {
		return strings.TrimSpace(string(body))
	}

	var text string
	if err := json.Unmarshal(parsed.Detail, &text); err == nil {
		return text
	}

	return string(parsed.Detail)
}
