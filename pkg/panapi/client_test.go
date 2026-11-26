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
	"context"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/panasasinc/panfs-container-storage-interface-oss/pkg/utils"
)

const testToken = "test-token"

// testTenantID is the tenant of testToken on the fake realm.
const testTenantID = "11111111-2222-3333-4444-555555555555"

// recordedRequest is one request the fake realm received.
type recordedRequest struct {
	Method string
	Path   string
	Auth   string
	Body   string
}

// fakeRealm is an in-memory PanAPI served over httptest. It implements the routes the client uses.
type fakeRealm struct {
	mu       sync.Mutex
	nextID   int64
	volumes  map[int64]Volume
	requests []recordedRequest
	// failures maps "METHOD /path" to a status code the realm answers with instead of serving the route.
	failures map[string]int
	// storagesets are the storagesets assigned to the tenant of the token, in the order the realm lists them.
	storagesets []map[string]any
	// subnets maps the id of a tenant subnet to the rmlist of its mount recipe.
	subnets map[string]string
	// subnetOrder keeps the ids in the order the realm lists them.
	subnetOrder []string
}

func (r *fakeRealm) addSubnet(id, rmlist string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.subnets == nil {
		r.subnets = make(map[string]string)
	}
	r.subnets[id] = rmlist
	r.subnetOrder = append(r.subnetOrder, id)
}

func newFakeRealm() *fakeRealm {
	return &fakeRealm{
		nextID:   10,
		volumes:  make(map[int64]Volume),
		failures: make(map[string]int),
		storagesets: []map[string]any{
			{"storageset_id": 3, "storageset_name": "Set 1", "quota_bytes": 100000000000},
			{"storageset_id": 7, "storageset_name": "Fast Set", "quota_bytes": 100000000000},
		},
	}
}

func (r *fakeRealm) addVolume(pathname string, soft, hard int64) Volume {
	r.mu.Lock()
	defer r.mu.Unlock()
	vol := Volume{ID: r.nextID, Pathname: pathname, Status: "online", StoragesetID: 1, SoftQuotaBytes: &soft, HardQuotaBytes: &hard}
	r.volumes[vol.ID] = vol
	r.nextID++
	return vol
}

func (r *fakeRealm) recorded() []recordedRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]recordedRequest(nil), r.requests...)
}

// lastBody returns the body of the last request with the given method.
func (r *fakeRealm) lastBody(t *testing.T, method string) map[string]any {
	t.Helper()
	requests := r.recorded()
	for i := len(requests) - 1; i >= 0; i-- {
		if requests[i].Method != method {
			continue
		}
		var body map[string]any
		decoder := json.NewDecoder(strings.NewReader(requests[i].Body))
		decoder.UseNumber()
		if err := decoder.Decode(&body); err != nil {
			t.Fatalf("decode %s body %q: %v", method, requests[i].Body, err)
		}
		return body
	}
	t.Fatalf("no %s request was recorded", method)
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeDetail(w http.ResponseWriter, status int, detail string) {
	writeJSON(w, status, map[string]string{"detail": detail})
}

func (r *fakeRealm) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	body, _ := io.ReadAll(req.Body)

	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests = append(r.requests, recordedRequest{req.Method, req.URL.Path, req.Header.Get("Authorization"), string(body)})

	if req.Header.Get("Authorization") != "Bearer "+testToken {
		writeDetail(w, http.StatusUnauthorized, "Invalid or expired token")
		return
	}
	if status, ok := r.failures[req.Method+" "+req.URL.Path]; ok {
		writeDetail(w, status, "injected failure")
		return
	}

	switch {
	case req.URL.Path == authMePath && req.Method == http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"tenant_id": testTenantID, "roles": []string{"admin"}})

	case req.URL.Path == tenantsPath+"/"+testTenantID+"/storagesets" && req.Method == http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"storagesets": r.storagesets, "total": len(r.storagesets)})

	case req.URL.Path == subnetsPath && req.Method == http.MethodGet:
		list := make([]map[string]string, 0, len(r.subnetOrder))
		for _, id := range r.subnetOrder {
			list = append(list, map[string]string{"id": id, "name": "subnet-" + id})
		}
		writeJSON(w, http.StatusOK, map[string]any{"subnets": list, "total": len(list)})

	case strings.HasPrefix(req.URL.Path, subnetsPath+"/") && strings.HasSuffix(req.URL.Path, "/mount-info") && req.Method == http.MethodGet:
		id := strings.TrimSuffix(strings.TrimPrefix(req.URL.Path, subnetsPath+"/"), "/mount-info")
		rmlist, ok := r.subnets[id]
		if !ok {
			writeDetail(w, http.StatusNotFound, "Subnet not found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"subnet_id": id, "rmlist": rmlist})

	case req.URL.Path == volumesPath && req.Method == http.MethodGet:
		list := make([]Volume, 0, len(r.volumes))
		for _, vol := range r.volumes {
			list = append(list, vol)
		}
		writeJSON(w, http.StatusOK, map[string]any{"volumes": list, "total": len(list)})

	case req.URL.Path == volumesPath && req.Method == http.MethodPost:
		var create volumeCreateRequest
		if err := json.Unmarshal(body, &create); err != nil {
			writeDetail(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		for _, vol := range r.volumes {
			if vol.Pathname == create.Pathname {
				writeDetail(w, http.StatusConflict, "Volume already exists")
				return
			}
		}
		vol := Volume{
			ID: r.nextID, Pathname: create.Pathname, Status: "online", StoragesetID: create.StoragesetID,
			SoftQuotaBytes: create.SoftQuotaBytes, HardQuotaBytes: create.HardQuotaBytes,
		}
		r.volumes[vol.ID] = vol
		r.nextID++
		writeJSON(w, http.StatusCreated, map[string]int64{"id": vol.ID})

	case strings.HasPrefix(req.URL.Path, volumesPath+"/"):
		id, err := strconv.ParseInt(strings.TrimPrefix(req.URL.Path, volumesPath+"/"), 10, 64)
		if err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
				"detail": []map[string]any{{"type": "int_parsing", "loc": []string{"path", "volume_id"}, "msg": "not an integer"}},
			})
			return
		}
		vol, ok := r.volumes[id]
		if !ok {
			writeDetail(w, http.StatusNotFound, "Volume not found")
			return
		}
		switch req.Method {
		case http.MethodGet:
			writeJSON(w, http.StatusOK, map[string]any{"volume": vol})
		case http.MethodPatch:
			var patch volumePatchRequest
			if err := json.Unmarshal(body, &patch); err != nil {
				writeDetail(w, http.StatusUnprocessableEntity, err.Error())
				return
			}
			if patch.SoftQuotaBytes != nil {
				vol.SoftQuotaBytes = patch.SoftQuotaBytes
			}
			if patch.HardQuotaBytes != nil {
				vol.HardQuotaBytes = patch.HardQuotaBytes
			}
			r.volumes[id] = vol
			w.WriteHeader(http.StatusNoContent)
		case http.MethodDelete:
			delete(r.volumes, id)
			w.WriteHeader(http.StatusNoContent)
		default:
			writeDetail(w, http.StatusMethodNotAllowed, "Method Not Allowed")
		}

	default:
		writeDetail(w, http.StatusNotFound, "Not Found")
	}
}

