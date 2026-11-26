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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/panasasinc/panfs-container-storage-interface-oss/pkg/panapi"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

// TenantNetworkGVR names the TenantNetwork custom resource.
var TenantNetworkGVR = schema.GroupVersionResource{Group: "panfs.vdura.com", Version: "v1alpha1", Resource: "tenantnetworks"}

const (
	// TenantNetworkKind is the kind of the TenantNetwork custom resource.
	TenantNetworkKind = "TenantNetwork"
	// Finalizer keeps a TenantNetwork until the pods of its DaemonSet cleaned up their nodes.
	Finalizer = "panfs.vdura.com/tenant-network"

	// specHashAnnotation holds the hash of the DaemonSet the operator wants; a different hash means an update.
	specHashAnnotation = "panfs.vdura.com/spec-hash"
	// ownerLabelKey marks the objects the operator creates for a TenantNetwork.
	ownerLabelKey = "panfs.vdura.com/tenant-network-name"
	// namePrefix starts the name of every object the operator creates.
	namePrefix = "panfs-tenant-network-"
	// maxResourceNameLength keeps the names and label values of the created objects valid.
	maxResourceNameLength = 40

	productLabelValue = "com.vdura.csi.panfs"
	secretMountPath   = "/etc/panfs-tenant-network/secret"
)

// Phases of a TenantNetwork.
const (
	PhaseReady    = "Ready"
	PhasePending  = "Pending"
	PhaseError    = "Error"
	PhaseRemoving = "Removing"
)

// TenantNetworkSpec is the spec of a TenantNetwork. Only the Secret name is required.
type TenantNetworkSpec struct {
	// RealmSecretName names a Secret in the namespace of the resource with the keys api, token and, optionally, ca_cert.
	// The tenant is the one the token belongs to.
	RealmSecretName string `json:"realmSecretName"`
	// NodeSelector selects the nodes of the tenant. Without it, they are all nodes of the cluster.
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	// VLANParentInterface names the interface of the nodes that carries the VLAN, when the subnet of the tenant
	// is a VLAN segment. Without it, the interface with the underlay address of the node is used.
	VLANParentInterface string `json:"vlanParentInterface,omitempty"`
	// Interval, FirewalldZone and DetachOnRemoval replace the defaults of the operator for this tenant.
	Interval        string  `json:"interval,omitempty"`
	FirewalldZone   *string `json:"firewalldZone,omitempty"`
	DetachOnRemoval *bool   `json:"detachOnRemoval,omitempty"`
}

// Topology is a node label as a key and a value.
type Topology struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// NodeStatus is the state of one node of the tenant.
type NodeStatus struct {
	// State is Ready or NotReady.
	State string `json:"state"`
	// OverlayIP is the address of the node on the tenant network, once known.
	OverlayIP string `json:"overlayIP,omitempty"`
	// Message says why the node is not ready.
	Message string `json:"message,omitempty"`
}

// States of a node in the status of a TenantNetwork.
const (
	NodeStateReady    = "Ready"
	NodeStateNotReady = "NotReady"
)

// TenantNetworkStatus is the status of a TenantNetwork.
type TenantNetworkStatus struct {
	Phase              string `json:"phase"`
	Message            string `json:"message,omitempty"`
	Tenant             string `json:"tenant,omitempty"`
	DaemonSet          string `json:"daemonSet,omitempty"`
	DesiredNodes       int64  `json:"desiredNodes"`
	ReadyNodes         int64  `json:"readyNodes"`
	ObservedGeneration int64  `json:"observedGeneration,omitempty"`
	// ManagedTopology is the node label that the pods set on a node where the network of the tenant works:
	// the key panfs.vdura.com/tenant-network and, as its value, the namespace of the resource.
	// A StorageClass uses it in allowedTopologies, and a workload in its node selector.
	ManagedTopology *Topology `json:"managedTopology,omitempty"`

	// Interface is the name of the tenant interface on the nodes, once a node reported it.
	Interface string `json:"interface,omitempty"`
	// Nodes holds the state of every node that the selector matches, by node name.
	Nodes map[string]NodeStatus `json:"nodes,omitempty"`
}

// NodeOptions are the defaults of the DaemonSets the operator creates.
type NodeOptions struct {
	// Image is the image of the node program.
	Image            string
	PullPolicy       string
	ImagePullSecrets []string
	// Interval is the time between two checks of the tenant network on a node.
	Interval time.Duration
	// FirewalldZone is the firewalld zone of the tenant interface; empty leaves firewalld alone.
	FirewalldZone string
	// VLANParent names the interface that carries the VLAN on a VLAN segment; empty means the interface
	// with the underlay address of the node.
	VLANParent string
	// DetachOnRemoval makes a pod detach its node when the node leaves the tenant.
	DetachOnRemoval   bool
	LogLevel          string
	PriorityClassName string
	// ClusterRole names an existing ClusterRole that lets a pod read and patch nodes.
	ClusterRole string
}

// TenantResolver tells which tenant a token belongs to.
type TenantResolver interface {
	GetTenantNetwork(ctx context.Context, secrets map[string]string) (*panapi.TenantNetwork, error)
}

