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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/panasasinc/panfs-container-storage-interface-oss/pkg/panapi"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

const (
	// TenantLabelKey is the node label an administrator sets: the node is meant for this tenant.
	TenantLabelKey = "panfs.vdura.com/tenant"
	// ReadyLabelKey is the node label this program sets: the network of this tenant works on the node.
	ReadyLabelKey = "panfs.vdura.com/tenant-network"

	// OverlayAnnotation, InterfaceAnnotation and MessageAnnotation are node annotations this program sets:
	// the address of the node on the tenant network, the name of the tenant interface,
	// and, while the node is not ready, the reason.
	OverlayAnnotation   = "panfs.vdura.com/tenant-network-overlay-ip"
	InterfaceAnnotation = "panfs.vdura.com/tenant-network-interface"
	MessageAnnotation   = "panfs.vdura.com/tenant-network-message"

	// notReadyRetry is the longest wait before the next check of a node that is not ready.
	notReadyRetry = 5 * time.Second
)

// API is the part of the PanAPI client the reconciler uses.
type API interface {
	GetTenantNetwork(ctx context.Context, secrets map[string]string) (*panapi.TenantNetwork, error)
	AttachHost(ctx context.Context, subnetID, underlayIP, hostname string, secrets map[string]string) (*panapi.Attachment, error)
	FindAttachment(ctx context.Context, subnetID, underlayIP string, secrets map[string]string) (*panapi.Attachment, error)
	DetachHost(ctx context.Context, subnetID, attachmentID string, secrets map[string]string) error
}

// Config is the configuration of a Reconciler.
type Config struct {
	// Tenant is the name of the tenant; it is the value of both node labels.
	Tenant string
	// NodeName is the name of the Kubernetes node the program runs on.
	NodeName string
	// HostIP is the address of the node on the realm underlay.
	HostIP string
	// SecretDir is the directory the Secret with the PanAPI address and token is mounted at.
	SecretDir string
	// ReadyLabelKey is the key of the node label that reports a working tenant network.
	ReadyLabelKey string
	// ReadyValue is the value of that label. The operator sets it to the namespace of the TenantNetwork;
	// an empty value means the tenant name.
	ReadyValue string
	// ReadyFile is created while the tenant network works; the readiness probe checks it.
	ReadyFile string
	// Interval is the time between two checks.
	Interval time.Duration
	// Namespace and DaemonSet name the DaemonSet this pod belongs to. They are used on shutdown,
	// to tell an uninstall from a restart. An empty DaemonSet name switches that check off.
	Namespace string
	DaemonSet string
	// VLANParent names the interface that carries the VLAN when the tenant subnet is a VLAN segment.
	// An empty name means the interface with the underlay address of the node.
	VLANParent string
	// DetachOnRemoval makes the program remove the tenant interface and detach the node from the subnet
	// when the node leaves the tenant: its tenant label is gone, or the DaemonSet is being deleted.
	DetachOnRemoval bool
}

// Reconciler keeps one node on the network of one tenant and reports the result as a node label.
type Reconciler struct {
	Config   Config
	API      API
	Run      Runner
	Firewall *Firewalld
	// Kube sets the node label. A nil client switches the label off, for a run outside a cluster.
	Kube kubernetes.Interface
	// Events reports changes on the TenantNetwork of this pod. A nil sink switches the events off.
	Events *EventSink
	Log    klog.Logger

	// ready is the last reported state; nil before the first report.
	ready *bool
	// attachmentID is the attachment of the last pass; a new one is reported as an event.
	attachmentID string
	// overlay and iface are the address and the interface of the node on the tenant network, once known.
	overlay, iface string
	// message says why the node is not ready; it is reported as a node annotation.
	message string
	// failures counts the passes in a row that failed; warned is set once they were reported as an event.
	failures int
	warned   bool
}

// readSecrets reads the keys of the mounted Secret. It is read on every pass, so a changed token is picked up.
func readSecrets(dir string) (map[string]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read the secret directory %s: %w", dir, err)
	}
	secrets := make(map[string]string)
	for _, entry := range entries {
		// A mounted Secret has hidden helper entries (..data, ..<timestamp>); skip them.
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		secrets[entry.Name()] = strings.TrimSpace(string(data))
	}
	return secrets, nil
}