// startRealm serves a fake realm over plain HTTP and returns it with the secrets that reach it.
func startRealm(t *testing.T) (*fakeRealm, map[string]string) {
	t.Helper()
	realm := newFakeRealm()
	server := httptest.NewServer(realm)
	t.Cleanup(server.Close)

	return realm, map[string]string{
		utils.RealmConnectionContext.Token: testToken,
		utils.RealmConnectionContext.API:   server.URL,
	}
}

func param(short string) string {
	return utils.VendorPrefix + short
}

// inStorageset returns the parameters with the required storageset id 3 added, unless they already carry one.
func inStorageset(parameters map[string]string) map[string]string {
	out := map[string]string{StoragesetParameter: "3"}
	for key, value := range parameters {
		out[key] = value
	}
	return out
}

func TestCreateVolume(t *testing.T) {
	const size = int64(10737418241) // 10 GiB plus one byte: must reach the realm unrounded

	t.Run("sends exact bytes and translated parameters", func(t *testing.T) {
		realm, secrets := startRealm(t)

		vol, err := NewClient().CreateVolume(t.Context(), CreateVolumeParams{
			Name: "pvc-1",
			Parameters: map[string]string{
				StoragesetParameter:           "3",
				param("layout"):               "raid6+",
				param("recoverypriority"):     "75",
				param("efsa"):                 "retry",
				param("volservice"):           "0x0400000000000005",
				param("maxwidth"):             "3",
				param("stripeunit"):           "64k",
				param("rgwidth"):              "8",
				param("rgdepth"):              "2",
				param("user"):                 "1001",
				param("group"):                "100",
				param("uperm"):                "all",
				param("gperm"):                "read-only",
				param("operm"):                "none",
				"csi.storage.k8s.io/pvc/name": "unrelated",
			},
			SoftQuotaBytes: size,
			HardQuotaBytes: size,
		}, secrets)
		if err != nil {
			t.Fatalf("CreateVolume() error = %v", err)
		}

		if vol.Pathname != "/pvc-1" || vol.Name() != "pvc-1" || vol.SoftQuota() != size || vol.HardQuota() != size {
			t.Errorf("CreateVolume() = %+v, want pathname /pvc-1 with quotas %d", vol, size)
		}
		if vol.CSIID() != strconv.FormatInt(vol.ID, 10) || vol.ID == 0 {
			t.Errorf("CSIID() = %q for id %d", vol.CSIID(), vol.ID)
		}
		if got := vol.VolumeContext()[utils.VolumeNameContextKey]; got != "pvc-1" {
			t.Errorf("VolumeContext()[%s] = %q, want pvc-1", utils.VolumeNameContextKey, got)
		}

		want := map[string]any{
			"pathname":          "/pvc-1",
			"storageset_id":     json.Number("3"),
			"raid_type":         "raid6+",
			"recovery_priority": json.Number("75"),
			"efsa_mode":         "retry",
			"volume_service_id": "0x0400000000000005",
			"max_width":         json.Number("3"),
			"stripe_unit_bytes": json.Number("65536"),
			"rg_width":          json.Number("8"),
			"rg_depth":          json.Number("2"),
			"user":              "1001",
			"group":             "100",
			"user_permissions":  "all",
			"group_permissions": "read-only",
			"other_permissions": "none",
			"soft_quota_bytes":  json.Number("10737418241"),
			"hard_quota_bytes":  json.Number("10737418241"),
		}
		got := realm.lastBody(t, http.MethodPost)
		if len(got) != len(want) {
			t.Errorf("POST body has %d fields, want %d: %v", len(got), len(want), got)
		}
		for key, wantValue := range want {
			if got[key] != wantValue {
				t.Errorf("POST body %s = %v, want %v", key, got[key], wantValue)
			}
		}
	})

	t.Run("no quotas", func(t *testing.T) {
		realm, secrets := startRealm(t)

		if _, err := NewClient().CreateVolume(t.Context(), CreateVolumeParams{
			Name:       "/pvc-2",
			Parameters: inStorageset(map[string]string{param("recoverypriority"): "10"}),
		}, secrets); err != nil {
			t.Fatalf("CreateVolume() error = %v", err)
		}

		got := realm.lastBody(t, http.MethodPost)
		if got["pathname"] != "/pvc-2" || got["storageset_id"] != json.Number("3") || got["recovery_priority"] != json.Number("10") {
			t.Errorf("POST body = %v, want pathname /pvc-2, storageset 3, recovery_priority 10", got)
		}
		for _, key := range []string{"soft_quota_bytes", "hard_quota_bytes"} {
			if _, ok := got[key]; ok {
				t.Errorf("POST body carries %s for a volume without a quota: %v", key, got)
			}
		}
	})

	errorCases := []struct {
		name    string
		params  map[string]string
		prepare func(*fakeRealm)
		wantErr error
		// wantPost says whether the create request may reach the realm.
		wantPost bool
	}{
		{"existing pathname is a conflict", nil,
			func(r *fakeRealm) { r.addVolume("/pvc-1", 1, 1) }, ErrorAlreadyExist, true},
		{"storageset name that is not assigned to the tenant", map[string]string{param("storageset"): "No Such Set"}, nil, ErrorInvalidArgument, false},
		{"storageset id 0", map[string]string{param("storageset"): "0"}, nil, ErrorInvalidArgument, false},
		{"description parameter", map[string]string{param("description"): "text"}, nil, ErrorInvalidArgument, false},
		{"encryption parameter", map[string]string{param("encryption"): "off"}, nil, ErrorInvalidArgument, false},
		{"legacy recovery parameter", map[string]string{param("recovery"): "10"}, nil, ErrorInvalidArgument, false},
		{"user name instead of uid", map[string]string{param("user"): "alice"}, nil, ErrorInvalidArgument, false},
		{"group name instead of gid", map[string]string{param("group"): "staff"}, nil, ErrorInvalidArgument, false},
		{"non-integer width", map[string]string{param("rgwidth"): "wide"}, nil, ErrorInvalidArgument, false},
		{"bad stripe unit", map[string]string{param("stripeunit"): "64"}, nil, ErrorInvalidArgument, false},
		{"create is forbidden", nil,
			func(r *fakeRealm) { r.failures["POST "+volumesPath] = http.StatusForbidden }, ErrorUnauthenticated, true},
		{"create is rejected", nil,
			func(r *fakeRealm) { r.failures["POST "+volumesPath] = http.StatusBadRequest }, ErrorInvalidArgument, true},
	}
	for _, tc := range errorCases {
		t.Run(tc.name, func(t *testing.T) {
			realm, secrets := startRealm(t)
			if tc.prepare != nil {
				tc.prepare(realm)
			}

			vol, err := NewClient().CreateVolume(t.Context(), CreateVolumeParams{Name: "pvc-1", Parameters: inStorageset(tc.params)}, secrets)
			if !errors.Is(err, tc.wantErr) || vol != nil {
				t.Errorf("CreateVolume() = %v, %v; want nil, %v", vol, err, tc.wantErr)
			}

			posted := false
			for _, req := range realm.recorded() {
				posted = posted || req.Method == http.MethodPost
			}
			if posted != tc.wantPost {
				t.Errorf("create request reached the realm = %v, want %v", posted, tc.wantPost)
			}
		})
	}
}