// cachedTenant is the tenant name of one Secret version.
type cachedTenant struct {
	secretVersion string
	tenant        string
	at            time.Time
}

// Operator creates one DaemonSet of the node program for every TenantNetwork.
type Operator struct {
	Kube    kubernetes.Interface
	Dynamic dynamic.Interface
	API     TenantResolver
	Options NodeOptions
	// Resync is the time between two passes over all TenantNetworks.
	Resync time.Duration
	// TenantTTL is how long the tenant name of an unchanged Secret is trusted.
	TenantTTL time.Duration
	Log       klog.Logger

	tenants map[string]cachedTenant
	// claims maps a tenant, and a namespace, to the TenantNetwork that serves it in the current pass.
	claims, namespaces map[string]string
	// nodeOwners maps a node to the TenantNetwork that selects it in the current pass, and nodeLists holds
	// the nodes of each selector that was listed in the current pass.
	nodeOwners map[string]string
	nodeLists  map[string]*corev1.NodeList
	// reported holds the last event of each kind per TenantNetwork, so an unchanged state is reported once.
	reported map[string]string
}

// Run reconciles all TenantNetworks until the context is done.
func (o *Operator) Run(ctx context.Context) {
	ticker := time.NewTicker(o.Resync)
	defer ticker.Stop()
	for {
		o.ReconcileAll(ctx)
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return
		}
	}
}

// ReconcileAll runs one pass over the TenantNetworks of all namespaces.
func (o *Operator) ReconcileAll(ctx context.Context) {
	list, err := o.Dynamic.Resource(TenantNetworkGVR).Namespace(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		o.Log.Error(err, "cannot list the TenantNetworks")
		return
	}
	// The oldest TenantNetwork of a tenant serves it; see apply.
	sort.SliceStable(list.Items, func(i, j int) bool {
		a, b := list.Items[i].GetCreationTimestamp(), list.Items[j].GetCreationTimestamp()
		return a.Before(&b)
	})
	o.claims, o.namespaces = make(map[string]string), make(map[string]string)
	o.nodeOwners, o.nodeLists = make(map[string]string), make(map[string]*corev1.NodeList)
	// A TenantNetwork that already runs a DaemonSet keeps its tenant and its namespace, whatever happens to it
	// in this pass. Without this, a short failure of the oldest one (PanAPI is down, its Secret is missing)
	// would let a newer one start a second DaemonSet on the same nodes; removing either of the two later
	// would detach the nodes under the other.
	for i := range list.Items {
		item := &list.Items[i]
		last := statusOf(item)
		if item.GetDeletionTimestamp() != nil || last.Tenant == "" || last.DaemonSet == "" {
			continue
		}
		if _, claimed := o.claims[last.Tenant]; !claimed {
			o.claims[last.Tenant] = item.GetNamespace() + "/" + item.GetName()
		}
		if _, claimed := o.namespaces[item.GetNamespace()]; !claimed {
			o.namespaces[item.GetNamespace()] = item.GetName()
		}
		// Its nodes too: a running TenantNetwork keeps the nodes it selects.
		selector, _, _ := unstructured.NestedStringMap(item.Object, "spec", "nodeSelector")
		if _, err := labels.ValidatedSelectorFromSet(selector); err != nil {
			continue // apply reports the invalid selector
		}
		if nodes, err := o.listNodes(ctx, labels.SelectorFromSet(selector).String()); err == nil {
			o.claimNodes(nodes, item.GetNamespace()+"/"+item.GetName())
		}
	}
	for i := range list.Items {
		item := &list.Items[i]
		log := o.Log.WithValues("tenantNetwork", item.GetNamespace()+"/"+item.GetName())
		if err := o.reconcile(ctx, item, log); err != nil {
			log.Error(err, "cannot reconcile")
		}
	}
	o.cleanNodes(ctx, list.Items)
}

// reconcile brings the objects of one TenantNetwork to the wanted state and reports the result in its status.
func (o *Operator) reconcile(ctx context.Context, item *unstructured.Unstructured, log klog.Logger) error {
	if item.GetDeletionTimestamp() != nil {
		return o.remove(ctx, item, log)
	}

	status, err := o.apply(ctx, item, log)
	if err != nil {
		status.Phase, status.Message = PhaseError, err.Error()
		// Keep what was known before the failure: the DaemonSet may still run, and the next pass uses the
		// tenant and the DaemonSet of the status to keep the claim of this TenantNetwork.
		last := statusOf(item)
		if status.Tenant == "" && last.DaemonSet != "" {
			status.Tenant = last.Tenant
		}
		if status.DaemonSet == "" {
			status.DaemonSet, status.ManagedTopology = last.DaemonSet, last.ManagedTopology
		}
		o.eventOnce(ctx, item, "state", EventTypeError, reasonOf(err), upperFirst(err.Error()))
	} else {
		message := status.Message
		if status.Phase == PhaseReady {
			message = fmt.Sprintf("the tenant network works on every node of tenant %s (%d of %d)",
				status.Tenant, status.ReadyNodes, status.DesiredNodes)
		}
		o.eventOnce(ctx, item, "state", corev1.EventTypeNormal, status.Phase, upperFirst(message))
	}
	status.ObservedGeneration = item.GetGeneration()
	if statusErr := o.writeStatus(ctx, item, status); statusErr != nil {
		log.Error(statusErr, "cannot write the status")
	}
	return err
}

