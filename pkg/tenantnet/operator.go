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
	// SecretName names a Secret in the namespace of the resource with the keys api, token and, optionally, ca_cert.
	// The tenant is the one the token belongs to.
	SecretName string `json:"secretName"`
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
	// ReadyLabel is the label that the pods set on a node where the network of the tenant works:
	// panfs.vdura.com/tenant-network=<namespace of the resource>.
	ReadyLabel string `json:"readyLabel,omitempty"`

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
	for i := range list.Items {
		item := &list.Items[i]
		log := o.Log.WithValues("tenantNetwork", item.GetNamespace()+"/"+item.GetName())
		if err := o.reconcile(ctx, item, log); err != nil {
			log.Error(err, "cannot reconcile")
		}
	}
}

// reconcile brings the objects of one TenantNetwork to the wanted state and reports the result in its status.
func (o *Operator) reconcile(ctx context.Context, item *unstructured.Unstructured, log klog.Logger) error {
	if item.GetDeletionTimestamp() != nil {
		return o.remove(ctx, item, log)
	}

	status, err := o.apply(ctx, item, log)
	if err != nil {
		status.Phase, status.Message = PhaseError, err.Error()
		o.eventOnce(ctx, item, "state", corev1.EventTypeWarning, reasonOf(err), upperFirst(err.Error()))
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
	if spec.SecretName == "" {
		return status, withReason(ReasonInvalidSpec, fmt.Errorf("spec.secretName is required"))
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

	if err := o.ensureFinalizer(ctx, item); err != nil {
		return status, err
	}

	// The value of the ready label is the namespace, so one namespace holds the network of one tenant.
	if owner, claimed := o.namespaces[namespace]; claimed {
		return status, withReason(ReasonTenantAlreadyServed,
			fmt.Errorf("namespace %q already has TenantNetwork %s; a namespace holds the network of one tenant", namespace, owner))
	}
	if o.namespaces != nil {
		o.namespaces[namespace] = name
	}

	tenant, err := o.resolveTenant(ctx, namespace, spec.SecretName)
	if err != nil {
		return status, err
	}
	status.Tenant = tenant
	// A node is on one tenant network through one DaemonSet: a second one would undo the work of the first
	// when it is removed.
	if owner, claimed := o.claims[tenant]; claimed {
		return status, withReason(ReasonTenantAlreadyServed,
			fmt.Errorf("tenant %q is already served by TenantNetwork %s", tenant, owner))
	}
	if o.claims != nil {
		o.claims[tenant] = namespace + "/" + name
	}
	status.ReadyLabel = ReadyLabelKey + "=" + namespace
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
		fmt.Sprintf("Using Secret %s; its token belongs to tenant %s", spec.SecretName, tenant))

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
		return "", withReason(ReasonSecretNotFound, fmt.Errorf("Secret %q does not exist in namespace %q", secretName, namespace))
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
	if _, err := o.Kube.RbacV1().ClusterRoleBindings().Create(ctx, clusterBinding, metav1.CreateOptions{}); err != nil &&
		!apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create the ClusterRoleBinding: %w", err)
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
	podLabels := map[string]string{"app": meta.Name, "product": productLabelValue, TenantLabelKey: tenant}
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
							Secret: &corev1.SecretVolumeSource{SecretName: spec.SecretName},
						}},
						{Name: "host-dbus", VolumeSource: corev1.VolumeSource{
							HostPath: &corev1.HostPathVolumeSource{Path: "/run/dbus"},
						}},
					},
				},
			},
		},
	}
	daemonSet.Labels[TenantLabelKey] = tenant

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

// nodeStatuses returns the state of every node that the selector matches, by node name, and the name of the
// tenant interface. The node pods report both on their node: the ready label, and annotations.
func (o *Operator) nodeStatuses(ctx context.Context, selector, readyValue string) (map[string]NodeStatus, string, error) {
	list, err := o.Kube.CoreV1().Nodes().List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, "", fmt.Errorf("list the nodes with the label %s: %w", selector, err)
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

// upperFirst returns the text with its first letter in upper case.
func upperFirst(text string) string {
	if text == "" {
		return text
	}
	return strings.ToUpper(text[:1]) + text[1:]
}