// Reconcile runs one pass: attach the node, build or correct the interface, set the firewall zone,
// and check that a tenant director answers. It returns nil only when the tenant network works.
func (r *Reconciler) Reconcile(ctx context.Context) error {
	secrets, err := readSecrets(r.Config.SecretDir)
	if err != nil {
		return err
	}

	network, err := r.API.GetTenantNetwork(ctx, secrets)
	if err != nil {
		return fmt.Errorf("read the tenant network from PanAPI: %w", err)
	}
	// The label must tell the truth: never report tenant A for a token of tenant B.
	if network.TenantName != r.Config.Tenant {
		return fmt.Errorf("the token belongs to tenant %q, but this release is for tenant %q",
			network.TenantName, r.Config.Tenant)
	}

	attachment, err := r.API.AttachHost(ctx, network.SubnetID, r.Config.HostIP, r.Config.NodeName, secrets)
	if err != nil {
		return fmt.Errorf("attach the node to the tenant subnet: %w", err)
	}

	if attachment.ID != r.attachmentID {
		r.attachmentID = attachment.ID
		r.Events.Event(ctx, corev1.EventTypeNormal, ReasonNodeAttached, fmt.Sprintf(
			"Node %s is attached to the subnet of tenant %s, overlay address %s/%d",
			r.Config.NodeName, r.Config.Tenant, attachment.OverlayIP, attachment.OverlayPrefix))
	}

	name, changed, err := EnsureTenantInterface(ctx, r.Run, attachment, r.Config.VLANParent)
	if err != nil {
		return fmt.Errorf("set up the tenant interface: %w", err)
	}
	r.overlay, r.iface = attachment.OverlayIP, name
	if changed {
		// A VXLAN interface floods to the realm nodes itself; a VLAN fabric does that on its own.
		detail := fmt.Sprintf("%s %d, MTU %d", strings.ToUpper(attachment.SegmentType), attachment.SegmentID, attachment.MTU)
		if attachment.Tunnel != nil {
			detail += fmt.Sprintf(", %d realm nodes in the flood list", len(attachment.Tunnel.VTEPs))
		}
		r.Log.Info("tenant interface was created or corrected", "interface", name,
			"overlay", fmt.Sprintf("%s/%d", attachment.OverlayIP, attachment.OverlayPrefix), "detail", detail)
		r.Events.Event(ctx, corev1.EventTypeNormal, ReasonInterfaceConfigured, fmt.Sprintf(
			"Node %s: interface %s was created or corrected (%s)", r.Config.NodeName, name, detail))
	}

	if r.Firewall != nil {
		moved, err := r.Firewall.EnsureZone(ctx, name)
		if err != nil {
			return err
		}
		if moved {
			r.Log.Info("tenant interface was moved to the firewalld zone", "interface", name, "zone", r.Firewall.Zone)
			r.Events.Event(ctx, corev1.EventTypeNormal, ReasonFirewallZoneSet, fmt.Sprintf(
				"Node %s: interface %s was moved to the firewalld zone %s", r.Config.NodeName, name, r.Firewall.Zone))
		}
	}

	director, err := Ping(ctx, r.Run, name, network.Directors)
	if err != nil {
		return err
	}
	r.Log.V(4).Info("tenant director answers", "interface", name, "director", director)
	return nil
}

// readyValue returns the value of the ready label.
func (r *Reconciler) readyValue() string {
	if r.Config.ReadyValue != "" {
		return r.Config.ReadyValue
	}
	return r.Config.Tenant
}

// setReady reports the state: the node label, the node annotations and the ready file.
func (r *Reconciler) setReady(ctx context.Context, ready bool) error {
	if r.Config.ReadyFile != "" {
		if ready {
			if err := os.WriteFile(r.Config.ReadyFile, []byte(r.Config.Tenant+"\n"), 0o644); err != nil {
				return fmt.Errorf("write the ready file: %w", err)
			}
		} else if err := os.Remove(r.Config.ReadyFile); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove the ready file: %w", err)
		}
	}
	if r.Kube == nil {
		return nil
	}

	// A merge patch with null removes a key. A patch that changes nothing does not touch the node object.
	orNull := func(value string) any {
		if value == "" {
			return nil
		}
		return value
	}
	var label, message any
	if ready {
		label = r.readyValue()
	} else {
		message = orNull(r.message)
	}
	patch, err := json.Marshal(map[string]any{
		"metadata": map[string]any{
			"labels": map[string]any{r.Config.ReadyLabelKey: label},
			"annotations": map[string]any{
				OverlayAnnotation: orNull(r.overlay), InterfaceAnnotation: orNull(r.iface), MessageAnnotation: message,
			},
		},
	})
	if err != nil {
		return err
	}
	if _, err := r.Kube.CoreV1().Nodes().Patch(ctx, r.Config.NodeName, types.MergePatchType, patch,
		metav1.PatchOptions{}); err != nil {
		return fmt.Errorf("patch the label %s of node %s: %w", r.Config.ReadyLabelKey, r.Config.NodeName, err)
	}
	return nil
}