// apply creates or updates the objects of a TenantNetwork. The returned status is valid with an error too.
func (o *Operator) apply(ctx context.Context, item *unstructured.Unstructured, log klog.Logger) (TenantNetworkStatus, error) {
	var status TenantNetworkStatus
	namespace, name := item.GetNamespace(), item.GetName()

	var spec TenantNetworkSpec
	if raw, ok := item.Object["spec"].(map[string]interface{}); ok {
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(raw, &spec); err != nil {
			return status, withReason(ReasonInvalidSpec, fmt.Errorf("the spec is not valid: %w", err))
		}
	}
	if spec.RealmSecretName == "" {
		return status, withReason(ReasonInvalidSpec, fmt.Errorf("spec.realmSecretName is required"))
	}
	if len(name) > maxResourceNameLength {
		return status, withReason(ReasonInvalidSpec, fmt.Errorf("the name is longer than %d characters", maxResourceNameLength))
	}
	interval := o.Options.Interval
	if spec.Interval != "" {
		parsed, err := time.ParseDuration(spec.Interval)
		if err != nil || parsed < time.Second {
			return status, withReason(ReasonInvalidSpec,
				fmt.Errorf("the interval %q in spec.interval is not a duration of one second or longer", spec.Interval))
		}
		interval = parsed
	}

	if spec.VLANParentInterface != "" && !interfaceNamePattern.MatchString(spec.VLANParentInterface) {
		return status, withReason(ReasonInvalidSpec,
			fmt.Errorf("the name %q in spec.vlanParentInterface is not a valid interface name", spec.VLANParentInterface))
	}

	if spec.FirewalldZone != nil && *spec.FirewalldZone != "" && !zoneNamePattern.MatchString(*spec.FirewalldZone) {
		return status, withReason(ReasonInvalidSpec,
			fmt.Errorf("the name %q in spec.firewalldZone is not a valid firewalld zone name", *spec.FirewalldZone))
	}

	if err := o.ensureFinalizer(ctx, item); err != nil {
		return status, err
	}

	// The value of the ready label is the namespace, so one namespace holds the network of one tenant.
	if owner, claimed := o.namespaces[namespace]; claimed && owner != name {
		return status, withReason(ReasonTenantAlreadyServed,
			fmt.Errorf("namespace %q already has TenantNetwork %s; a namespace holds the network of one tenant", namespace, owner))
	}
	if o.namespaces != nil {
		o.namespaces[namespace] = name
	}

	tenant, err := o.resolveTenant(ctx, namespace, spec.RealmSecretName)
	if err != nil {
		return status, err
	}
	status.Tenant = tenant
	// A node is on one tenant network through one DaemonSet: a second one would undo the work of the first
	// when it is removed.
	if owner, claimed := o.claims[tenant]; claimed && owner != namespace+"/"+name {
		return status, withReason(ReasonTenantAlreadyServed,
			fmt.Errorf("tenant %q is already served by TenantNetwork %s", tenant, owner))
	}
	if o.claims != nil {
		o.claims[tenant] = namespace + "/" + name
	}
	status.ManagedTopology = &Topology{Key: ReadyLabelKey, Value: namespace}
	// An empty selector selects every node.
	if _, err := labels.ValidatedSelectorFromSet(spec.NodeSelector); err != nil {
		return status, withReason(ReasonInvalidSpec, fmt.Errorf("the node selector in spec.nodeSelector is not valid: %w", err))
	}
	selectorText := labels.SelectorFromSet(spec.NodeSelector).String()
	nodesText := "all nodes"
	if selectorText != "" {
		nodesText = "the nodes with the label " + selectorText
	}
	o.eventOnce(ctx, item, "secret", corev1.EventTypeNormal, ReasonSecretFound,
		fmt.Sprintf("Using Secret %s; its token belongs to tenant %s", spec.RealmSecretName, tenant))

	// A node can be on the network of one tenant only. Two DaemonSets on one node would overwrite the ready
	// label of each other, and the realm does not support two tenant networks on one node.
	nodes, err := o.listNodes(ctx, selectorText)
	if err != nil {
		return status, err
	}
	if node, owner := o.claimNodes(nodes, namespace+"/"+name); owner != "" {
		return status, withReason(ReasonNodeSelectorOverlap, fmt.Errorf(
			"node %q is already selected by TenantNetwork %s; a node can be on the network of one tenant only. "+
				"Set spec.nodeSelector so that the two select different nodes", node, owner))
	}

	objectName := namePrefix + name
	status.DaemonSet = objectName
	meta := metav1.ObjectMeta{
		Name:      objectName,
		Namespace: namespace,
		Labels:    map[string]string{"app": objectName, "product": productLabelValue, ownerLabelKey: name},
		OwnerReferences: []metav1.OwnerReference{{
			APIVersion: item.GetAPIVersion(), Kind: item.GetKind(), Name: name, UID: item.GetUID(),
		}},
	}
	if err := o.ensureRBAC(ctx, meta); err != nil {
		return status, err
	}

	wanted := o.daemonSet(meta, spec, tenant, interval)
	current, err := o.Kube.AppsV1().DaemonSets(namespace).Get(ctx, objectName, metav1.GetOptions{})
	switch {
	case apierrors.IsNotFound(err):
		if current, err = o.Kube.AppsV1().DaemonSets(namespace).Create(ctx, wanted, metav1.CreateOptions{}); err != nil {
			return status, fmt.Errorf("create the DaemonSet: %w", err)
		}
		log.Info("the DaemonSet was created", "daemonSet", objectName, "tenant", tenant)
		o.events(item).Event(ctx, corev1.EventTypeNormal, ReasonDaemonSetCreated, fmt.Sprintf(
			"Created DaemonSet %s for %s", objectName, nodesText))
	case err != nil:
		return status, fmt.Errorf("read the DaemonSet: %w", err)
	case current.Annotations[specHashAnnotation] != wanted.Annotations[specHashAnnotation]:
		wanted.ResourceVersion = current.ResourceVersion
		if current, err = o.Kube.AppsV1().DaemonSets(namespace).Update(ctx, wanted, metav1.UpdateOptions{}); err != nil {
			return status, fmt.Errorf("update the DaemonSet: %w", err)
		}
		log.Info("the DaemonSet was updated", "daemonSet", objectName, "tenant", tenant)
		o.events(item).Event(ctx, corev1.EventTypeNormal, ReasonDaemonSetUpdated, fmt.Sprintf(
			"Updated DaemonSet %s; its pods are replaced one node at a time", objectName))
	}

	status.DesiredNodes = int64(current.Status.DesiredNumberScheduled)
	status.ReadyNodes = int64(current.Status.NumberReady)
	if status.Nodes, status.Interface, err = o.nodeStatuses(ctx, selectorText, namespace); err != nil {
		return status, err
	}
	switch {
	case status.DesiredNodes == 0 && len(status.Nodes) > 0:
		// Nodes match the selector, but the DaemonSet has not counted them yet: it was created a moment ago.
		// Saying "no node" here would be wrong, and it would be reported as an event.
		status.Phase = PhasePending
		status.Message = "waiting for the pods to start on " + countNodes(len(status.Nodes)) +
			"; if this stays, see the events of DaemonSet " + objectName
	case status.DesiredNodes == 0:
		status.Phase = PhasePending
		status.Message = "no node to serve: the selector is for " + nodesText
	case status.ReadyNodes < status.DesiredNodes:
		status.Phase = PhasePending
		status.Message = fmt.Sprintf("the tenant network works on %d of %d nodes", status.ReadyNodes, status.DesiredNodes)
	default:
		status.Phase = PhaseReady
	}
	return status, nil
}