func TestGetVolume(t *testing.T) {
	realm, secrets := startRealm(t)
	existing := realm.addVolume("/home", 0, 0)
	client := NewClient()

	t.Run("by id", func(t *testing.T) {
		vol, err := client.GetVolume(t.Context(), existing.CSIID(), secrets)
		if err != nil || vol.ID != existing.ID || vol.Name() != "home" {
			t.Errorf("GetVolume(%s) = %+v, %v; want volume /home", existing.CSIID(), vol, err)
		}
	})

	notFound := []string{"4000000000", "pvc-not-an-id", "", "0", "-1", "99999999999"}
	for _, id := range notFound {
		t.Run("not found: "+id, func(t *testing.T) {
			if vol, err := client.GetVolume(t.Context(), id, secrets); !errors.Is(err, ErrorNotFound) || vol != nil {
				t.Errorf("GetVolume(%q) = %v, %v; want nil, ErrorNotFound", id, vol, err)
			}
		})
	}

	byName := []struct {
		name    string
		wantErr error
	}{
		{"home", nil},
		{"/home", nil},
		{"hom", ErrorNotFound},
		{"home/sub", ErrorNotFound},
	}
	for _, tc := range byName {
		t.Run("by name: "+tc.name, func(t *testing.T) {
			vol, err := client.GetVolumeByName(t.Context(), tc.name, secrets)
			if !errors.Is(err, tc.wantErr) || (tc.wantErr == nil && vol.ID != existing.ID) {
				t.Errorf("GetVolumeByName(%q) = %+v, %v; want error %v", tc.name, vol, err, tc.wantErr)
			}
		})
	}
}

