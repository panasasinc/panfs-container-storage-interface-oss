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

// Package panapi is a client for the PanFS REST API (PanAPI).
// It covers the routes the CSI controller needs: volume create, list, get, patch and delete.
// Every call authenticates with a service account token taken from the StorageClass secret.
package panapi

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/panasasinc/panfs-container-storage-interface-oss/pkg/utils"
	"k8s.io/klog/v2"
)

const (
	// requestTimeout caps one PanAPI call; a shorter deadline on the caller's context wins.
	requestTimeout = 30 * time.Second

	// maxResponseBytes bounds how much of a response body the client reads.
	maxResponseBytes = 16 << 20

	volumesPath = "/api/v1/volumes"
)

var llog klog.Logger = klog.NewKlogr()

// Client talks to PanAPI on behalf of the CSI controller.
// The realm address and the credentials come from the secrets of each call, so one Client serves many realms.
type Client struct {
	mu sync.Mutex
	// httpClients caches one HTTP client per CA certificate, so connections are reused.
	httpClients map[string]*http.Client
}

// NewClient returns a Client ready for use.
func NewClient() *Client {
	return &Client{httpClients: make(map[string]*http.Client)}
}

// endpoint is where and how to reach one realm.
type endpoint struct {
	baseURL string
	token   string
	caCert  string
}

// APIURL returns the PanAPI base URL a StorageClass secret names in its api key.
// A bare host, with or without a port, means HTTPS; a full URL may use https:// or http://.
func APIURL(secrets map[string]string) (*url.URL, error) {
	key := utils.RealmConnectionContext.API

	value := secrets[key]
	if value == "" {
		return nil, fmt.Errorf("missing %s in secrets", key)
	}
	if !strings.Contains(value, "://") {
		value = "https://" + value
	}

	parsed, err := url.Parse(value)
	if err != nil || parsed.Hostname() == "" {
		return nil, fmt.Errorf("%s in secrets is not a host or a URL", key)
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return nil, fmt.Errorf("%s in secrets must use https:// or http://", key)
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	if parsed.Path != "" || parsed.RawQuery != "" || parsed.User != nil {
		return nil, fmt.Errorf("%s in secrets must be a host or a URL without a path", key)
	}
	return parsed, nil
}

// MountAddress returns the address the node mounts volumes from: the host of the api key in the secret.
func MountAddress(secrets map[string]string) (string, error) {
	parsed, err := APIURL(secrets)
	if err != nil {
		return "", err
	}
	host := parsed.Hostname()
	if strings.Contains(host, ":") {
		return "[" + host + "]", nil // an IPv6 address needs brackets inside a panfs:// URL
	}
	return host, nil
}

// endpointFromSecrets reads the PanAPI address and the credentials from a StorageClass secret.
func endpointFromSecrets(secrets map[string]string) (endpoint, error) {
	keys := utils.RealmConnectionContext

	token := secrets[keys.Token]
	if token == "" {
		return endpoint{}, fmt.Errorf("%w: missing %s in secrets", ErrorInvalidArgument, keys.Token)
	}

	parsed, err := APIURL(secrets)
	if err != nil {
		return endpoint{}, fmt.Errorf("%w: %v", ErrorInvalidArgument, err)
	}
	if parsed.Scheme == "http" {
		llog.Info("WARNING: PanAPI is reached over plain HTTP; the API token crosses the network in clear text",
			"host", parsed.Host)
	}

	return endpoint{baseURL: parsed.String(), token: token, caCert: secrets[keys.CACert]}, nil
}

// httpClient returns the cached HTTP client for a CA certificate, creating it on first use.
// An empty certificate means the system trust store.
func (c *Client) httpClient(caCert string) (*http.Client, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if hc, ok := c.httpClients[caCert]; ok {
		return hc, nil
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	if caCert != "" {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM([]byte(caCert)) {
			return nil, fmt.Errorf("%w: %s holds no PEM certificate",
				ErrorInvalidArgument, utils.RealmConnectionContext.CACert)
		}
		transport.TLSClientConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
	}

	hc := &http.Client{Transport: transport}
	c.httpClients[caCert] = hc
	return hc, nil
}

// do sends one authenticated request and decodes a JSON response into out when out is not nil.
func (c *Client) do(ctx context.Context, secrets map[string]string, method, path string, in, out any) error {
	ep, err := endpointFromSecrets(secrets)
	if err != nil {
		return err
	}

	hc, err := c.httpClient(ep.caCert)
	if err != nil {
		return err
	}

	var body io.Reader
	if in != nil {
		payload, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("%w: encode request: %v", ErrorInternal, err)
		}
		body = bytes.NewReader(payload)
	}

	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, method, ep.baseURL+path, body)
	if err != nil {
		return fmt.Errorf("%w: build request: %v", ErrorInternal, err)
	}
	req.Header.Set("Authorization", "Bearer "+ep.token)
	req.Header.Set("Accept", "application/json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	llog.V(5).Info("PanAPI request", "method", method, "path", path)
	resp, err := hc.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %s %s: %v", ErrorUnavailable, method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return fmt.Errorf("%w: read response of %s %s: %v", ErrorUnavailable, method, path, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return errorFromResponse(resp.StatusCode, data)
	}

	if out == nil {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("%w: decode response of %s %s: %v", ErrorInternal, method, path, err)
	}
	return nil
}