// remove deletes the DaemonSet of a TenantNetwork that is being deleted, waits until its pods are gone,
// and then lets the TenantNetwork go. A pod that sees its DaemonSet being deleted cleans up its node.
func (o *Operator) remove(ctx context.Context, item *unstructured.Unstructured, log klog.Logger) error {
	if !hasFinalizer(item) {
		return nil
	}
	namespace, objectName := item.GetNamespace(), namePrefix+item.GetName()

	daemonSet, err := o.Kube.AppsV1().DaemonSets(namespace).Get(ctx, objectName, metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("read the DaemonSet: %w", err)
	}
	if err == nil {
		if daemonSet.DeletionTimestamp == nil {
			foreground := metav1.DeletePropagationForeground
			err := o.Kube.AppsV1().DaemonSets(namespace).Delete(ctx, objectName, metav1.DeleteOptions{PropagationPolicy: &foreground})
			if err != nil && !apierrors.IsNotFound(err) {
				return fmt.Errorf("delete the DaemonSet: %w", err)
			}
			log.Info("the DaemonSet is being deleted; waiting for its pods to clean up", "daemonSet", objectName)
		}
		status := statusOf(item)
		status.Phase, status.Message = PhaseRemoving, "waiting for the pods to clean up their nodes"
		o.eventOnce(ctx, item, "state", corev1.EventTypeNormal, ReasonRemoving,
			"Deleting DaemonSet "+objectName+"; waiting for the pods to clean up their nodes")
		return o.writeStatus(ctx, item, status)
	}

	// The pods are gone. A pod that lost its permissions before it stopped (the whole namespace was deleted)
	// could not remove the ready label of its node, so remove what is left: a label without a pod behind it
	// would send workloads to a node that nothing keeps on the tenant network.
	left, err := o.Kube.CoreV1().Nodes().List(ctx, metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set{ReadyLabelKey: namespace}).String(),
	})
	if err != nil {
		return fmt.Errorf("list the nodes with the label %s=%s: %w", ReadyLabelKey, namespace, err)
	}
	for i := range left.Items {
		if err := o.cleanNode(ctx, &left.Items[i], true, true); err != nil {
			return err
		}
		// The log too: in a namespace that is being deleted, the event cannot be written.
		log.Info("removed a label that the pod of the node left behind", "node", left.Items[i].Name,
			"label", ReadyLabelKey+"="+namespace)
		o.events(item).Event(ctx, corev1.EventTypeNormal, ReasonNodeLabelRemoved, fmt.Sprintf(
			"Node %s: its pod left the label %s=%s behind; the operator removed it",
			left.Items[i].Name, ReadyLabelKey, namespace))
	}

	err = o.Kube.RbacV1().ClusterRoleBindings().Delete(ctx, clusterBindingName(namespace, objectName), metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete the ClusterRoleBinding: %w", err)
	}

	var finalizers []string
	for _, finalizer := range item.GetFinalizers() {
		if finalizer != Finalizer {
			finalizers = append(finalizers, finalizer)
		}
	}
	item.SetFinalizers(finalizers)
	if _, err := o.Dynamic.Resource(TenantNetworkGVR).Namespace(namespace).Update(ctx, item, metav1.UpdateOptions{}); err != nil &&
		!apierrors.IsNotFound(err) {
		return fmt.Errorf("remove the finalizer: %w", err)
	}
	delete(o.tenants, namespace+"/"+item.GetName())
	for _, kind := range []string{"state", "secret"} {
		delete(o.reported, string(item.GetUID())+"/"+kind)
	}
	log.Info("the pods are gone; the TenantNetwork was released")
	return nil
}

