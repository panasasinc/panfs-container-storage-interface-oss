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

package panapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/panasasinc/panfs-container-storage-interface-oss/pkg/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const attachmentJSON = `{"id":"a1","subnet_id":"s1","underlay_ip":"10.0.0.5","hostname":"node-1",
"overlay_ip":"192.168.0.58","overlay_prefix":16,"mtu":1450,"segment_type":"vxlan","segment_id":5001,
"tunnel":{"mac":"02:00:5e:10:00:a4","udp_port":4789,"vteps":["10.0.1.1","10.0.1.2"]}}`

// tenantNetworkRealm serves the routes of the tenant network calls. postBody is what an attach answers with.
func tenantNetworkRealm(t *testing.T, subnets, postBody string, postStatus int) (map[string]string, *[]string) {
	t.Helper()
	var bodies []string
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/auth/me", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+testToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = io.WriteString(w, `{"tenant_id":"t1"}`)
	})
	mux.HandleFunc("GET /api/v1/tenants/t1", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"tenant":{"id":"t1","name":"acme"}}`)
	})
	mux.HandleFunc("GET /api/v1/subnets", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, subnets)
	})
	mux.HandleFunc("GET /api/v1/subnets/s1/mount-info", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"rmlist":"192.168.0.2;192.168.0.3"}`)
	})
	mux.HandleFunc("POST /api/v1/subnets/s1/attachments", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(body))
		w.WriteHeader(postStatus)
		_, _ = io.WriteString(w, postBody)
	})
	mux.HandleFunc("GET /api/v1/subnets/s1/attachments", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "10.0.0.5", r.URL.Query().Get("underlay_ip"))
		_, _ = io.WriteString(w, `{"attachments":[`+attachmentJSON+`],"total":1}`)
	})
	mux.HandleFunc("DELETE /api/v1/subnets/s1/attachments/a1", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("DELETE /api/v1/subnets/s1/attachments/gone", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"detail":"not found"}`)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	keys := utils.RealmConnectionContext
	return map[string]string{keys.API: server.URL, keys.Token: testToken}, &bodies
}

func TestSplitRMList(t *testing.T) {
	assert.Equal(t, []string{"10.0.0.1", "10.0.0.2"}, SplitRMList("10.0.0.1;10.0.0.2"))
	assert.Equal(t, []string{"10.0.0.1", "10.0.0.2"}, SplitRMList(" 10.0.0.1 , 10.0.0.2 "))
	assert.Empty(t, SplitRMList(""))
}

func TestGetTenantNetwork(t *testing.T) {
	ctx := context.Background()

	secrets, _ := tenantNetworkRealm(t, `{"subnets":[{"id":"s1"}]}`, "", http.StatusOK)
	network, err := NewClient().GetTenantNetwork(ctx, secrets)
	require.NoError(t, err)
	assert.Equal(t, &TenantNetwork{TenantName: "acme", SubnetID: "s1", Directors: []string{"192.168.0.2", "192.168.0.3"}}, network)

	secrets, _ = tenantNetworkRealm(t, `{"subnets":[]}`, "", http.StatusOK)
	_, err = NewClient().GetTenantNetwork(ctx, secrets)
	assert.True(t, errors.Is(err, ErrorNotFound))

	secrets[utils.RealmConnectionContext.Token] = "wrong"
	_, err = NewClient().GetTenantNetwork(ctx, secrets)
	assert.True(t, errors.Is(err, ErrorUnauthenticated))
}

func TestAttachHost(t *testing.T) {
	ctx := context.Background()

	t.Run("a first attach returns the new record", func(t *testing.T) {
		secrets, bodies := tenantNetworkRealm(t, "", attachmentJSON, http.StatusCreated)
		attachment, err := NewClient().AttachHost(ctx, "s1", "10.0.0.5", "node-1", secrets)
		require.NoError(t, err)
		assert.Equal(t, "192.168.0.58", attachment.OverlayIP)
		assert.Equal(t, 5001, attachment.SegmentID)
		assert.Equal(t, []string{"10.0.1.1", "10.0.1.2"}, attachment.Tunnel.VTEPs)

		var sent map[string]string
		require.NoError(t, json.Unmarshal([]byte((*bodies)[0]), &sent))
		assert.Equal(t, map[string]string{"underlay_ip": "10.0.0.5", "hostname": "node-1"}, sent)
	})

	t.Run("a repeated attach without a record reads the list", func(t *testing.T) {
		secrets, _ := tenantNetworkRealm(t, "", `{}`, http.StatusOK)
		attachment, err := NewClient().AttachHost(ctx, "s1", "10.0.0.5", "node-1", secrets)
		require.NoError(t, err)
		assert.Equal(t, "a1", attachment.ID)
	})

	t.Run("a refused attach is an error", func(t *testing.T) {
		secrets, _ := tenantNetworkRealm(t, "", `{"detail":"forbidden"}`, http.StatusForbidden)
		_, err := NewClient().AttachHost(ctx, "s1", "10.0.0.5", "node-1", secrets)
		assert.Error(t, err)
	})
}

func TestFindAndDetachHost(t *testing.T) {
	ctx := context.Background()
	secrets, _ := tenantNetworkRealm(t, "", "", http.StatusOK)

	found, err := NewClient().FindAttachment(ctx, "s1", "10.0.0.5", secrets)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, "a1", found.ID)

	require.NoError(t, NewClient().DetachHost(ctx, "s1", "a1", secrets))
	// An attachment that is already gone is not an error.
	require.NoError(t, NewClient().DetachHost(ctx, "s1", "gone", secrets))
}
