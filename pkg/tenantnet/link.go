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

// Package tenantnet puts a Kubernetes node on the network of a PanFS tenant and reports the result as a node label.
package tenantnet

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/panasasinc/panfs-container-storage-interface-oss/pkg/panapi"
)

// floodMAC is the all-zero address of a VXLAN flood entry.
const floodMAC = "00:00:00:00:00:00"

// OwnerAlias is the alias that marks an interface as created by this program. An interface with the name
// of a tenant interface but without this alias belongs to somebody else and is never changed or removed.
const OwnerAlias = "panfs-tenant-network"

// Runner runs a command of the host network tools and returns its output.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// ExecRunner runs commands with os/exec.
type ExecRunner struct{}

// Run runs the command and returns its standard output. The error carries the standard error text.
func (ExecRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if text := strings.TrimSpace(stderr.String()); text != "" {
			return out, fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, text)
		}
		return out, fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return out, nil
}

// InterfaceName returns the name of the tenant interface of a VXLAN segment.
func InterfaceName(segmentID int) string {
	return "vxlan" + strconv.Itoa(segmentID)
}

// VLANInterfaceName returns the name of the tenant interface of a VLAN segment.
// It does not carry the name of the parent interface, so it always fits the 15 characters of an interface name.
func VLANInterfaceName(segmentID int) string {
	return "vlan" + strconv.Itoa(segmentID)
}

// InterfaceNameOf returns the name of the tenant interface of an attachment, or "" for a segment type
// this package cannot build.
func InterfaceNameOf(a *panapi.Attachment) string {
	switch a.SegmentType {
	case panapi.SegmentTypeVXLAN:
		return InterfaceName(a.SegmentID)
	case panapi.SegmentTypeVLAN:
		return VLANInterfaceName(a.SegmentID)
	}
	return ""
}