// hasFinalizer reports whether the TenantNetwork carries the finalizer of the operator.
func hasFinalizer(item *unstructured.Unstructured) bool {
	for _, finalizer := range item.GetFinalizers() {
		if finalizer == Finalizer {
			return true
		}
	}
	return false
}

// ensureFinalizer adds the finalizer of the operator to the TenantNetwork.
func (o *Operator) ensureFinalizer(ctx context.Context, item *unstructured.Unstructured) error {
	if hasFinalizer(item) {
		return nil
	}
	item.SetFinalizers(append(item.GetFinalizers(), Finalizer))
	updated, err := o.Dynamic.Resource(TenantNetworkGVR).Namespace(item.GetNamespace()).Update(ctx, item, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("add the finalizer: %w", err)
	}
	item.SetResourceVersion(updated.GetResourceVersion())
	return nil
}

// resolveTenant returns the name of the tenant that the token in the Secret belongs to.
// The answer of PanAPI is kept while the Secret does not change, for at most TenantTTL.
func (o *Operator) resolveTenant(ctx context.Context, namespace, secretName string) (string, error) {
	secret, err := o.Kube.CoreV1().Secrets(namespace).Get(ctx, secretName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return "", withReason(ReasonSecretNotFound, fmt.Errorf("the Secret %q does not exist in namespace %q", secretName, namespace))
	}
	if err != nil {
		return "", fmt.Errorf("read the Secret %q: %w", secretName, err)
	}

	key := namespace + "/" + secretName
	if cached, ok := o.tenants[key]; ok && cached.secretVersion == secret.ResourceVersion && time.Since(cached.at) < o.TenantTTL {
		return cached.tenant, nil
	}

	secrets := make(map[string]string, len(secret.Data))
	for name, value := range secret.Data {
		secrets[name] = strings.TrimSpace(string(value))
	}
	network, err := o.API.GetTenantNetwork(ctx, secrets)
	if err != nil {
		return "", withReason(ReasonPanAPIError,
			fmt.Errorf("read the tenant of the token in Secret %q from PanAPI: %w", secretName, err))
	}
	if network == nil || network.TenantName == "" {
		return "", withReason(ReasonPanAPIError, fmt.Errorf("PanAPI returned no tenant name for the token in Secret %q", secretName))
	}
	if o.tenants == nil {
		o.tenants = make(map[string]cachedTenant)
	}
	o.tenants[key] = cachedTenant{secretVersion: secret.ResourceVersion, tenant: network.TenantName, at: time.Now()}
	return network.TenantName, nil
}

// clusterBindingName returns the name of the ClusterRoleBinding of one TenantNetwork.
func clusterBindingName(namespace, objectName string) string {
	return namespace + "-" + objectName
}