func TestExpandVolume(t *testing.T) {
	const gb = int64(1) << 30

	testCases := []struct {
		name               string
		soft, hard, target int64
		wantSoft, wantHard int64
		wantHardInPatch    bool
	}{
		{"hard quota equal to size grows with it", 10 * gb, 10 * gb, 20*gb + 1, 20*gb + 1, 20*gb + 1, true},
		{"larger hard quota is kept", 10 * gb, 50 * gb, 20 * gb, 20 * gb, 50 * gb, false},
		{"unlimited hard quota stays unlimited", 10 * gb, 0, 20 * gb, 20 * gb, 0, false},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			realm, secrets := startRealm(t)
			vol := realm.addVolume("/pvc-1", tc.soft, tc.hard)
			client := NewClient()

			if err := client.ExpandVolume(t.Context(), vol.CSIID(), tc.target, secrets); err != nil {
				t.Fatalf("ExpandVolume() error = %v", err)
			}

			got, err := client.GetVolume(t.Context(), vol.CSIID(), secrets)
			if err != nil || got.SoftQuota() != tc.wantSoft || got.HardQuota() != tc.wantHard {
				t.Errorf("after ExpandVolume(%d): soft %d hard %d, err %v; want soft %d hard %d",
					tc.target, got.SoftQuota(), got.HardQuota(), err, tc.wantSoft, tc.wantHard)
			}
			if _, inPatch := realm.lastBody(t, http.MethodPatch)["hard_quota_bytes"]; inPatch != tc.wantHardInPatch {
				t.Errorf("PATCH carries hard_quota_bytes = %v, want %v", inPatch, tc.wantHardInPatch)
			}
		})
	}

	t.Run("missing volume", func(t *testing.T) {
		_, secrets := startRealm(t)
		if err := NewClient().ExpandVolume(t.Context(), "12345", gb, secrets); !errors.Is(err, ErrorNotFound) {
			t.Errorf("ExpandVolume() error = %v, want ErrorNotFound", err)
		}
	})
}

func TestDeleteVolume(t *testing.T) {
	realm, secrets := startRealm(t)
	vol := realm.addVolume("/pvc-1", 1, 1)
	client := NewClient()

	if err := client.DeleteVolume(t.Context(), vol.CSIID(), secrets); err != nil {
		t.Fatalf("DeleteVolume() error = %v", err)
	}
	if _, err := client.GetVolume(t.Context(), vol.CSIID(), secrets); !errors.Is(err, ErrorNotFound) {
		t.Errorf("GetVolume() after delete error = %v, want ErrorNotFound", err)
	}

	for _, id := range []string{vol.CSIID(), "pvc-1"} {
		if err := client.DeleteVolume(t.Context(), id, secrets); !errors.Is(err, ErrorNotFound) {
			t.Errorf("DeleteVolume(%q) of a missing volume error = %v, want ErrorNotFound", id, err)
		}
	}
}

// TestVolumeNameAsVolumeID covers a statically provisioned volume: its CSI volume ID is the volume name.
func TestVolumeNameAsVolumeID(t *testing.T) {
	const gb = int64(1) << 30

	for _, handle := range []string{"static-volume", "/static-volume"} {
		t.Run(handle, func(t *testing.T) {
			realm, secrets := startRealm(t)
			existing := realm.addVolume("/static-volume", gb, gb)
			realm.addVolume("/other", gb, gb)
			client := NewClient()

			vol, err := client.GetVolume(t.Context(), handle, secrets)
			if err != nil || vol.ID != existing.ID {
				t.Fatalf("GetVolume(%q) = %+v, %v; want volume %d", handle, vol, err, existing.ID)
			}

			if err := client.ExpandVolume(t.Context(), handle, 2*gb, secrets); err != nil {
				t.Fatalf("ExpandVolume(%q) error = %v", handle, err)
			}
			got, err := client.GetVolume(t.Context(), existing.CSIID(), secrets)
			if err != nil || got.SoftQuota() != 2*gb || got.HardQuota() != 2*gb {
				t.Errorf("after ExpandVolume(%q): soft %d hard %d, err %v; want %d", handle, got.SoftQuota(), got.HardQuota(), err, 2*gb)
			}

			if err := client.DeleteVolume(t.Context(), handle, secrets); err != nil {
				t.Fatalf("DeleteVolume(%q) error = %v", handle, err)
			}
			if _, err := client.GetVolume(t.Context(), existing.CSIID(), secrets); !errors.Is(err, ErrorNotFound) {
				t.Errorf("GetVolume() after delete error = %v, want ErrorNotFound", err)
			}
			if _, err := client.GetVolumeByName(t.Context(), "other", secrets); err != nil {
				t.Errorf("the other volume must stay, GetVolumeByName(other) error = %v", err)
			}
		})
	}

	t.Run("a directory inside a volume is not a volume", func(t *testing.T) {
		realm, secrets := startRealm(t)
		realm.addVolume("/static-volume", gb, gb)
		client := NewClient()

		handle := "/static-volume/dir1"
		if err := client.ExpandVolume(t.Context(), handle, 2*gb, secrets); !errors.Is(err, ErrorNotFound) {
			t.Errorf("ExpandVolume(%q) error = %v, want ErrorNotFound", handle, err)
		}
		if err := client.DeleteVolume(t.Context(), handle, secrets); !errors.Is(err, ErrorNotFound) {
			t.Errorf("DeleteVolume(%q) error = %v, want ErrorNotFound", handle, err)
		}
	})

	t.Run("ParseVolumeID", func(t *testing.T) {
		for handle, want := range map[string]bool{"8": true, "08": true, "0": false, "/8": false, "pvc-8": false, "": false, "-1": false} {
			if _, got := ParseVolumeID(handle); got != want {
				t.Errorf("ParseVolumeID(%q) = %v, want %v", handle, got, want)
			}
		}
	})
}

