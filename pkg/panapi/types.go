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
	"strconv"
	"strings"

	"github.com/panasasinc/panfs-container-storage-interface-oss/pkg/utils"
)

// Volume is a PanFS volume as PanAPI reports it.
type Volume struct {
	ID             int64  `json:"id"`
	Pathname       string `json:"pathname"`
	Status         string `json:"status"`
	StoragesetID   int64  `json:"storageset_id"`
	SoftQuotaBytes *int64 `json:"soft_quota_bytes"`
	HardQuotaBytes *int64 `json:"hard_quota_bytes"`
}

// CSIID returns the CSI volume ID: the PanAPI volume id in decimal form.
func (v *Volume) CSIID() string {
	return strconv.FormatInt(v.ID, 10)
}

// Name returns the volume name: the pathname without the leading slash.
func (v *Volume) Name() string {
	return strings.TrimPrefix(v.Pathname, "/")
}

// SoftQuota returns the soft quota in bytes; 0 means no soft limit.
func (v *Volume) SoftQuota() int64 {
	if v.SoftQuotaBytes == nil {
		return 0
	}
	return *v.SoftQuotaBytes
}

// HardQuota returns the hard quota in bytes; 0 means no hard limit.
func (v *Volume) HardQuota() int64 {
	if v.HardQuotaBytes == nil {
		return 0
	}
	return *v.HardQuotaBytes
}

// VolumeContext returns the CSI volume context of the volume.
// The node plugin mounts the volume by name, so the name travels in the context.
func (v *Volume) VolumeContext() map[string]string {
	return map[string]string{
		utils.VolumeNameContextKey: v.Name(),
	}
}

// CreateVolumeParams holds what the CSI controller asks for in CreateVolume.
type CreateVolumeParams struct {
	// Name is the volume name; a leading slash is optional.
	Name string
	// Parameters are the StorageClass parameters, with their vendor prefix.
	Parameters map[string]string
	// SoftQuotaBytes and HardQuotaBytes are exact byte counts; 0 means no limit.
	SoftQuotaBytes int64
	HardQuotaBytes int64
}

// volumeCreateRequest is the body of POST /api/v1/volumes.
type volumeCreateRequest struct {
	Pathname         string `json:"pathname"`
	StoragesetID     int64  `json:"storageset_id"`
	RaidType         string `json:"raid_type,omitempty"`
	RecoveryPriority *int64 `json:"recovery_priority,omitempty"`
	EfsaMode         string `json:"efsa_mode,omitempty"`
	SoftQuotaBytes   *int64 `json:"soft_quota_bytes,omitempty"`
	HardQuotaBytes   *int64 `json:"hard_quota_bytes,omitempty"`
	VolumeServiceID  string `json:"volume_service_id,omitempty"`
	MaxWidth         *int64 `json:"max_width,omitempty"`
	StripeUnitBytes  *int64 `json:"stripe_unit_bytes,omitempty"`
	RGWidth          *int64 `json:"rg_width,omitempty"`
	RGDepth          *int64 `json:"rg_depth,omitempty"`
	User             string `json:"user,omitempty"`
	Group            string `json:"group,omitempty"`
	UserPermissions  string `json:"user_permissions,omitempty"`
	GroupPermissions string `json:"group_permissions,omitempty"`
	OtherPermissions string `json:"other_permissions,omitempty"`
}

// volumePatchRequest is the body of PATCH /api/v1/volumes/{id}.
type volumePatchRequest struct {
	SoftQuotaBytes *int64 `json:"soft_quota_bytes,omitempty"`
	HardQuotaBytes *int64 `json:"hard_quota_bytes,omitempty"`
}

type volumeCreateResponse struct {
	ID int64 `json:"id"`
}

type volumeGetResponse struct {
	Volume Volume `json:"volume"`
}

type volumeListResponse struct {
	Volumes []Volume `json:"volumes"`
}

// authMeResponse is the body of GET /api/v1/auth/me; only the tenant of the token is used.
type authMeResponse struct {
	TenantID string `json:"tenant_id"`
}

// tenantStoragesetsResponse is the body of GET /api/v1/tenants/{tenant_id}/storagesets.
type tenantStoragesetsResponse struct {
	Storagesets []struct {
		ID   int64  `json:"storageset_id"`
		Name string `json:"storageset_name"`
	} `json:"storagesets"`
}

// subnetListResponse is the body of GET /api/v1/subnets; only the id is used.
type subnetListResponse struct {
	Subnets []struct {
		ID string `json:"id"`
	} `json:"subnets"`
}

// mountInfoResponse is the body of GET /api/v1/subnets/{subnet_id}/mount-info; only the rmlist is used.
type mountInfoResponse struct {
	RMList string `json:"rmlist"`
}