// ensureRBAC creates the ServiceAccount of the pods and its permissions:
// read and patch nodes (the ready label), read its own DaemonSet (to tell a removal from a restart), and write events.
func (o *Operator) ensureRBAC(ctx context.Context, meta metav1.ObjectMeta) error {
	namespace, name := meta.Namespace, meta.Name

	account := &corev1.ServiceAccount{ObjectMeta: meta}
	if _, err := o.Kube.CoreV1().ServiceAccounts(namespace).Create(ctx, account, metav1.CreateOptions{}); err != nil &&
		!apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create the ServiceAccount: %w", err)
	}

	role := &rbacv1.Role{
		ObjectMeta: meta,
		Rules: []rbacv1.PolicyRule{
			{APIGroups: []string{"apps"}, Resources: []string{"daemonsets"}, ResourceNames: []string{name}, Verbs: []string{"get"}},
			// The pods report what they do as events on the TenantNetwork.
			{APIGroups: []string{""}, Resources: []string{"events"}, Verbs: []string{"create"}},
		},
	}
	roles := o.Kube.RbacV1().Roles(namespace)
	if current, err := roles.Get(ctx, name, metav1.GetOptions{}); apierrors.IsNotFound(err) {
		if _, err := roles.Create(ctx, role, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("create the Role: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("read the Role: %w", err)
	} else if !equality.Semantic.DeepEqual(current.Rules, role.Rules) {
		role.ResourceVersion = current.ResourceVersion
		if _, err := roles.Update(ctx, role, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("update the Role: %w", err)
		}
	}

	subjects := []rbacv1.Subject{{Kind: rbacv1.ServiceAccountKind, Name: name, Namespace: namespace}}
	binding := &rbacv1.RoleBinding{
		ObjectMeta: meta,
		Subjects:   subjects,
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "Role", Name: name},
	}
	if _, err := o.Kube.RbacV1().RoleBindings(namespace).Create(ctx, binding, metav1.CreateOptions{}); err != nil &&
		!apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create the RoleBinding: %w", err)
	}

	// A ClusterRoleBinding has no namespace, so the TenantNetwork cannot own it; remove() deletes it.
	clusterBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: clusterBindingName(namespace, name), Labels: meta.Labels},
		Subjects:   subjects,
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: o.Options.ClusterRole},
	}
	clusterBindings := o.Kube.RbacV1().ClusterRoleBindings()
	current, err := clusterBindings.Get(ctx, clusterBinding.Name, metav1.GetOptions{})
	if err == nil && (current.RoleRef != clusterBinding.RoleRef || !equality.Semantic.DeepEqual(current.Subjects, subjects)) {
		// The role of a binding cannot be changed. A binding to another role is left over from an operator
		// that was installed under another name; replace it, or the pods cannot set the ready label.
		if err := clusterBindings.Delete(ctx, clusterBinding.Name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("delete the outdated ClusterRoleBinding: %w", err)
		}
		err = apierrors.NewNotFound(rbacv1.Resource("clusterrolebindings"), clusterBinding.Name)
	}
	if apierrors.IsNotFound(err) {
		if _, err := clusterBindings.Create(ctx, clusterBinding, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("create the ClusterRoleBinding: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("read the ClusterRoleBinding: %w", err)
	}
	return nil
}

// daemonSet returns the DaemonSet of the node program for one tenant.
func (o *Operator) daemonSet(meta metav1.ObjectMeta, spec TenantNetworkSpec, tenant string, interval time.Duration) *appsv1.DaemonSet {
	firewalldZone := o.Options.FirewalldZone
	if spec.FirewalldZone != nil {
		firewalldZone = *spec.FirewalldZone
	}
	detach := o.Options.DetachOnRemoval
	if spec.DetachOnRemoval != nil {
		detach = *spec.DetachOnRemoval
	}
	vlanParent := o.Options.VLANParent
	if spec.VLANParentInterface != "" {
		vlanParent = spec.VLANParentInterface
	}

	fieldEnv := func(name, path string) corev1.EnvVar {
		return corev1.EnvVar{Name: name, ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: path}}}
	}
	var pullSecrets []corev1.LocalObjectReference
	for _, name := range o.Options.ImagePullSecrets {
		pullSecrets = append(pullSecrets, corev1.LocalObjectReference{Name: name})
	}
	selector := map[string]string{"app": meta.Name}
	// The tenant name is not a label: PanFS does not promise that it is a valid label value.
	podLabels := map[string]string{"app": meta.Name, "product": productLabelValue}
	no, grace, maxUnavailable := false, int64(60), intstr.FromInt32(1)

	daemonSet := &appsv1.DaemonSet{
		ObjectMeta: *meta.DeepCopy(),
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: selector},
			// One node at a time: a pod removes the ready label of its node when it stops.
			UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
				Type:          appsv1.RollingUpdateDaemonSetStrategyType,
				RollingUpdate: &appsv1.RollingUpdateDaemonSet{MaxUnavailable: &maxUnavailable},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: podLabels},
				Spec: corev1.PodSpec{
					// The tenant interface is an interface of the node.
					HostNetwork:                   true,
					TerminationGracePeriodSeconds: &grace,
					ServiceAccountName:            meta.Name,
					PriorityClassName:             o.Options.PriorityClassName,
					ImagePullSecrets:              pullSecrets,
					Tolerations:                   []corev1.Toleration{{Operator: corev1.TolerationOpExists}},
					// Only the nodes an administrator chose for this tenant.
					NodeSelector: spec.NodeSelector,
					Containers: []corev1.Container{{
						Name:            "tenant-network",
						Image:           o.Options.Image,
						ImagePullPolicy: corev1.PullPolicy(o.Options.PullPolicy),
						Args: []string{
							"--v=" + o.Options.LogLevel,
							"--interval=" + interval.String(),
							"--firewalld-zone=" + firewalldZone,
							fmt.Sprintf("--detach-on-removal=%t", detach),
							"--vlan-parent=" + vlanParent,
						},
						Env: []corev1.EnvVar{
							{Name: "TENANT", Value: tenant},
							// The value of the ready label: the namespace of the TenantNetwork
							{Name: "READY_VALUE", Value: meta.Namespace},
							fieldEnv("NODE_NAME", "spec.nodeName"),
							fieldEnv("POD_NAMESPACE", "metadata.namespace"),
							{Name: "DAEMONSET_NAME", Value: meta.Name},
							// The TenantNetwork the pod reports its events on
							{Name: "TENANT_NETWORK_NAME", Value: meta.Labels[ownerLabelKey]},
							{Name: "TENANT_NETWORK_UID", Value: string(meta.OwnerReferences[0].UID)},
							fieldEnv("HOST_IP", "status.hostIP"),
						},
						// The pod is Ready only while a tenant director answers on the tenant interface.
						ReadinessProbe: &corev1.Probe{
							ProbeHandler:     corev1.ProbeHandler{Exec: &corev1.ExecAction{Command: []string{"test", "-f", "/tmp/ready"}}},
							PeriodSeconds:    10,
							FailureThreshold: 1,
						},
						// NET_ADMIN changes host interfaces, NET_RAW sends ICMP, and SELinux lets only
						// an unconfined container type talk to firewalld over the D-Bus socket of the host.
						SecurityContext: &corev1.SecurityContext{
							AllowPrivilegeEscalation: &no,
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{"ALL"},
								Add:  []corev1.Capability{"NET_ADMIN", "NET_RAW"},
							},
							SELinuxOptions: &corev1.SELinuxOptions{Type: "spc_t"},
						},
						VolumeMounts: []corev1.VolumeMount{
							{Name: "realm-secret", MountPath: secretMountPath, ReadOnly: true},
							{Name: "host-dbus", MountPath: "/run/dbus"},
						},
					}},
					Volumes: []corev1.Volume{
						{Name: "realm-secret", VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{SecretName: spec.RealmSecretName},
						}},
						{Name: "host-dbus", VolumeSource: corev1.VolumeSource{
							HostPath: &corev1.HostPathVolumeSource{Path: "/run/dbus"},
						}},
					},
				},
			},
		},
	}

	// The hash covers everything the operator decides, so a change of an option or of the tenant is an update.
	encoded, _ := json.Marshal(daemonSet.Spec)
	sum := sha256.Sum256(encoded)
	daemonSet.Annotations = map[string]string{specHashAnnotation: hex.EncodeToString(sum[:8])}
	return daemonSet
}