func TestAuthentication(t *testing.T) {
	realm, secrets := startRealm(t)
	realm.addVolume("/home", 0, 0)
	client := NewClient()

	if _, err := client.GetVolumeByName(t.Context(), "home", secrets); err != nil {
		t.Fatalf("GetVolumeByName() error = %v", err)
	}
	if got := realm.recorded()[0].Auth; got != "Bearer "+testToken {
		t.Errorf("Authorization header = %q, want the bearer token", got)
	}

	wrong := map[string]string{}
	for key, value := range secrets {
		wrong[key] = value
	}
	wrong[utils.RealmConnectionContext.Token] = "wrong-token-value"
	_, err := client.GetVolumeByName(t.Context(), "home", wrong)
	if !errors.Is(err, ErrorUnauthenticated) {
		t.Errorf("GetVolumeByName() with a wrong token error = %v, want ErrorUnauthenticated", err)
	}
	if err != nil && strings.Contains(err.Error(), "wrong-token-value") {
		t.Errorf("error text leaks the token: %v", err)
	}
}

func TestErrorFromResponse(t *testing.T) {
	testCases := []struct {
		status     int
		body       string
		wantErr    error
		wantDetail string
	}{
		{http.StatusBadRequest, `{"detail":"Invalid parameter value"}`, ErrorInvalidArgument, "Invalid parameter value"},
		{http.StatusUnprocessableEntity, `{"detail":[{"type":"int_parsing","msg":"bad"}]}`, ErrorInvalidArgument, `"int_parsing"`},
		{http.StatusUnauthorized, `{"detail":"Not authenticated"}`, ErrorUnauthenticated, "Not authenticated"},
		{http.StatusForbidden, `{"detail":"Forbidden"}`, ErrorUnauthenticated, "Forbidden"},
		{http.StatusNotFound, `{"detail":"Volume not found"}`, ErrorNotFound, "Volume not found"},
		{http.StatusConflict, `{"detail":"Volume already exists"}`, ErrorAlreadyExist, "Volume already exists"},
		{http.StatusServiceUnavailable, `{"detail":"retry"}`, ErrorUnavailable, "retry"},
		{http.StatusGatewayTimeout, `<html>gateway timeout</html>`, ErrorUnavailable, "<html>gateway timeout</html>"},
		{http.StatusInternalServerError, ``, ErrorInternal, "HTTP 500"},
		{http.StatusTeapot, `{"other":"shape"}`, ErrorInternal, `{"other":"shape"}`},
	}
	for _, tc := range testCases {
		t.Run(strconv.Itoa(tc.status), func(t *testing.T) {
			err := errorFromResponse(tc.status, []byte(tc.body))
			if !errors.Is(err, tc.wantErr) || !strings.Contains(err.Error(), tc.wantDetail) {
				t.Errorf("errorFromResponse(%d, %q) = %v; want %v with detail %q", tc.status, tc.body, err, tc.wantErr, tc.wantDetail)
			}
		})
	}
}

func TestEndpointFromSecrets(t *testing.T) {
	keys := utils.RealmConnectionContext

	testCases := []struct {
		name    string
		secrets map[string]string
		wantURL string
		wantErr bool
	}{
		{"bare host means HTTPS", map[string]string{keys.API: "realm.example.com", keys.Token: "t"}, "https://realm.example.com", false},
		{"bare host with port", map[string]string{keys.API: "realm:8443", keys.Token: "t"}, "https://realm:8443", false},
		{"explicit HTTPS URL with port", map[string]string{keys.API: "https://api.example.com:8443/", keys.Token: "t"}, "https://api.example.com:8443", false},
		{"explicit plain HTTP is allowed", map[string]string{keys.API: "http://10.0.0.1:8000", keys.Token: "t"}, "http://10.0.0.1:8000", false},
		{"missing token", map[string]string{keys.API: "realm"}, "", true},
		{"empty token", map[string]string{keys.API: "realm", keys.Token: ""}, "", true},
		{"no address at all", map[string]string{keys.Token: "t"}, "", true},
		{"unsupported scheme", map[string]string{keys.API: "ftp://realm", keys.Token: "t"}, "", true},
		{"URL with a path", map[string]string{keys.API: "https://realm/api/v1", keys.Token: "t"}, "", true},
		{"URL with credentials", map[string]string{keys.API: "https://user:pass@realm", keys.Token: "t"}, "", true},
		{"URL without a host", map[string]string{keys.API: "https://", keys.Token: "t"}, "", true},
		{"nil secrets", nil, "", true},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ep, err := endpointFromSecrets(tc.secrets)
			if (err != nil) != tc.wantErr || ep.baseURL != tc.wantURL {
				t.Errorf("endpointFromSecrets(%v) = %q, %v; want %q, wantErr %v", tc.secrets, ep.baseURL, err, tc.wantURL, tc.wantErr)
			}
			if err != nil && !errors.Is(err, ErrorInvalidArgument) {
				t.Errorf("endpointFromSecrets(%v) error = %v, want ErrorInvalidArgument", tc.secrets, err)
			}
		})
	}
}