// ParseVolumeID reports whether a CSI volume ID is a PanAPI volume id, and returns the id.
// The driver gives every volume it creates its PanAPI volume id as the CSI volume ID.
// A statically provisioned volume carries the volume name instead; a name that consists of digits only
// must be written with a leading slash, so that it is not taken for an id.
func ParseVolumeID(volumeID string) (int64, bool) {
	id, err := strconv.ParseUint(volumeID, 10, 32)
	if err != nil || id == 0 {
		return 0, false
	}
	return int64(id), true
}

// volumePath returns the route of the volume with the given PanAPI volume id.
func volumePath(id int64) string {
	return volumesPath + "/" + strconv.FormatInt(id, 10)
}

// CreateVolume creates a volume and returns it as PanAPI stores it.
// It returns ErrorAlreadyExist when PanAPI reports a conflict; the caller decides whether the volume really exists.
func (c *Client) CreateVolume(ctx context.Context, p CreateVolumeParams, secrets map[string]string) (*Volume, error) {
	req, err := buildCreateRequest(p)
	if err != nil {
		return nil, err
	}

	var created volumeCreateResponse
	if err := c.do(ctx, secrets, http.MethodPost, volumesPath, req, &created); err != nil {
		return nil, err
	}

	return c.GetVolume(ctx, strconv.FormatInt(created.ID, 10), secrets)
}

// GetVolume returns the volume with the given CSI volume ID, or ErrorNotFound.
// The CSI volume ID is a PanAPI volume id, or the volume name for a statically provisioned volume.
func (c *Client) GetVolume(ctx context.Context, volumeID string, secrets map[string]string) (*Volume, error) {
	id, ok := ParseVolumeID(volumeID)
	if !ok {
		return c.GetVolumeByName(ctx, volumeID, secrets)
	}

	var got volumeGetResponse
	if err := c.do(ctx, secrets, http.MethodGet, volumePath(id), nil, &got); err != nil {
		return nil, err
	}
	return &got.Volume, nil
}

// GetVolumeByName returns the volume with the given name, or ErrorNotFound.
// PanAPI has no lookup by name, so the client lists the volumes and matches the pathname.
func (c *Client) GetVolumeByName(ctx context.Context, name string, secrets map[string]string) (*Volume, error) {
	var list volumeListResponse
	if err := c.do(ctx, secrets, http.MethodGet, volumesPath, nil, &list); err != nil {
		return nil, err
	}

	pathname := "/" + strings.TrimPrefix(name, "/")
	for i := range list.Volumes {
		if list.Volumes[i].Pathname == pathname {
			return &list.Volumes[i], nil
		}
	}
	return nil, fmt.Errorf("%w: no volume with pathname %s", ErrorNotFound, pathname)
}

// ExpandVolume raises the soft quota of a volume to sizeBytes.
// A hard quota smaller than the new size is raised with it; an unlimited hard quota stays unlimited.
func (c *Client) ExpandVolume(ctx context.Context, volumeID string, sizeBytes int64, secrets map[string]string) error {
	vol, err := c.GetVolume(ctx, volumeID, secrets)
	if err != nil {
		return err
	}

	patch := volumePatchRequest{SoftQuotaBytes: &sizeBytes}
	if hard := vol.HardQuota(); hard > 0 && hard < sizeBytes {
		patch.HardQuotaBytes = &sizeBytes
	}

	return c.do(ctx, secrets, http.MethodPatch, volumePath(vol.ID), patch, nil)
}

// DeleteVolume deletes the volume with the given CSI volume ID.
// It returns ErrorNotFound when the volume does not exist.
func (c *Client) DeleteVolume(ctx context.Context, volumeID string, secrets map[string]string) error {
	id, ok := ParseVolumeID(volumeID)
	if !ok {
		vol, err := c.GetVolumeByName(ctx, volumeID, secrets)
		if err != nil {
			return err
		}
		id = vol.ID
	}
	return c.do(ctx, secrets, http.MethodDelete, volumePath(id), nil, nil)
}