// statusOf returns the status stored in a TenantNetwork.
func statusOf(item *unstructured.Unstructured) TenantNetworkStatus {
	var status TenantNetworkStatus
	if raw, ok := item.Object["status"].(map[string]interface{}); ok {
		_ = runtime.DefaultUnstructuredConverter.FromUnstructured(raw, &status)
	}
	return status
}

// writeStatus stores the status of a TenantNetwork when it differs from the stored one.
func (o *Operator) writeStatus(ctx context.Context, item *unstructured.Unstructured, status TenantNetworkStatus) error {
	if equality.Semantic.DeepEqual(statusOf(item), status) {
		return nil
	}
	raw, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&status)
	if err != nil {
		return err
	}
	updated := item.DeepCopy()
	updated.Object["status"] = raw
	_, err = o.Dynamic.Resource(TenantNetworkGVR).Namespace(item.GetNamespace()).UpdateStatus(ctx, updated, metav1.UpdateOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

// listNodes returns the nodes that a selector matches. A selector is listed once per pass.
func (o *Operator) listNodes(ctx context.Context, selector string) (*corev1.NodeList, error) {
	if list, ok := o.nodeLists[selector]; ok {
		return list, nil
	}
	list, err := o.Kube.CoreV1().Nodes().List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, fmt.Errorf("list the nodes with the label %q: %w", selector, err)
	}
	if o.nodeLists != nil {
		o.nodeLists[selector] = list
	}
	return list, nil
}

// claimNodes records that a TenantNetwork selects the nodes. When another TenantNetwork already selects one of
// them, nothing is recorded, and that node and its owner are returned.
func (o *Operator) claimNodes(nodes *corev1.NodeList, key string) (node, owner string) {
	for i := range nodes.Items {
		if current := o.nodeOwners[nodes.Items[i].Name]; current != "" && current != key {
			return nodes.Items[i].Name, current
		}
	}
	if o.nodeOwners != nil {
		for i := range nodes.Items {
			o.nodeOwners[nodes.Items[i].Name] = key
		}
	}
	return "", ""
}