func TestTLS(t *testing.T) {
	realm := newFakeRealm()
	realm.addVolume("/home", 0, 0)
	server := httptest.NewTLSServer(realm)
	t.Cleanup(server.Close)

	caCert := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}))
	secrets := func(ca string) map[string]string {
		s := map[string]string{
			utils.RealmConnectionContext.Token: testToken,
			utils.RealmConnectionContext.API:   server.URL,
		}
		if ca != "" {
			s[utils.RealmConnectionContext.CACert] = ca
		}
		return s
	}

	t.Run("realm certificate verified against ca_cert", func(t *testing.T) {
		if _, err := NewClient().GetVolumeByName(t.Context(), "home", secrets(caCert)); err != nil {
			t.Errorf("GetVolumeByName() error = %v, want success with the realm's CA", err)
		}
	})
	t.Run("unknown certificate is refused without ca_cert", func(t *testing.T) {
		_, err := NewClient().GetVolumeByName(t.Context(), "home", secrets(""))
		if !errors.Is(err, ErrorUnavailable) {
			t.Errorf("GetVolumeByName() error = %v, want ErrorUnavailable for an untrusted certificate", err)
		}
		// The error tells the reader which keys of the Secret solve it.
		for _, want := range []string{"Hint:", `"ca_cert"`, `"insecure_skip_tls_verify"`} {
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Errorf("GetVolumeByName() error = %v, want it to contain %s", err, want)
			}
		}
	})
	t.Run("another connection failure has no certificate hint", func(t *testing.T) {
		s := secrets("")
		s[utils.RealmConnectionContext.API] = "https://127.0.0.1:1"
		_, err := NewClient().GetVolumeByName(t.Context(), "home", s)
		if !errors.Is(err, ErrorUnavailable) || strings.Contains(err.Error(), "Hint:") {
			t.Errorf("GetVolumeByName() error = %v, want ErrorUnavailable without a hint", err)
		}
	})
	t.Run("unknown certificate is accepted with insecure_skip_tls_verify", func(t *testing.T) {
		s := secrets("")
		s[utils.RealmConnectionContext.InsecureSkipTLSVerify] = "true"
		if _, err := NewClient().GetVolumeByName(t.Context(), "home", s); err != nil {
			t.Errorf("GetVolumeByName() error = %v, want success without verification", err)
		}
	})
	t.Run("insecure_skip_tls_verify=false keeps the verification", func(t *testing.T) {
		s := secrets("")
		s[utils.RealmConnectionContext.InsecureSkipTLSVerify] = "false"
		if _, err := NewClient().GetVolumeByName(t.Context(), "home", s); !errors.Is(err, ErrorUnavailable) {
			t.Errorf("GetVolumeByName() error = %v, want ErrorUnavailable for an untrusted certificate", err)
		}
	})
	t.Run("insecure_skip_tls_verify that is not a boolean", func(t *testing.T) {
		s := secrets(caCert)
		s[utils.RealmConnectionContext.InsecureSkipTLSVerify] = "maybe"
		if _, err := NewClient().GetVolumeByName(t.Context(), "home", s); !errors.Is(err, ErrorInvalidArgument) {
			t.Errorf("GetVolumeByName() error = %v, want ErrorInvalidArgument", err)
		}
	})
	t.Run("ca_cert that is not PEM", func(t *testing.T) {
		if _, err := NewClient().GetVolumeByName(t.Context(), "home", secrets("not a certificate")); !errors.Is(err, ErrorInvalidArgument) {
			t.Errorf("GetVolumeByName() error = %v, want ErrorInvalidArgument", err)
		}
	})
}

func TestDeadlineAndConnectionFailures(t *testing.T) {
	t.Run("caller deadline cancels the call", func(t *testing.T) {
		release := make(chan struct{})
		server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-release }))
		t.Cleanup(server.Close)
		t.Cleanup(func() { close(release) })

		ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
		defer cancel()

		start := time.Now()
		_, err := NewClient().GetVolume(ctx, "1", map[string]string{
			utils.RealmConnectionContext.Token: testToken,
			utils.RealmConnectionContext.API:   server.URL,
		})
		if !errors.Is(err, ErrorUnavailable) {
			t.Errorf("GetVolume() error = %v, want ErrorUnavailable", err)
		}
		if elapsed := time.Since(start); elapsed > 5*time.Second {
			t.Errorf("GetVolume() returned after %v; the caller's deadline was not honoured", elapsed)
		}
	})

	t.Run("realm is down", func(t *testing.T) {
		server := httptest.NewServer(http.NotFoundHandler())
		server.Close()

		_, err := NewClient().GetVolume(t.Context(), "1", map[string]string{
			utils.RealmConnectionContext.Token: testToken,
			utils.RealmConnectionContext.API:   server.URL,
		})
		if !errors.Is(err, ErrorUnavailable) {
			t.Errorf("GetVolume() error = %v, want ErrorUnavailable", err)
		}
	})

	t.Run("response that is not JSON", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("<html>login page</html>"))
		}))
		t.Cleanup(server.Close)

		_, err := NewClient().GetVolume(t.Context(), "1", map[string]string{
			utils.RealmConnectionContext.Token: testToken,
			utils.RealmConnectionContext.API:   server.URL,
		})
		if !errors.Is(err, ErrorInternal) {
			t.Errorf("GetVolume() error = %v, want ErrorInternal", err)
		}
	})
}