// interfaceNamePattern is what a name of a network interface may look like. The name goes into command lines.
var interfaceNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,14}$`)

// linkState is the part of `ip -json -details link show` and `ip -json addr show` that the reconciler reads.
type linkState struct {
	Flags   []string `json:"flags"`
	MTU     int      `json:"mtu"`
	Address string   `json:"address"`
	// Alias is the alias of the interface; see OwnerAlias.
	Alias string `json:"ifalias"`
	// Link is the parent interface of a VLAN interface.
	Link     string `json:"link"`
	LinkInfo struct {
		Kind string `json:"info_kind"`
		Data struct {
			ID    int    `json:"id"`
			Local string `json:"local"`
			Port  int    `json:"port"`
		} `json:"info_data"`
	} `json:"linkinfo"`
	AddrInfo []struct {
		Family    string `json:"family"`
		Local     string `json:"local"`
		PrefixLen int    `json:"prefixlen"`
	} `json:"addr_info"`
}

// fdbEntry is one entry of `bridge -json fdb show`.
type fdbEntry struct {
	MAC string `json:"mac"`
	Dst string `json:"dst"`
}

// validateAttachment checks that the record describes an interface this package can build.
// The values go into command lines, so every one of them is parsed first.
func validateAttachment(a *panapi.Attachment) error {
	switch a.SegmentType {
	case panapi.SegmentTypeVXLAN:
		if a.Tunnel == nil {
			return fmt.Errorf("the attachment has no tunnel recipe")
		}
		if a.SegmentID <= 0 || a.SegmentID >= 1<<24 {
			return fmt.Errorf("segment id %d is not a VXLAN network id", a.SegmentID)
		}
		if a.Tunnel.UDPPort <= 0 || a.Tunnel.UDPPort > 65535 {
			return fmt.Errorf("tunnel UDP port %d is not valid", a.Tunnel.UDPPort)
		}
		if _, err := net.ParseMAC(a.Tunnel.MAC); err != nil {
			return fmt.Errorf("tunnel MAC %q is not valid", a.Tunnel.MAC)
		}
		if len(a.Tunnel.VTEPs) == 0 {
			return fmt.Errorf("the attachment has no VTEP")
		}
		for _, vtep := range a.Tunnel.VTEPs {
			if net.ParseIP(vtep) == nil {
				return fmt.Errorf("VTEP %q is not an IP address", vtep)
			}
		}
	case panapi.SegmentTypeVLAN:
		if a.SegmentID <= 0 || a.SegmentID > 4094 {
			return fmt.Errorf("segment id %d is not a VLAN id", a.SegmentID)
		}
	default:
		return fmt.Errorf("segment type %q is not supported; only %q and %q are",
			a.SegmentType, panapi.SegmentTypeVXLAN, panapi.SegmentTypeVLAN)
	}
	if a.MTU < 68 {
		return fmt.Errorf("MTU %d is not valid", a.MTU)
	}
	overlay := net.ParseIP(a.OverlayIP)
	if overlay == nil || overlay.To4() == nil {
		return fmt.Errorf("overlay address %q is not an IPv4 address", a.OverlayIP)
	}
	if a.OverlayPrefix <= 0 || a.OverlayPrefix > 32 {
		return fmt.Errorf("overlay prefix %d is not valid", a.OverlayPrefix)
	}
	if net.ParseIP(a.UnderlayIP) == nil {
		return fmt.Errorf("underlay address %q is not an IP address", a.UnderlayIP)
	}
	return nil
}

// sameIP reports whether two address strings name the same IP address.
func sameIP(a, b string) bool {
	ipA, ipB := net.ParseIP(a), net.ParseIP(b)
	return ipA != nil && ipA.Equal(ipB)
}

// hasFlag reports whether the link has the flag.
func (s *linkState) hasFlag(flag string) bool {
	for _, f := range s.Flags {
		if f == flag {
			return true
		}
	}
	return false
}

// namedLink is one interface of the host.
type namedLink struct {
	Name string `json:"ifname"`
	linkState
}

// listLinks returns the interfaces of the host.
func listLinks(ctx context.Context, run Runner) ([]namedLink, error) {
	// `ip link show` on a missing interface is an error; list all links instead and pick by name.
	out, err := run.Run(ctx, "ip", "-json", "-details", "addr", "show")
	if err != nil {
		return nil, err
	}
	var links []namedLink
	if err := json.Unmarshal(out, &links); err != nil {
		return nil, fmt.Errorf("parse the output of ip addr show: %w", err)
	}
	return links, nil
}

// findLink returns the state of an interface, or nil when the interface does not exist.
func findLink(links []namedLink, name string) *linkState {
	for i := range links {
		if links[i].Name == name {
			return &links[i].linkState
		}
	}
	return nil
}

// readLink returns the state of an interface, or nil when the interface does not exist.
func readLink(ctx context.Context, run Runner, name string) (*linkState, error) {
	links, err := listLinks(ctx, run)
	if err != nil {
		return nil, err
	}
	return findLink(links, name), nil
}

// EnsureInterface builds the tenant interface of an attachment, or corrects it in place.
// See EnsureTenantInterface; the parent of a VLAN interface is found by the underlay address.
func EnsureInterface(ctx context.Context, run Runner, a *panapi.Attachment) (name string, changed bool, err error) {
	return EnsureTenantInterface(ctx, run, a, "")
}

// EnsureTenantInterface builds the tenant interface of an attachment, or corrects it in place.
// An interface that already matches the record is not touched, so mounted volumes keep working.
// vlanParent names the interface that carries the VLAN of a VLAN segment; when it is empty, the interface
// with the underlay address of the node is used. It returns the interface name and whether anything was changed.
func EnsureTenantInterface(ctx context.Context, run Runner, a *panapi.Attachment, vlanParent string) (name string, changed bool, err error) {
	if err := validateAttachment(a); err != nil {
		return "", false, err
	}
	name = InterfaceNameOf(a)
	links, err := listLinks(ctx, run)
	if err != nil {
		return name, false, err
	}
	state := findLink(links, name)
	if state != nil && state.Alias != OwnerAlias {
		if !isHalfBuilt(state, a, links, vlanParent) {
			return name, false, fmt.Errorf("an interface %s already exists on the node and was not created by the "+
				"PanFS Tenant Network (it has no alias %q); it is left as it is. Remove or rename it", name, OwnerAlias)
		}
		// An earlier pod created the interface and stopped before it could mark it. Nothing uses it yet.
		if err := markOwned(ctx, run, name); err != nil {
			return name, false, err
		}
		changed = true
	}

	var built bool
	if a.SegmentType == panapi.SegmentTypeVLAN {
		state, built, err = ensureVLANLink(ctx, run, a, name, state, links, vlanParent)
	} else {
		state, built, err = ensureVXLANLink(ctx, run, a, name, state)
	}
	changed = changed || built
	if err != nil {
		return name, changed, err
	}

	settled, err := ensureLinkSettings(ctx, run, a, name, state)
	changed = changed || settled
	if err != nil || a.SegmentType != panapi.SegmentTypeVXLAN {
		return name, changed, err
	}
	fdbChanged, err := ensureFloodList(ctx, run, name, a.Tunnel.VTEPs)
	return name, changed || fdbChanged, err
}

// markOwned marks an interface as created by this program.
func markOwned(ctx context.Context, run Runner, name string) error {
	_, err := run.Run(ctx, "ip", "link", "set", name, "alias", OwnerAlias)
	return err
}

// isHalfBuilt reports whether an unmarked interface is one that an earlier pod created for this segment and
// did not finish: it is down, it has no IPv4 address, and it has every setting that cannot be changed later
// as this program would create it (VXLAN: network id, port and local address; VLAN: id and parent).
// Nothing can use such an interface, and taking it over never deletes it.
func isHalfBuilt(state *linkState, a *panapi.Attachment, links []namedLink, vlanParent string) bool {
	if state.LinkInfo.Kind != a.SegmentType || state.LinkInfo.Data.ID != a.SegmentID || state.hasFlag("UP") {
		return false
	}
	for _, addr := range state.AddrInfo {
		if addr.Family == "inet" {
			return false
		}
	}
	if a.SegmentType == panapi.SegmentTypeVLAN {
		parent, err := vlanParentOf(links, a, vlanParent)
		return err == nil && state.Link == parent
	}
	return state.LinkInfo.Data.Port == a.Tunnel.UDPPort && sameIP(state.LinkInfo.Data.Local, a.UnderlayIP)
}

// ensureVXLANLink makes the VXLAN interface exist with the right network id, port, local address and MAC.
func ensureVXLANLink(ctx context.Context, run Runner, a *panapi.Attachment, name string, state *linkState) (*linkState, bool, error) {
	changed := false
	mac := strings.ToLower(a.Tunnel.MAC)

	// The network id, the port and the local address cannot be changed on an existing VXLAN interface.
	if state != nil {
		data := state.LinkInfo.Data
		if state.LinkInfo.Kind != "vxlan" || data.ID != a.SegmentID || data.Port != a.Tunnel.UDPPort ||
			!sameIP(data.Local, a.UnderlayIP) {
			if _, err := run.Run(ctx, "ip", "link", "del", name); err != nil {
				return state, changed, err
			}
			changed, state = true, nil
		}
	}

	if state == nil {
		// Creating the first VXLAN interface makes the kernel load the vxlan module.
		if _, err := run.Run(ctx, "ip", "link", "add", name, "type", "vxlan",
			"id", strconv.Itoa(a.SegmentID), "dstport", strconv.Itoa(a.Tunnel.UDPPort), "local", a.UnderlayIP); err != nil {
			return state, changed, err
		}
		if err := markOwned(ctx, run, name); err != nil {
			return state, true, err
		}
		changed, state = true, &linkState{}
	}

	if !strings.EqualFold(state.Address, mac) {
		// The address of a running interface cannot always be changed; take the interface down first.
		if state.hasFlag("UP") {
			if _, err := run.Run(ctx, "ip", "link", "set", name, "down"); err != nil {
				return state, changed, err
			}
			state.Flags = nil
		}
		if _, err := run.Run(ctx, "ip", "link", "set", name, "address", mac); err != nil {
			return state, changed, err
		}
		changed = true
	}
	return state, changed, nil
}

// vlanParentOf returns the interface that carries the VLAN: the named one, or the one with the underlay address.
func vlanParentOf(links []namedLink, a *panapi.Attachment, vlanParent string) (string, error) {
	if vlanParent != "" {
		if !interfaceNamePattern.MatchString(vlanParent) {
			return "", fmt.Errorf("VLAN parent interface %q is not a valid interface name", vlanParent)
		}
		if findLink(links, vlanParent) == nil {
			return "", fmt.Errorf("VLAN parent interface %q does not exist on the node", vlanParent)
		}
		return vlanParent, nil
	}
	for i := range links {
		for _, addr := range links[i].AddrInfo {
			if sameIP(addr.Local, a.UnderlayIP) {
				return links[i].Name, nil
			}
		}
	}
	return "", fmt.Errorf("no interface of the node has the underlay address %s; name the VLAN parent interface", a.UnderlayIP)
}

// ensureVLANLink makes the VLAN interface exist on the right parent with the right VLAN id.
// A VLAN fabric forwards and learns on its own, so there is no MAC address to set and no flood list.
func ensureVLANLink(ctx context.Context, run Runner, a *panapi.Attachment, name string, state *linkState,
	links []namedLink, vlanParent string) (*linkState, bool, error) {
	parent, err := vlanParentOf(links, a, vlanParent)
	if err != nil {
		return state, false, err
	}
	changed := false

	// The parent and the VLAN id cannot be changed on an existing VLAN interface.
	if state != nil && (state.LinkInfo.Kind != "vlan" || state.LinkInfo.Data.ID != a.SegmentID || state.Link != parent) {
		if _, err := run.Run(ctx, "ip", "link", "del", name); err != nil {
			return state, changed, err
		}
		changed, state = true, nil
	}
	if state == nil {
		// Creating the first VLAN interface makes the kernel load the 8021q module.
		if _, err := run.Run(ctx, "ip", "link", "add", "link", parent, "name", name, "type", "vlan",
			"id", strconv.Itoa(a.SegmentID)); err != nil {
			return state, changed, err
		}
		if err := markOwned(ctx, run, name); err != nil {
			return state, true, err
		}
		changed, state = true, &linkState{}
	}
	return state, changed, nil
}

// ensureLinkSettings sets the MTU, brings the interface up, and keeps exactly one IPv4 address on it:
// the overlay address of the record.
func ensureLinkSettings(ctx context.Context, run Runner, a *panapi.Attachment, name string, state *linkState) (bool, error) {
	changed := false
	if state.MTU != a.MTU {
		if _, err := run.Run(ctx, "ip", "link", "set", name, "mtu", strconv.Itoa(a.MTU)); err != nil {
			return changed, err
		}
		changed = true
	}
	if !state.hasFlag("UP") {
		if _, err := run.Run(ctx, "ip", "link", "set", name, "up"); err != nil {
			return changed, err
		}
		changed = true
	}

	hasOverlay := false
	for _, addr := range state.AddrInfo {
		if addr.Family != "inet" {
			continue
		}
		if sameIP(addr.Local, a.OverlayIP) && addr.PrefixLen == a.OverlayPrefix {
			hasOverlay = true
			continue
		}
		cidr := addr.Local + "/" + strconv.Itoa(addr.PrefixLen)
		if _, err := run.Run(ctx, "ip", "addr", "del", cidr, "dev", name); err != nil {
			return changed, err
		}
		changed = true
	}
	if !hasOverlay {
		cidr := a.OverlayIP + "/" + strconv.Itoa(a.OverlayPrefix)
		if _, err := run.Run(ctx, "ip", "addr", "add", cidr, "dev", name); err != nil {
			return changed, err
		}
		changed = true
	}
	return changed, nil
}

// ensureFloodList makes the flood entries of the interface equal to the VTEP list.
func ensureFloodList(ctx context.Context, run Runner, name string, vteps []string) (bool, error) {
	out, err := run.Run(ctx, "bridge", "-json", "fdb", "show", "dev", name)
	if err != nil {
		return false, err
	}
	var entries []fdbEntry
	if err := json.Unmarshal(out, &entries); err != nil {
		return false, fmt.Errorf("parse the output of bridge fdb show: %w", err)
	}

	want := make(map[string]string, len(vteps))
	for _, vtep := range vteps {
		want[net.ParseIP(vtep).String()] = vtep
	}

	changed := false
	for _, entry := range entries {
		if entry.MAC != floodMAC || entry.Dst == "" {
			continue
		}
		key := entry.Dst
		if ip := net.ParseIP(entry.Dst); ip != nil {
			key = ip.String()
		}
		if _, ok := want[key]; ok {
			delete(want, key)
			continue
		}
		if _, err := run.Run(ctx, "bridge", "fdb", "del", floodMAC, "dev", name, "dst", entry.Dst); err != nil {
			return changed, err
		}
		changed = true
	}
	// Add the missing entries in the order of the record.
	for _, vtep := range vteps {
		if _, ok := want[net.ParseIP(vtep).String()]; !ok {
			continue
		}
		delete(want, net.ParseIP(vtep).String())
		if _, err := run.Run(ctx, "bridge", "fdb", "append", floodMAC, "dev", name, "dst", vtep); err != nil {
			return changed, err
		}
		changed = true
	}
	return changed, nil
}

// Ping reports whether one of the addresses answers on the interface.
func Ping(ctx context.Context, run Runner, name string, addresses []string) (answered string, err error) {
	if len(addresses) == 0 {
		return "", fmt.Errorf("the tenant subnet has no director address to check")
	}
	var lastErr error
	for _, address := range addresses {
		if net.ParseIP(address) == nil {
			lastErr = fmt.Errorf("director address %q is not an IP address", address)
			continue
		}
		// Three requests: on a new interface the first one is often lost while the neighbour is resolved.
		// ping reports success when one of them is answered.
		if _, lastErr = run.Run(ctx, "ping", "-c", "3", "-W", "2", "-I", name, address); lastErr == nil {
			return address, nil
		}
	}
	return "", fmt.Errorf("no tenant director answers on %s: %w", name, lastErr)
}