// step runs one pass and reports its result.
func (r *Reconciler) step(ctx context.Context) {
	passCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	err := r.Reconcile(passCtx)
	ready := err == nil
	if r.ready == nil || *r.ready != ready {
		if ready {
			r.Log.Info("the tenant network works; the node is marked ready",
				"label", r.Config.ReadyLabelKey+"="+r.readyValue())
			r.Events.Event(passCtx, corev1.EventTypeNormal, ReasonNodeReady, fmt.Sprintf(
				"Node %s: a tenant director answers on %s; the label %s=%s is set",
				r.Config.NodeName, r.iface, r.Config.ReadyLabelKey, r.readyValue()))
		} else {
			r.Log.Error(err, "the tenant network does not work; the node is marked not ready")
		}
	} else if !ready {
		r.Log.Error(err, "the tenant network still does not work")
	}
	// A node that was ready reports a failure at once. A node that starts reports it only when a second pass
	// fails too: on a new interface the first check often fails while the realm learns the node.
	if ready {
		r.failures, r.warned = 0, false
	} else if r.failures++; !r.warned && ((r.ready != nil && *r.ready) || r.failures >= 2) {
		r.warned = true
		r.Events.Event(passCtx, EventTypeError, ReasonNodeNotReady, fmt.Sprintf("Node %s: %v", r.Config.NodeName, err))
	}
	r.message = ""
	if err != nil {
		r.message = err.Error()
	}

	if err := r.setReady(passCtx, ready); err != nil {
		r.Log.Error(err, "cannot report the state of the tenant network")
		return
	}
	r.ready = &ready
}

// Loop runs passes until the context is done, then removes the ready label.
// The interface stays, because mounted volumes depend on it.
func (r *Reconciler) Loop(ctx context.Context) {
	for {
		r.step(ctx)

		// A node that is not ready is checked again sooner, so it becomes ready without a long wait.
		wait := r.Config.Interval
		if (r.ready == nil || !*r.ready) && wait > notReadyRetry {
			wait = notReadyRetry
		}
		timer := time.NewTimer(wait)

		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			r.shutdown()
			return
		}
	}
}

// shutdown runs when the pod stops. Nobody checks the network after this point, so the ready label is removed.
// When the node left the tenant, the interface is removed and the node is detached from the subnet as well.
// After a plain restart, for example an upgrade, both stay, because mounted volumes depend on them.
func (r *Reconciler) shutdown() {
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()

	// Decide before the label is touched: the decision reads the node.
	removed, reason := r.removedFromTenant(ctx)

	r.message = "the pod of the node stopped"
	if removed && r.Config.DetachOnRemoval {
		// The interface is about to go; do not leave its name and address on the node.
		r.overlay, r.iface, r.message = "", "", ""
	}
	if err := r.setReady(ctx, false); err != nil {
		r.Log.Error(err, "shutdown: cannot remove the ready label")
	} else {
		r.Log.Info("shutdown: the ready label was removed")
	}

	if !removed {
		r.Log.Info("shutdown: the node stays in the tenant; the interface and the attachment stay")
		r.Events.Event(ctx, corev1.EventTypeNormal, ReasonNodeStopped, fmt.Sprintf(
			"Node %s: the pod stopped; the ready label was removed, the interface and the attachment stay", r.Config.NodeName))
		return
	}
	if !r.Config.DetachOnRemoval {
		r.Log.Info("shutdown: the node left the tenant, but detach on removal is off", "reason", reason)
		return
	}
	r.Log.Info("shutdown: the node left the tenant; removing the interface and the attachment", "reason", reason)
	if err := r.Detach(ctx); err != nil {
		r.Log.Error(err, "shutdown: cannot detach the node from the tenant subnet")
		r.Events.Event(ctx, EventTypeError, ReasonNodeNotReady, fmt.Sprintf(
			"Node %s left the tenant, but it could not be detached: %v", r.Config.NodeName, err))
		return
	}
	r.Events.Event(ctx, corev1.EventTypeNormal, ReasonNodeDetached, fmt.Sprintf(
		"Node %s left the tenant (%s): the interface was removed and the node was detached from the subnet",
		r.Config.NodeName, reason))
}

