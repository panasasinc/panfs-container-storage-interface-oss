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

package tenantnet

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
)

const (
	firewalldBusName = "org.fedoraproject.FirewallD1"
	firewalldPath    = "/org/fedoraproject/FirewallD1"
	firewalldZone    = "org.fedoraproject.FirewallD1.zone"
)

// zoneNamePattern is what a firewalld zone name may look like; the name goes into a command line.
var zoneNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,17}$`)

// Firewalld moves an interface into a firewalld zone of the host, through the system D-Bus of the host.
//
// A new interface falls into the default zone, which usually drops the connections that the realm opens
// to the PanFS client. The change is made on the running firewall only, so the reconciler repeats it.
type Firewalld struct {
	// Run runs dbus-send.
	Run Runner
	// Socket is the path of the system D-Bus socket of the host inside the container.
	Socket string
	// Zone is the zone the tenant interface belongs to. An empty zone switches the step off.
	Zone string
}

// dbusSend calls one method on the system bus of the host and returns the reply text.
func (f *Firewalld) dbusSend(ctx context.Context, dest, path, method string, args ...string) (string, error) {
	base := []string{"--bus=unix:path=" + f.Socket, "--print-reply=literal", "--reply-timeout=10000",
		"--dest=" + dest, path, method}
	out, err := f.Run.Run(ctx, "dbus-send", append(base, args...)...)
	return strings.TrimSpace(string(out)), err
}

// EnsureZone puts the interface into the configured zone when firewalld runs on the host.
// It returns true when it changed the zone. A host without firewalld, or without D-Bus, needs no change.
func (f *Firewalld) EnsureZone(ctx context.Context, name string) (bool, error) {
	if f.Zone == "" {
		return false, nil
	}
	if !zoneNamePattern.MatchString(f.Zone) {
		return false, fmt.Errorf("firewalld zone %q is not a valid zone name", f.Zone)
	}
	if _, err := os.Stat(f.Socket); err != nil {
		// No system bus on the host means no firewalld.
		return false, nil
	}

	// Ask the bus first: a direct call to firewalld could start a stopped service.
	owned, err := f.dbusSend(ctx, "org.freedesktop.DBus", "/org/freedesktop/DBus",
		"org.freedesktop.DBus.NameHasOwner", "string:"+firewalldBusName)
	if err != nil {
		return false, fmt.Errorf("ask the host D-Bus for firewalld: %w", err)
	}
	if !strings.Contains(owned, "true") {
		return false, nil
	}

	current, err := f.dbusSend(ctx, firewalldBusName, firewalldPath, firewalldZone+".getZoneOfInterface", "string:"+name)
	if err != nil {
		return false, fmt.Errorf("read the firewalld zone of %s: %w", name, err)
	}
	if current == f.Zone {
		return false, nil
	}

	if _, err := f.dbusSend(ctx, firewalldBusName, firewalldPath, firewalldZone+".changeZoneOfInterface",
		"string:"+f.Zone, "string:"+name); err != nil {
		return false, fmt.Errorf("move %s to the firewalld zone %s: %w", name, f.Zone, err)
	}
	return true, nil
}