// TestFakeClientMatchesClient runs the same scenario against Client and FakeClient,
// so csi-sanity, which uses the fake, exercises the contract the real client has.
func TestFakeClientMatchesClient(t *testing.T) {
	type backend interface {
		CreateVolume(ctx context.Context, p CreateVolumeParams, secrets map[string]string) (*Volume, error)
		GetVolume(ctx context.Context, volumeID string, secrets map[string]string) (*Volume, error)
		GetVolumeByName(ctx context.Context, name string, secrets map[string]string) (*Volume, error)
		ExpandVolume(ctx context.Context, volumeID string, sizeBytes int64, secrets map[string]string) error
		DeleteVolume(ctx context.Context, volumeID string, secrets map[string]string) error
	}

	_, secrets := startRealm(t)

	backends := map[string]backend{"Client": NewClient(), "FakeClient": NewFakeClient()}
	for name, b := range backends {
		t.Run(name, func(t *testing.T) {
			ctx := t.Context()
			const size = int64(5368709120)

			if _, err := b.GetVolumeByName(ctx, "pvc-a", secrets); !errors.Is(err, ErrorNotFound) {
				t.Fatalf("GetVolumeByName() before create error = %v, want ErrorNotFound", err)
			}

			vol, err := b.CreateVolume(ctx, CreateVolumeParams{Name: "pvc-a", Parameters: inStorageset(nil), SoftQuotaBytes: size, HardQuotaBytes: size}, secrets)
			if err != nil || vol.Name() != "pvc-a" || vol.SoftQuota() != size || vol.HardQuota() != size {
				t.Fatalf("CreateVolume() = %+v, %v", vol, err)
			}

			if _, err := b.CreateVolume(ctx, CreateVolumeParams{Name: "/pvc-a", Parameters: inStorageset(nil), SoftQuotaBytes: size}, secrets); !errors.Is(err, ErrorAlreadyExist) {
				t.Errorf("second CreateVolume() error = %v, want ErrorAlreadyExist", err)
			}
			if _, err := b.CreateVolume(ctx, CreateVolumeParams{Name: "pvc-b", Parameters: inStorageset(map[string]string{param("user"): "bob"})}, secrets); !errors.Is(err, ErrorInvalidArgument) {
				t.Errorf("CreateVolume() with a user name error = %v, want ErrorInvalidArgument", err)
			}

			byName, err := b.GetVolumeByName(ctx, "pvc-a", secrets)
			if err != nil || byName.ID != vol.ID {
				t.Errorf("GetVolumeByName() = %+v, %v; want id %d", byName, err, vol.ID)
			}

			if err := b.ExpandVolume(ctx, vol.CSIID(), 2*size, secrets); err != nil {
				t.Errorf("ExpandVolume() error = %v", err)
			}
			grown, err := b.GetVolume(ctx, vol.CSIID(), secrets)
			if err != nil || grown.SoftQuota() != 2*size || grown.HardQuota() != 2*size {
				t.Errorf("after expand: %+v, %v; want both quotas %d", grown, err, 2*size)
			}

			if err := b.DeleteVolume(ctx, vol.CSIID(), secrets); err != nil {
				t.Errorf("DeleteVolume() error = %v", err)
			}
			for _, id := range []string{vol.CSIID(), "not-an-id"} {
				if err := b.DeleteVolume(ctx, id, secrets); !errors.Is(err, ErrorNotFound) {
					t.Errorf("DeleteVolume(%q) of a missing volume error = %v, want ErrorNotFound", id, err)
				}
				if err := b.ExpandVolume(ctx, id, size, secrets); !errors.Is(err, ErrorNotFound) {
					t.Errorf("ExpandVolume(%q) of a missing volume error = %v, want ErrorNotFound", id, err)
				}
				if _, err := b.GetVolume(ctx, id, secrets); !errors.Is(err, ErrorNotFound) {
					t.Errorf("GetVolume(%q) of a missing volume error = %v, want ErrorNotFound", id, err)
				}
			}
		})
	}
}