// removedFromTenant reports whether the node no longer belongs to the tenant, and why.
// When the state cannot be read, for whatever reason, the answer is no:
// a wrong detach cuts mounted volumes off, and a missed detach only leaves a record behind.
func (r *Reconciler) removedFromTenant(ctx context.Context) (bool, string) {
	if r.Kube == nil {
		return false, ""
	}

	node, err := r.Kube.CoreV1().Nodes().Get(ctx, r.Config.NodeName, metav1.GetOptions{})
	switch {
	case apierrors.IsNotFound(err):
		return true, "the node object is gone"
	case err != nil:
		// This includes missing permissions. They are no sign that the node left the tenant: the operator
		// can be reinstalled, or its ClusterRole can be missing for a moment, while this pod stops for another
		// reason. Only positive evidence leads to a detach.
		r.Log.Error(err, "shutdown: cannot read the node; assuming a restart")
		return false, ""
	}

	// The node selector of the DaemonSet says which nodes belong to the tenant now; an empty one means all nodes.
	// The pod reads it here, and not from its own settings: when the selector changes, the pod that stops
	// still has the old one. Without a DaemonSet, the node belongs to the tenant while it has the tenant label.
	selector := map[string]string{TenantLabelKey: r.Config.Tenant}
	if r.Config.DaemonSet != "" {
		daemonSet, err := r.Kube.AppsV1().DaemonSets(r.Config.Namespace).Get(ctx, r.Config.DaemonSet, metav1.GetOptions{})
		switch {
		case apierrors.IsNotFound(err):
			return true, "the DaemonSet is deleted"
		case err != nil:
			r.Log.Error(err, "shutdown: cannot read the DaemonSet; assuming a restart")
			return false, ""
		case daemonSet.DeletionTimestamp != nil:
			return true, "the DaemonSet is being deleted"
		}
		selector = daemonSet.Spec.Template.Spec.NodeSelector
	}
	for _, key := range sortedKeys(selector) {
		if node.Labels[key] != selector[key] {
			return true, "the node has no label " + key + "=" + selector[key]
		}
	}
	return false, ""
}

// Detach removes the tenant interface of the node and its attachment to the tenant subnet.
func (r *Reconciler) Detach(ctx context.Context) error {
	secrets, err := readSecrets(r.Config.SecretDir)
	if err != nil {
		return err
	}
	network, err := r.API.GetTenantNetwork(ctx, secrets)
	if err != nil {
		return fmt.Errorf("read the tenant network from PanAPI: %w", err)
	}
	if network.TenantName != r.Config.Tenant {
		return fmt.Errorf("the token belongs to tenant %q, not %q; nothing is detached", network.TenantName, r.Config.Tenant)
	}
	attachment, err := r.API.FindAttachment(ctx, network.SubnetID, r.Config.HostIP, secrets)
	if err != nil {
		return fmt.Errorf("read the attachment of the node: %w", err)
	}
	if attachment == nil {
		r.Log.Info("the node has no attachment; nothing to detach")
		return nil
	}

	// The interface first: once the realm forgets the node, the interface is of no use.
	if name := InterfaceNameOf(attachment); name != "" && attachment.SegmentID > 0 {
		state, err := readLink(ctx, r.Run, name)
		if err != nil {
			return err
		}
		switch {
		case state == nil:
		case state.Alias != OwnerAlias:
			r.Log.Info("an interface with the name of the tenant interface was not created by this program; it stays",
				"interface", name)
		default:
			if _, err := r.Run.Run(ctx, "ip", "link", "del", name); err != nil {
				return err
			}
			r.Log.Info("the tenant interface was removed", "interface", name)
		}
	}

	if err := r.API.DetachHost(ctx, network.SubnetID, attachment.ID, secrets); err != nil {
		return fmt.Errorf("detach the node from the tenant subnet: %w", err)
	}
	r.Log.Info("the node was detached from the tenant subnet", "overlay", attachment.OverlayIP)
	return nil
}

// sortedKeys returns the keys of a map in order.
func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