// nodeStatuses returns the state of every node that the selector matches, by node name, and the name of the
// tenant interface. The node pods report both on their node: the ready label, and annotations.
func (o *Operator) nodeStatuses(ctx context.Context, selector, readyValue string) (map[string]NodeStatus, string, error) {
	list, err := o.listNodes(ctx, selector)
	if err != nil {
		return nil, "", err
	}
	var nodes map[string]NodeStatus
	iface := ""
	for i := range list.Items {
		node := &list.Items[i]
		status := NodeStatus{
			State:     NodeStateNotReady,
			OverlayIP: node.Annotations[OverlayAnnotation],
			Message:   node.Annotations[MessageAnnotation],
		}
		if node.Labels[ReadyLabelKey] == readyValue {
			status.State, status.Message = NodeStateReady, ""
		} else if status.Message == "" {
			status.Message = "the pod of the node has not reported yet"
		}
		if nodes == nil {
			nodes = make(map[string]NodeStatus, len(list.Items))
		}
		nodes[node.Name] = status
		// The interface is named after the segment of the tenant, so it is the same on every node.
		if name := node.Annotations[InterfaceAnnotation]; name != "" && (iface == "" || name < iface) {
			iface = name
		}
	}
	return nodes, iface, nil
}

// cleanNodes removes the ready label that no TenantNetwork stands behind any more, with the annotations of the
// node pods. A pod removes them itself when its node leaves the tenant, but it cannot when it loses its
// permissions first: the namespace was deleted as a whole, or the finalizer was removed by hand.
// A label is left over when the namespace it names has no TenantNetwork at all. While a TenantNetwork exists
// there, in any state, its pods and remove look after the label. The annotations stay when another
// TenantNetwork selects the node, because the pod of that one writes them.
// Nothing is removed on doubt: a selector that cannot be read counts as a match, and a failed read ends the pass.
func (o *Operator) cleanNodes(ctx context.Context, items []unstructured.Unstructured) {
	namespaces := make(map[string]bool, len(items))
	selectors := make([]labels.Selector, 0, len(items))
	for i := range items {
		namespaces[items[i].GetNamespace()] = true
		selector := labels.Everything()
		if set, _, err := unstructured.NestedStringMap(items[i].Object, "spec", "nodeSelector"); err == nil {
			if valid, err := labels.ValidatedSelectorFromSet(set); err == nil {
				selector = valid
			}
		}
		selectors = append(selectors, selector)
	}

	// Not through listNodes: its lists are as old as the start of the pass, and a label that was set or
	// removed since then must be seen here.
	nodes, err := o.Kube.CoreV1().Nodes().List(ctx, metav1.ListOptions{LabelSelector: ReadyLabelKey})
	if err != nil {
		o.Log.Error(err, "cannot look for labels that were left on nodes")
		return
	}
	for i := range nodes.Items {
		node := &nodes.Items[i]
		value := node.Labels[ReadyLabelKey]
		if namespaces[value] {
			continue
		}
		selected := false
		for _, selector := range selectors {
			selected = selected || selector.Matches(labels.Set(node.Labels))
		}
		if err := o.cleanNode(ctx, node, true, !selected); err != nil {
			o.Log.Error(err, "cannot clean up the node", "node", node.Name)
			continue
		}
		o.Log.Info("removed a label that was left on the node: its namespace has no TenantNetwork",
			"node", node.Name, "label", ReadyLabelKey+"="+value)
	}
}

// cleanNode removes the ready label, the annotations of the node pods, or both, from a node.
func (o *Operator) cleanNode(ctx context.Context, node *corev1.Node, label, annotations bool) error {
	metadata := map[string]any{}
	if label {
		metadata["labels"] = map[string]any{ReadyLabelKey: nil}
	}
	if annotations {
		metadata["annotations"] = map[string]any{OverlayAnnotation: nil, InterfaceAnnotation: nil, MessageAnnotation: nil}
	}
	// A merge patch with null removes a key.
	patch, err := json.Marshal(map[string]any{"metadata": metadata})
	if err != nil {
		return err
	}
	if _, err := o.Kube.CoreV1().Nodes().Patch(ctx, node.Name, types.MergePatchType, patch, metav1.PatchOptions{}); err != nil {
		return fmt.Errorf("remove the label %s from node %s: %w", ReadyLabelKey, node.Name, err)
	}
	return nil
}

// events returns the sink for events about a TenantNetwork.
func (o *Operator) events(item *unstructured.Unstructured) *EventSink {
	return &EventSink{
		Kube:      o.Kube,
		Ref:       tenantNetworkRef(item.GetNamespace(), item.GetName(), item.GetUID()),
		Component: "panfs-tenant-network-operator",
		Log:       o.Log,
	}
}

// eventOnce writes an event of one kind unless the last event of that kind said the same.
func (o *Operator) eventOnce(ctx context.Context, item *unstructured.Unstructured, kind, eventType, reason, message string) {
	key, value := string(item.GetUID())+"/"+kind, reason+"\x00"+message
	if o.reported[key] == value {
		return
	}
	if o.reported == nil {
		o.reported = make(map[string]string)
	}
	o.reported[key] = value
	o.events(item).Event(ctx, eventType, reason, message)
}

// countNodes returns the number of nodes as text: "1 node", "3 nodes".
func countNodes(n int) string {
	if n == 1 {
		return "1 node"
	}
	return fmt.Sprintf("%d nodes", n)
}

// upperFirst returns the text with its first letter in upper case.
func upperFirst(text string) string {
	if text == "" {
		return text
	}
	return strings.ToUpper(text[:1]) + text[1:]
}
