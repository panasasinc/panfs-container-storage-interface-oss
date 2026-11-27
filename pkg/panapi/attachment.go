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
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// Segment types of a tenant subnet. The realm owns the choice.
const (
	// SegmentTypeVXLAN is a VXLAN overlay: the segment id is a VNI, and the attachment has a tunnel recipe.
	SegmentTypeVXLAN = "vxlan"
	// SegmentTypeVLAN is an 802.1Q VLAN: the segment id is a VLAN id, and the attachment has no tunnel recipe.
	SegmentTypeVLAN = "vlan"
)

// AttachmentTunnel is the tunnel recipe of an attachment. PanAPI sends it only for a tunnelled segment.
type AttachmentTunnel struct {
	// MAC is the link-layer address the host must set on its tenant interface.
	MAC string `json:"mac"`
	// UDPPort is the UDP port that carries the encapsulation.
	UDPPort int `json:"udp_port"`
	// VTEPs are the underlay addresses of the realm nodes; they are the flood list of the host.
	VTEPs []string `json:"vteps"`
}

// Attachment is the record of one host on a tenant subnet: everything the host needs to build its interface.
type Attachment struct {
	ID            string            `json:"id"`
	SubnetID      string            `json:"subnet_id"`
	UnderlayIP    string            `json:"underlay_ip"`
	Hostname      string            `json:"hostname"`
	OverlayIP     string            `json:"overlay_ip"`
	OverlayPrefix int               `json:"overlay_prefix"`
	MTU           int               `json:"mtu"`
	SegmentType   string            `json:"segment_type"`
	SegmentID     int               `json:"segment_id"`
	Tunnel        *AttachmentTunnel `json:"tunnel"`
}

// TenantNetwork is what a host needs to join the network of the token's tenant.
type TenantNetwork struct {
	// TenantName is the name of the tenant that owns the token.
	TenantName string
	// SubnetID is the id of the tenant subnet.
	SubnetID string
	// Directors are the addresses of the realm directors on the subnet that a client mounts from.
	Directors []string
}

// attachmentCreateRequest is the body of POST /api/v1/subnets/{subnet_id}/attachments.
type attachmentCreateRequest struct {
	UnderlayIP string `json:"underlay_ip"`
	Hostname   string `json:"hostname,omitempty"`
}

// attachmentListResponse is the body of GET /api/v1/subnets/{subnet_id}/attachments.
type attachmentListResponse struct {
	Attachments []Attachment `json:"attachments"`
}

// tenantGetResponse is the body of GET /api/v1/tenants/{tenant_id}; only the name is used.
type tenantGetResponse struct {
	Tenant struct {
		Name string `json:"name"`
	} `json:"tenant"`
}

// SplitRMList splits the rmlist of a mount recipe into its addresses. PanAPI joins them with ";";
// a comma is accepted too.
func SplitRMList(rmlist string) []string {
	var addresses []string
	for _, address := range strings.FieldsFunc(rmlist, func(r rune) bool { return r == ';' || r == ',' }) {
		if address = strings.TrimSpace(address); address != "" {
			addresses = append(addresses, address)
		}
	}
	return addresses
}

// GetTenantNetwork returns the tenant of the token, its subnet, and the director addresses on that subnet.
func (c *Client) GetTenantNetwork(ctx context.Context, secrets map[string]string) (*TenantNetwork, error) {
	var me authMeResponse
	if err := c.do(ctx, secrets, http.MethodGet, authMePath, nil, &me); err != nil {
		return nil, err
	}
	if me.TenantID == "" {
		return nil, fmt.Errorf("%w: the token belongs to no tenant", ErrorInvalidArgument)
	}

	var tenant tenantGetResponse
	if err := c.do(ctx, secrets, http.MethodGet, tenantsPath+"/"+url.PathEscape(me.TenantID), nil, &tenant); err != nil {
		return nil, err
	}

	var list subnetListResponse
	if err := c.do(ctx, secrets, http.MethodGet, subnetsPath, nil, &list); err != nil {
		return nil, err
	}
	if len(list.Subnets) == 0 {
		return nil, fmt.Errorf("%w: tenant %q has no subnet", ErrorNotFound, tenant.Tenant.Name)
	}

	// A tenant has one subnet today; see GetMountAddress.
	subnetID := list.Subnets[0].ID
	var info mountInfoResponse
	if err := c.do(ctx, secrets, http.MethodGet, subnetPath(subnetID)+"/mount-info", nil, &info); err != nil {
		return nil, err
	}

	return &TenantNetwork{
		TenantName: tenant.Tenant.Name,
		SubnetID:   subnetID,
		Directors:  SplitRMList(info.RMList),
	}, nil
}

// AttachHost attaches a host to a tenant subnet and returns its attachment record.
// A repeated call for the same underlay address does not move the host; the stored record is returned.
func (c *Client) AttachHost(ctx context.Context, subnetID, underlayIP, hostname string, secrets map[string]string) (*Attachment, error) {
	path := subnetPath(subnetID) + "/attachments"

	var created Attachment
	request := attachmentCreateRequest{UnderlayIP: underlayIP, Hostname: hostname}
	if err := c.do(ctx, secrets, http.MethodPost, path, request, &created); err != nil {
		return nil, err
	}
	if created.ID != "" && created.OverlayIP != "" {
		return &created, nil
	}

	// A repeated attach may answer without the record; read it from the list.
	found, err := c.FindAttachment(ctx, subnetID, underlayIP, secrets)
	if err != nil {
		return nil, err
	}
	if found == nil {
		return nil, fmt.Errorf("%w: subnet %s has no attachment for %s after the attach call",
			ErrorNotFound, subnetID, underlayIP)
	}
	return found, nil
}

// FindAttachment returns the attachment record of a host on a tenant subnet, or nil when the host is not attached.
func (c *Client) FindAttachment(ctx context.Context, subnetID, underlayIP string, secrets map[string]string) (*Attachment, error) {
	var list attachmentListResponse
	path := subnetPath(subnetID) + "/attachments?underlay_ip=" + url.QueryEscape(underlayIP)
	if err := c.do(ctx, secrets, http.MethodGet, path, nil, &list); err != nil {
		return nil, err
	}
	for i := range list.Attachments {
		if list.Attachments[i].UnderlayIP == underlayIP {
			return &list.Attachments[i], nil
		}
	}
	return nil, nil
}

// DetachHost removes the attachment of a host from a tenant subnet. A missing attachment is not an error.
func (c *Client) DetachHost(ctx context.Context, subnetID, attachmentID string, secrets map[string]string) error {
	path := subnetPath(subnetID) + "/attachments/" + url.PathEscape(attachmentID)
	err := c.do(ctx, secrets, http.MethodDelete, path, nil, nil)
	if errors.Is(err, ErrorNotFound) {
		return nil
	}
	return err
}

// subnetPath returns the path of one subnet.
func subnetPath(id string) string {
	return subnetsPath + "/" + url.PathEscape(id)
}