// TestCreateVolumeStorageset covers the optional storageset parameter: an id, a name, or nothing.
func TestCreateVolumeStorageset(t *testing.T) {
	testCases := []struct {
		name string
		// params is nil when the StorageClass has no storageset parameter.
		params map[string]string
		wantID int64
		// wantLookup says whether the client must read the storagesets of the tenant.
		wantLookup bool
	}{
		{"a number is the id", map[string]string{StoragesetParameter: "5"}, 5, false},
		{"a name is looked up", map[string]string{StoragesetParameter: "Fast Set"}, 7, true},
		{"without the parameter the first assigned storageset is used", nil, 3, true},
		{"an empty value is the same as no parameter", map[string]string{StoragesetParameter: " "}, 3, true},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			realm, secrets := startRealm(t)

			vol, err := NewClient().CreateVolume(t.Context(), CreateVolumeParams{Name: "pvc-1", Parameters: tc.params}, secrets)
			if err != nil || vol.StoragesetID != tc.wantID {
				t.Fatalf("CreateVolume() = %+v, %v; want storageset %d", vol, err, tc.wantID)
			}

			lookedUp := false
			for _, request := range realm.recorded() {
				if strings.HasSuffix(request.Path, "/storagesets") {
					lookedUp = true
				}
			}
			if lookedUp != tc.wantLookup {
				t.Errorf("storageset lookup = %v, want %v", lookedUp, tc.wantLookup)
			}
		})
	}

	t.Run("no storageset is assigned to the tenant", func(t *testing.T) {
		realm, secrets := startRealm(t)
		realm.storagesets = nil

		vol, err := NewClient().CreateVolume(t.Context(), CreateVolumeParams{Name: "pvc-1"}, secrets)
		if !errors.Is(err, ErrorInvalidArgument) || vol != nil {
			t.Errorf("CreateVolume() = %v, %v; want nil, ErrorInvalidArgument", vol, err)
		}
	})

	t.Run("the fake client keeps a numeric id and has one storageset", func(t *testing.T) {
		fake := NewFakeClient()
		byID, err := fake.CreateVolume(t.Context(), CreateVolumeParams{Name: "pvc-1", Parameters: map[string]string{StoragesetParameter: "5"}}, nil)
		if err != nil || byID.StoragesetID != 5 {
			t.Errorf("CreateVolume(storageset 5) = %+v, %v", byID, err)
		}
		byDefault, err := fake.CreateVolume(t.Context(), CreateVolumeParams{Name: "pvc-2"}, nil)
		if err != nil || byDefault.StoragesetID != 1 {
			t.Errorf("CreateVolume(no storageset) = %+v, %v", byDefault, err)
		}
	})
}

func TestAPIHost(t *testing.T) {
	key := utils.RealmConnectionContext.API
	cases := []struct {
		api     string
		want    string
		wantErr bool
	}{
		{"realm.example.com", "realm.example.com", false},
		{"realm.example.com:8443", "realm.example.com", false},
		{"http://10.0.0.5:8000", "10.0.0.5", false},
		{"https://realm.example.com/", "realm.example.com", false},
		{"https://[fd00::5]:8443", "[fd00::5]", false},
		{"", "", true},
		{"ftp://realm", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.api, func(t *testing.T) {
			got, err := APIHost(map[string]string{key: tc.api})
			if got != tc.want || (err != nil) != tc.wantErr {
				t.Errorf("APIHost(%q) = %q, %v; want %q, error %v", tc.api, got, err, tc.want, tc.wantErr)
			}
		})
	}
}

func TestMountSource(t *testing.T) {
	cases := []struct {
		rmlist     string
		wantHost   string
		wantOption string
		wantErr    bool
	}{
		{"192.168.0.2", "192.168.0.2", "rmlist=(192.168.0.2)", false},
		{" director.tenant.example.com ", "director.tenant.example.com", "rmlist=(director.tenant.example.com)", false},
		{"192.168.0.2,192.168.0.54, 192.168.0.55", "192.168.0.2", "rmlist=(192.168.0.2,192.168.0.54,192.168.0.55)", false},
		{"fd00::2", "[fd00::2]", "rmlist=(fd00::2)", false},
		{"", "", "", true},
		{"192.168.0.2,", "", "", true},
		{"192.168.0.2/volume", "", "", true},
		{"192.168.0.2 192.168.0.3", "", "", true},
		{"192.168.0.2),ro,(x", "", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.rmlist, func(t *testing.T) {
			host, option, err := MountSource(tc.rmlist)
			if host != tc.wantHost || option != tc.wantOption || (err != nil) != tc.wantErr {
				t.Errorf("MountSource(%q) = %q, %q, %v; want %q, %q, error %v",
					tc.rmlist, host, option, err, tc.wantHost, tc.wantOption, tc.wantErr)
			}
		})
	}
}

func TestGetMountAddress(t *testing.T) {
	t.Run("tenant without a subnet", func(t *testing.T) {
		_, secrets := startRealm(t)
		got, err := NewClient().GetMountAddress(t.Context(), secrets)
		if got != "" || err != nil {
			t.Errorf("GetMountAddress() = %q, %v; want \"\", nil", got, err)
		}
	})

	t.Run("rmlist of the tenant subnet", func(t *testing.T) {
		realm, secrets := startRealm(t)
		realm.addSubnet("subnet-1", "192.168.0.2")
		realm.addSubnet("subnet-2", "192.168.9.9")

		got, err := NewClient().GetMountAddress(t.Context(), secrets)
		if got != "192.168.0.2" || err != nil {
			t.Errorf("GetMountAddress() = %q, %v; want the rmlist of the first subnet", got, err)
		}
	})

	t.Run("token may not read the subnets", func(t *testing.T) {
		realm, secrets := startRealm(t)
		realm.addSubnet("subnet-1", "192.168.0.2")
		realm.failures[http.MethodGet+" "+subnetsPath] = http.StatusForbidden

		got, err := NewClient().GetMountAddress(t.Context(), secrets)
		if got != "" || err == nil {
			t.Errorf("GetMountAddress() = %q, %v; want an error", got, err)
		}
	})

	t.Run("token may not read the mount recipe", func(t *testing.T) {
		realm, secrets := startRealm(t)
		realm.addSubnet("subnet-1", "192.168.0.2")
		realm.failures[http.MethodGet+" "+subnetsPath+"/subnet-1/mount-info"] = http.StatusForbidden

		got, err := NewClient().GetMountAddress(t.Context(), secrets)
		if got != "" || err == nil {
			t.Errorf("GetMountAddress() = %q, %v; want an error", got, err)
		}
	})
}
