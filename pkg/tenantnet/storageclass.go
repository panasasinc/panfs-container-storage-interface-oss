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
	"reflect"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
)

// PanfsStorageClassGVR names the PanfsStorageClass custom resource.
var PanfsStorageClassGVR = schema.GroupVersionResource{Group: "panfs.vdura.com", Version: "v1alpha1", Resource: "panfsstorageclasses"}

const (
	// PanfsStorageClassKind is the kind of the PanfsStorageClass custom resource.
	PanfsStorageClassKind = "PanfsStorageClass"
	// StorageClassFinalizer keeps a PanfsStorageClass until the operator deleted its StorageClass.
	StorageClassFinalizer = "panfs.vdura.com/storage-class"
	// StorageClassOwnerAnnotation is set on a StorageClass that the operator created. Its value is the
	// namespace and the name of the PanfsStorageClass, as "namespace/name". A StorageClass has no namespace,
	// so the PanfsStorageClass cannot own it through an owner reference.
	StorageClassOwnerAnnotation = "panfs.vdura.com/storage-class-owner"
	// DefaultClassAnnotation marks the default StorageClass of a cluster.
	DefaultClassAnnotation = "storageclass.kubernetes.io/is-default-class"

	// Provisioner is the name of the PanFS CSI driver.
	Provisioner = "com.vdura.csi.panfs"
	// CSIDriverNamespaceAnnotation is the annotation of the CSIDriver object that names the namespace of the driver.
	CSIDriverNamespaceAnnotation = "csi-driver-namespace"
	// controllerAccountSuffix ends the name of the ServiceAccount of the CSI controller, which reads the realm Secret.
	// The driver chart names the account after its namespace: "<namespace>-controller".
	controllerAccountSuffix = "-controller"

	// secretReaderPrefix starts the name of the Role and the RoleBinding that let the CSI controller read the Secret.
	secretReaderPrefix = "panfs-storage-class-"
	// reservedParameterPrefix starts the StorageClass parameters that the operator sets itself.
	reservedParameterPrefix = "csi.storage.k8s.io/"
)

// Reasons of the events on a PanfsStorageClass.
const (
	ReasonStorageClassCreated  = "StorageClassCreated"
	ReasonStorageClassUpdated  = "StorageClassUpdated"
	ReasonStorageClassDeleted  = "StorageClassDeleted"
	ReasonSecretReaderKept     = "SecretReaderKept"
	ReasonTenantNetworkPending = "TenantNetworkNotReady"

	ReasonTenantNetworkNotFound = "TenantNetworkNotFound"
	ReasonStorageClassConflict  = "StorageClassConflict"
	ReasonImmutableField        = "ImmutableField"
	ReasonDriverNotFound        = "DriverNotFound"
)

// PanfsStorageClassSpec is the spec of a PanfsStorageClass. Exactly one of TenantNetwork and RealmSecretName is required.
type PanfsStorageClassSpec struct {
	// TenantNetwork names a TenantNetwork in the namespace of the resource. The StorageClass uses its Secret,
	// and its volumes are bound to the nodes where the network of the tenant works.
	TenantNetwork string `json:"tenantNetwork,omitempty"`
	// RealmSecretName names a Secret in the namespace of the resource with the keys api and token,
	// for a realm that is reachable without a tenant network.
	RealmSecretName string `json:"realmSecretName,omitempty"`
	// Parameters are the PanFS parameters of new volumes. They reach the StorageClass unchanged.
	Parameters map[string]string `json:"-"`
	// AllowVolumeExpansion defaults to true.
	AllowVolumeExpansion *bool `json:"allowVolumeExpansion,omitempty"`
	// ReclaimPolicy is Delete (the default) or Retain.
	ReclaimPolicy string `json:"reclaimPolicy,omitempty"`
	// VolumeBindingMode is Immediate or WaitForFirstConsumer. The default is Immediate with a tenant network,
	// and WaitForFirstConsumer without one.
	VolumeBindingMode string `json:"volumeBindingMode,omitempty"`
	// MountOptions are the mount options of the volumes.
	MountOptions []string `json:"mountOptions,omitempty"`
	// Default makes the StorageClass the default one of the cluster.
	Default bool `json:"default,omitempty"`
}

// PanfsStorageClassStatus is the status of a PanfsStorageClass.
type PanfsStorageClassStatus struct {
	Phase   string `json:"phase"`
	Message string `json:"message,omitempty"`
	// StorageClass is the name of the StorageClass that the operator created for the resource.
	StorageClass       string `json:"storageClass,omitempty"`
	ObservedGeneration int64  `json:"observedGeneration,omitempty"`
}

// ReconcileStorageClasses runs one pass over the PanfsStorageClasses of all namespaces.
func (o *Operator) ReconcileStorageClasses(ctx context.Context) {
	list, err := o.Dynamic.Resource(PanfsStorageClassGVR).Namespace(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		o.Log.Error(err, "cannot list the PanfsStorageClasses")
		return
	}
	// A StorageClass has no namespace: the oldest of two resources with one name gets it.
	sort.SliceStable(list.Items, func(i, j int) bool {
		a, b := list.Items[i].GetCreationTimestamp(), list.Items[j].GetCreationTimestamp()
		return a.Before(&b)
	})
	for i := range list.Items {
		item := &list.Items[i]
		log := o.Log.WithValues("panfsStorageClass", item.GetNamespace()+"/"+item.GetName())
		if err := o.reconcileStorageClass(ctx, item, log); err != nil {
			log.Error(err, "cannot reconcile")
		}
	}
}

// reconcileStorageClass brings the StorageClass of one resource to the wanted state and reports the result in its status.
func (o *Operator) reconcileStorageClass(ctx context.Context, item *unstructured.Unstructured, log klog.Logger) error {
	if item.GetDeletionTimestamp() != nil {
		return o.removeStorageClass(ctx, item, log)
	}

	status, err := o.applyStorageClass(ctx, item, log)
	if err != nil {
		status.Phase, status.Message = PhaseError, err.Error()
		o.classEventOnce(ctx, item, EventTypeError, reasonOf(err), upperFirst(err.Error()))
	} else {
		reason := status.Phase
		if status.Phase == PhasePending {
			reason = ReasonTenantNetworkPending
		}
		o.classEventOnce(ctx, item, corev1.EventTypeNormal, reason, upperFirst(status.Message))
	}
	status.ObservedGeneration = item.GetGeneration()
	if statusErr := o.writeStorageClassStatus(ctx, item, status); statusErr != nil {
		log.Error(statusErr, "cannot write the status")
	}
	return err
}

// classSource is what a PanfsStorageClass takes from its TenantNetwork, or from its own spec without one.
type classSource struct {
	secretName string
	// topology is the node label of the tenant network; nil without a tenant network.
	topology *Topology
	// waiting says why the StorageClass cannot be created yet; empty when it can.
	waiting string
	// note is added to the status message of an existing StorageClass.
	note string
}

// applyStorageClass creates or updates the StorageClass of a resource. The returned status is valid with an error too.
func (o *Operator) applyStorageClass(ctx context.Context, item *unstructured.Unstructured, log klog.Logger) (PanfsStorageClassStatus, error) {
	var status PanfsStorageClassStatus
	namespace, name := item.GetNamespace(), item.GetName()
	owner := namespace + "/" + name

	spec, err := storageClassSpecOf(item)
	if err != nil {
		return status, err
	}
	source, err := o.classSource(ctx, namespace, spec)
	if err != nil {
		return status, err
	}
	wanted := wantedStorageClass(name, owner, namespace, spec, source)

	if _, err := o.Kube.CoreV1().Secrets(namespace).Get(ctx, source.secretName, metav1.GetOptions{}); apierrors.IsNotFound(err) {
		return status, withReason(ReasonSecretNotFound,
			fmt.Errorf("the Secret %q does not exist in namespace %q", source.secretName, namespace))
	} else if err != nil {
		return status, fmt.Errorf("read the Secret %q: %w", source.secretName, err)
	}

	classes := o.Kube.StorageV1().StorageClasses()
	current, err := classes.Get(ctx, name, metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return status, fmt.Errorf("read the StorageClass: %w", err)
	}
	exists := err == nil
	if exists {
		switch currentOwner := current.Annotations[StorageClassOwnerAnnotation]; {
		case currentOwner == "":
			return status, withReason(ReasonStorageClassConflict, fmt.Errorf(
				"the StorageClass %q already exists and was not created by the operator; "+
					"use another name, or delete that StorageClass first", name))
		case currentOwner != owner:
			return status, withReason(ReasonStorageClassConflict, fmt.Errorf(
				"the StorageClass %q already belongs to PanfsStorageClass %s; a StorageClass has no namespace, "+
					"so the name must be unique in the cluster", name, currentOwner))
		}
		status.StorageClass = name
		if source.topology == nil && spec.TenantNetwork != "" {
			// The TenantNetwork does not report its topology at the moment: keep what the StorageClass has.
			wanted.AllowedTopologies = current.AllowedTopologies
		}
		if changed := immutableChanges(current, wanted); len(changed) > 0 {
			return status, withReason(ReasonImmutableField, fmt.Errorf(
				"%s cannot change after the StorageClass was created; the StorageClass %q is unchanged. "+
					"Undo the change, or delete this resource and create it again", strings.Join(changed, " and "), name))
		}
	} else if source.waiting != "" {
		status.Phase, status.Message = PhasePending, source.waiting
		return status, nil
	}

	// The finalizer comes before the first object: a resource that was refused above owns nothing, and one
	// that owns a Role or a StorageClass is never deleted without its cleanup. A finalizer that was removed
	// by hand comes back here.
	if err := o.ensureClassFinalizer(ctx, item); err != nil {
		return status, err
	}
	// The CSI controller must read the Secret before it sees the StorageClass.
	if err := o.ensureSecretReader(ctx, item, source.secretName); err != nil {
		return status, err
	}

	if !exists {
		if _, err := classes.Create(ctx, wanted, metav1.CreateOptions{}); err != nil {
			return status, fmt.Errorf("create the StorageClass: %w", err)
		}
		log.Info("the StorageClass was created", "storageClass", name)
		o.classEvents(item).Event(ctx, corev1.EventTypeNormal, ReasonStorageClassCreated, "Created StorageClass "+name)
	} else if mutableChanged(current, wanted) {
		updated := current.DeepCopy()
		updated.AllowVolumeExpansion, updated.MountOptions = wanted.AllowVolumeExpansion, wanted.MountOptions
		updated.AllowedTopologies = wanted.AllowedTopologies
		updated.Annotations[DefaultClassAnnotation] = wanted.Annotations[DefaultClassAnnotation]
		updated.Labels = wanted.Labels
		if _, err := classes.Update(ctx, updated, metav1.UpdateOptions{}); err != nil {
			return status, fmt.Errorf("update the StorageClass: %w", err)
		}
		log.Info("the StorageClass was updated", "storageClass", name)
		o.classEvents(item).Event(ctx, corev1.EventTypeNormal, ReasonStorageClassUpdated, "Updated StorageClass "+name)
	}

	status.Phase, status.StorageClass = PhaseReady, name
	status.Message = fmt.Sprintf("the StorageClass %s is in place", name)
	if source.note != "" {
		status.Message += "; " + source.note
	}
	return status, nil
}

// storageClassSpecOf reads and checks the spec of a PanfsStorageClass.
func storageClassSpecOf(item *unstructured.Unstructured) (PanfsStorageClassSpec, error) {
	var spec PanfsStorageClassSpec
	raw, _ := item.Object["spec"].(map[string]interface{})
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(raw, &spec); err != nil {
		return spec, withReason(ReasonInvalidSpec, fmt.Errorf("the spec is not valid: %w", err))
	}
	if (spec.TenantNetwork == "") == (spec.RealmSecretName == "") {
		return spec, withReason(ReasonInvalidSpec,
			fmt.Errorf("exactly one of spec.tenantNetwork and spec.realmSecretName is required"))
	}

	// A parameter value may be written as a number; a StorageClass holds strings.
	parameters, _ := raw["parameters"].(map[string]interface{})
	spec.Parameters = make(map[string]string, len(parameters))
	for key, value := range parameters {
		if strings.HasPrefix(key, reservedParameterPrefix) {
			return spec, withReason(ReasonInvalidSpec, fmt.Errorf(
				"the parameter %q in spec.parameters is set by the operator; remove it", key))
		}
		switch typed := value.(type) {
		case string:
			spec.Parameters[key] = typed
		case int64, int32, int:
			spec.Parameters[key] = fmt.Sprintf("%d", typed)
		default:
			return spec, withReason(ReasonInvalidSpec, fmt.Errorf(
				"the parameter %q in spec.parameters is not a string or a whole number", key))
		}
	}

	switch storagev1.VolumeBindingMode(spec.VolumeBindingMode) {
	case "":
		spec.VolumeBindingMode = string(storagev1.VolumeBindingWaitForFirstConsumer)
		if spec.TenantNetwork != "" {
			spec.VolumeBindingMode = string(storagev1.VolumeBindingImmediate)
		}
	case storagev1.VolumeBindingImmediate:
	case storagev1.VolumeBindingWaitForFirstConsumer:
		if spec.TenantNetwork != "" {
			// The provisioner compares allowedTopologies with the labels that the node plugin reported, when a
			// node is already selected. The label of the tenant network is not one of them, so no volume
			// would be created.
			return spec, withReason(ReasonInvalidSpec, fmt.Errorf(
				"spec.volumeBindingMode WaitForFirstConsumer cannot be used with spec.tenantNetwork; "+
					"use Immediate, which is the default: the volume is bound to the nodes of the tenant network"))
		}
	default:
		return spec, withReason(ReasonInvalidSpec, fmt.Errorf(
			"spec.volumeBindingMode %q is not Immediate or WaitForFirstConsumer", spec.VolumeBindingMode))
	}

	switch corev1.PersistentVolumeReclaimPolicy(spec.ReclaimPolicy) {
	case "":
		spec.ReclaimPolicy = string(corev1.PersistentVolumeReclaimDelete)
	case corev1.PersistentVolumeReclaimDelete, corev1.PersistentVolumeReclaimRetain:
	default:
		return spec, withReason(ReasonInvalidSpec, fmt.Errorf("spec.reclaimPolicy %q is not Delete or Retain", spec.ReclaimPolicy))
	}
	return spec, nil
}

// classSource returns the Secret and the topology of a PanfsStorageClass.
func (o *Operator) classSource(ctx context.Context, namespace string, spec PanfsStorageClassSpec) (classSource, error) {
	if spec.TenantNetwork == "" {
		return classSource{secretName: spec.RealmSecretName}, nil
	}

	network, err := o.Dynamic.Resource(TenantNetworkGVR).Namespace(namespace).Get(ctx, spec.TenantNetwork, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return classSource{}, withReason(ReasonTenantNetworkNotFound,
			fmt.Errorf("the TenantNetwork %q does not exist in namespace %q", spec.TenantNetwork, namespace))
	}
	if err != nil {
		return classSource{}, fmt.Errorf("read the TenantNetwork %q: %w", spec.TenantNetwork, err)
	}
	source := classSource{}
	source.secretName, _, _ = unstructured.NestedString(network.Object, "spec", "realmSecretName")
	if source.secretName == "" {
		return classSource{}, withReason(ReasonInvalidSpec,
			fmt.Errorf("the TenantNetwork %q names no Secret in spec.realmSecretName", spec.TenantNetwork))
	}

	networkStatus := statusOf(network)
	if topology := networkStatus.ManagedTopology; topology != nil && topology.Key != "" && topology.Value != "" {
		source.topology = topology
	}
	if networkStatus.Phase != PhaseReady || source.topology == nil || network.GetDeletionTimestamp() != nil {
		state := networkStatus.Phase
		if state == "" {
			state = "not reconciled yet"
		}
		if networkStatus.Message != "" {
			state += ": " + networkStatus.Message
		}
		source.waiting = fmt.Sprintf("waiting for the TenantNetwork %q to be ready (%s)", spec.TenantNetwork, state)
		source.note = fmt.Sprintf("the TenantNetwork %q is not ready (%s)", spec.TenantNetwork, state)
	}
	return source, nil
}

// wantedStorageClass builds the StorageClass of a PanfsStorageClass.
func wantedStorageClass(name, owner, namespace string, spec PanfsStorageClassSpec, source classSource) *storagev1.StorageClass {
	parameters := map[string]string{}
	for _, use := range []string{"provisioner", "node-publish", "controller-expand"} {
		parameters[reservedParameterPrefix+use+"-secret-name"] = source.secretName
		parameters[reservedParameterPrefix+use+"-secret-namespace"] = namespace
	}
	for key, value := range spec.Parameters {
		parameters[key] = value
	}

	expansion := true
	if spec.AllowVolumeExpansion != nil {
		expansion = *spec.AllowVolumeExpansion
	}
	reclaim := corev1.PersistentVolumeReclaimPolicy(spec.ReclaimPolicy)
	binding := storagev1.VolumeBindingMode(spec.VolumeBindingMode)
	class := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: map[string]string{"product": productLabelValue},
			Annotations: map[string]string{
				StorageClassOwnerAnnotation: owner,
				DefaultClassAnnotation:      fmt.Sprintf("%t", spec.Default),
			},
		},
		Provisioner:          Provisioner,
		Parameters:           parameters,
		ReclaimPolicy:        &reclaim,
		VolumeBindingMode:    &binding,
		AllowVolumeExpansion: &expansion,
		MountOptions:         spec.MountOptions,
	}
	if source.topology != nil {
		class.AllowedTopologies = []corev1.TopologySelectorTerm{{
			MatchLabelExpressions: []corev1.TopologySelectorLabelRequirement{{
				Key: source.topology.Key, Values: []string{source.topology.Value},
			}},
		}}
	}
	return class
}

// immutableChanges names the fields of the spec that differ from the StorageClass and that a StorageClass cannot change.
func immutableChanges(current, wanted *storagev1.StorageClass) []string {
	var changed []string
	if !reflect.DeepEqual(current.Parameters, wanted.Parameters) {
		// The Secret is part of the parameters.
		secretKey := reservedParameterPrefix + "provisioner-secret-name"
		if current.Parameters[secretKey] != wanted.Parameters[secretKey] {
			changed = append(changed, "the Secret (spec.tenantNetwork or spec.realmSecretName)")
		} else {
			changed = append(changed, "spec.parameters")
		}
	}
	if current.ReclaimPolicy != nil && *current.ReclaimPolicy != *wanted.ReclaimPolicy {
		changed = append(changed, fmt.Sprintf("spec.reclaimPolicy (the StorageClass has %s)", *current.ReclaimPolicy))
	}
	if current.VolumeBindingMode != nil && *current.VolumeBindingMode != *wanted.VolumeBindingMode {
		changed = append(changed, fmt.Sprintf("spec.volumeBindingMode (the StorageClass has %s)", *current.VolumeBindingMode))
	}
	return changed
}

// mutableChanged reports whether a field differs that a StorageClass can change.
func mutableChanged(current, wanted *storagev1.StorageClass) bool {
	return !equality.Semantic.DeepEqual(current.AllowVolumeExpansion, wanted.AllowVolumeExpansion) ||
		!equality.Semantic.DeepEqual(current.MountOptions, wanted.MountOptions) ||
		!equality.Semantic.DeepEqual(current.AllowedTopologies, wanted.AllowedTopologies) ||
		current.Annotations[DefaultClassAnnotation] != wanted.Annotations[DefaultClassAnnotation] ||
		!equality.Semantic.DeepEqual(current.Labels, wanted.Labels)
}

// controllerAccount returns the ServiceAccount of the CSI controller as a subject. Its name is the option of the
// operator, or the name that the driver chart gives it: "<namespace of the driver>-controller".
func (o *Operator) controllerAccount(ctx context.Context) (rbacv1.Subject, error) {
	namespace, err := o.driverNamespace(ctx)
	if err != nil {
		return rbacv1.Subject{}, err
	}
	name := o.Options.CSIControllerServiceAccount
	if name == "" {
		name = namespace + controllerAccountSuffix
	}
	return rbacv1.Subject{Kind: rbacv1.ServiceAccountKind, Name: name, Namespace: namespace}, nil
}

// driverNamespace returns the namespace of the CSI driver: the option of the operator, or the annotation
// that the driver chart sets on the CSIDriver object.
func (o *Operator) driverNamespace(ctx context.Context) (string, error) {
	if o.Options.CSIDriverNamespace != "" {
		return o.Options.CSIDriverNamespace, nil
	}
	driver, err := o.Kube.StorageV1().CSIDrivers().Get(ctx, Provisioner, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return "", withReason(ReasonDriverNotFound, fmt.Errorf("the PanFS CSI driver is not installed: there is no CSIDriver %s", Provisioner))
	}
	if err != nil {
		return "", fmt.Errorf("read the CSIDriver %s: %w", Provisioner, err)
	}
	namespace := driver.Annotations[CSIDriverNamespaceAnnotation]
	if namespace == "" {
		return "", withReason(ReasonDriverNotFound, fmt.Errorf(
			"the CSIDriver %s has no annotation %s; set the namespace of the driver in the options of the operator",
			Provisioner, CSIDriverNamespaceAnnotation))
	}
	return namespace, nil
}

// ensureSecretReader lets the CSI controller read the Secret of a PanfsStorageClass, with a Role and a RoleBinding
// in the namespace of the Secret. The resource does not own them: the volumes of the StorageClass need the Secret
// for an expansion and for their deletion, also after the resource is gone. See removeStorageClass.
func (o *Operator) ensureSecretReader(ctx context.Context, item *unstructured.Unstructured, secretName string) error {
	account, err := o.controllerAccount(ctx)
	if err != nil {
		return err
	}
	namespace, name := item.GetNamespace(), secretReaderPrefix+item.GetName()
	meta := metav1.ObjectMeta{
		Name:      name,
		Namespace: namespace,
		Labels:    map[string]string{"product": productLabelValue},
	}

	role := &rbacv1.Role{
		ObjectMeta: meta,
		Rules: []rbacv1.PolicyRule{
			{APIGroups: []string{""}, Resources: []string{"secrets"}, ResourceNames: []string{secretName}, Verbs: []string{"get"}},
		},
	}
	roles := o.Kube.RbacV1().Roles(namespace)
	if current, err := roles.Get(ctx, name, metav1.GetOptions{}); apierrors.IsNotFound(err) {
		if _, err := roles.Create(ctx, role, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("create the Role: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("read the Role: %w", err)
	} else {
		// A Role that was kept for the volumes of a deleted resource of this name may name another Secret.
		// Those volumes still need it, so the Secret is added and nothing is taken away.
		role.Rules[0].ResourceNames = secretNames(current.Rules, secretName)
		if !equality.Semantic.DeepEqual(current.Rules, role.Rules) {
			role.ResourceVersion = current.ResourceVersion
			if _, err := roles.Update(ctx, role, metav1.UpdateOptions{}); err != nil {
				return fmt.Errorf("update the Role: %w", err)
			}
		}
	}

	binding := &rbacv1.RoleBinding{
		ObjectMeta: meta,
		Subjects:   []rbacv1.Subject{account},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "Role", Name: name},
	}
	bindings := o.Kube.RbacV1().RoleBindings(namespace)
	if current, err := bindings.Get(ctx, name, metav1.GetOptions{}); apierrors.IsNotFound(err) {
		if _, err := bindings.Create(ctx, binding, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("create the RoleBinding: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("read the RoleBinding: %w", err)
	} else if !equality.Semantic.DeepEqual(current.Subjects, binding.Subjects) {
		// The driver moved to another namespace.
		binding.ResourceVersion = current.ResourceVersion
		if _, err := bindings.Update(ctx, binding, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("update the RoleBinding: %w", err)
		}
	}
	return nil
}

// secretNames returns the Secrets that the rules of a Role name, and the given one, sorted.
func secretNames(rules []rbacv1.PolicyRule, secretName string) []string {
	names := map[string]bool{secretName: true}
	for _, rule := range rules {
		for _, name := range rule.ResourceNames {
			names[name] = true
		}
	}
	sorted := make([]string, 0, len(names))
	for name := range names {
		sorted = append(sorted, name)
	}
	sort.Strings(sorted)
	return sorted
}

// removeStorageClass deletes the StorageClass of a resource that is being deleted, and then lets the resource go.
// Existing volumes keep working without their StorageClass. The Role and the RoleBinding for the Secret stay
// while a volume of the StorageClass exists: the CSI controller reads the Secret to expand and to delete a volume.
func (o *Operator) removeStorageClass(ctx context.Context, item *unstructured.Unstructured, log klog.Logger) error {
	if !hasFinalizerNamed(item, StorageClassFinalizer) {
		return nil
	}
	namespace, name := item.GetNamespace(), item.GetName()

	classes := o.Kube.StorageV1().StorageClasses()
	current, err := classes.Get(ctx, name, metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("read the StorageClass: %w", err)
	}
	// Only its own StorageClass: one of the same name that belongs to another resource stays.
	if err == nil && current.Annotations[StorageClassOwnerAnnotation] == namespace+"/"+name {
		if err := classes.Delete(ctx, name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("delete the StorageClass: %w", err)
		}
		log.Info("the StorageClass was deleted", "storageClass", name)
		o.classEvents(item).Event(ctx, corev1.EventTypeNormal, ReasonStorageClassDeleted,
			"Deleted StorageClass "+name+"; its volumes keep working")
	}

	// The Role and the RoleBinding, also when the StorageClass is already gone: they can exist without it,
	// after a failed creation or after somebody deleted the StorageClass.
	reader := secretReaderPrefix + name
	_, err = o.Kube.RbacV1().Roles(namespace).Get(ctx, reader, metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("read the Role: %w", err)
	}
	if err == nil {
		volumes, err := o.Kube.CoreV1().PersistentVolumes().List(ctx, metav1.ListOptions{})
		if err != nil {
			return fmt.Errorf("list the volumes of the StorageClass: %w", err)
		}
		left := 0
		for i := range volumes.Items {
			volume := &volumes.Items[i]
			if volume.Spec.StorageClassName == name && volume.Spec.CSI != nil && volume.Spec.CSI.Driver == Provisioner {
				left++
			}
		}
		if left > 0 {
			log.Info("the Role and the RoleBinding for the Secret stay: volumes of the StorageClass exist",
				"role", reader, "volumes", left)
			o.classEvents(item).Event(ctx, corev1.EventTypeNormal, ReasonSecretReaderKept, fmt.Sprintf(
				"Kept Role and RoleBinding %s: the CSI controller needs the Secret for %s of the StorageClass. "+
					"Delete them by hand when the volumes are gone", reader, countVolumes(left)))
		} else {
			if err := o.Kube.RbacV1().RoleBindings(namespace).Delete(ctx, reader, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
				return fmt.Errorf("delete the RoleBinding: %w", err)
			}
			if err := o.Kube.RbacV1().Roles(namespace).Delete(ctx, reader, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
				return fmt.Errorf("delete the Role: %w", err)
			}
		}
	}

	var finalizers []string
	for _, finalizer := range item.GetFinalizers() {
		if finalizer != StorageClassFinalizer {
			finalizers = append(finalizers, finalizer)
		}
	}
	item.SetFinalizers(finalizers)
	if _, err := o.Dynamic.Resource(PanfsStorageClassGVR).Namespace(namespace).Update(ctx, item, metav1.UpdateOptions{}); err != nil &&
		!apierrors.IsNotFound(err) {
		return fmt.Errorf("remove the finalizer: %w", err)
	}
	delete(o.reported, string(item.GetUID())+"/class")
	return nil
}

// countVolumes returns the number of volumes as text: "1 volume", "3 volumes".
func countVolumes(n int) string {
	if n == 1 {
		return "1 volume"
	}
	return fmt.Sprintf("%d volumes", n)
}

// hasFinalizerNamed reports whether the object carries the finalizer.
func hasFinalizerNamed(item *unstructured.Unstructured, name string) bool {
	for _, finalizer := range item.GetFinalizers() {
		if finalizer == name {
			return true
		}
	}
	return false
}

// ensureClassFinalizer adds the finalizer of the operator to the PanfsStorageClass.
func (o *Operator) ensureClassFinalizer(ctx context.Context, item *unstructured.Unstructured) error {
	if hasFinalizerNamed(item, StorageClassFinalizer) {
		return nil
	}
	item.SetFinalizers(append(item.GetFinalizers(), StorageClassFinalizer))
	updated, err := o.Dynamic.Resource(PanfsStorageClassGVR).Namespace(item.GetNamespace()).Update(ctx, item, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("add the finalizer: %w", err)
	}
	item.SetResourceVersion(updated.GetResourceVersion())
	return nil
}

// storageClassStatusOf reads the status of a PanfsStorageClass.
func storageClassStatusOf(item *unstructured.Unstructured) PanfsStorageClassStatus {
	var status PanfsStorageClassStatus
	if raw, ok := item.Object["status"].(map[string]interface{}); ok {
		_ = runtime.DefaultUnstructuredConverter.FromUnstructured(raw, &status)
	}
	return status
}

// writeStorageClassStatus stores the status of a PanfsStorageClass when it differs from the stored one.
func (o *Operator) writeStorageClassStatus(ctx context.Context, item *unstructured.Unstructured, status PanfsStorageClassStatus) error {
	if equality.Semantic.DeepEqual(storageClassStatusOf(item), status) {
		return nil
	}
	raw, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&status)
	if err != nil {
		return err
	}
	updated := item.DeepCopy()
	updated.Object["status"] = raw
	_, err = o.Dynamic.Resource(PanfsStorageClassGVR).Namespace(item.GetNamespace()).UpdateStatus(ctx, updated, metav1.UpdateOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

// classEvents returns the sink for events about a PanfsStorageClass.
func (o *Operator) classEvents(item *unstructured.Unstructured) *EventSink {
	return &EventSink{
		Kube: o.Kube,
		Ref: corev1.ObjectReference{
			APIVersion: PanfsStorageClassGVR.GroupVersion().String(), Kind: PanfsStorageClassKind,
			Namespace: item.GetNamespace(), Name: item.GetName(), UID: item.GetUID(),
		},
		Component: "panfs-tenant-network-operator",
		Log:       o.Log,
	}
}

// classEventOnce writes the state of a PanfsStorageClass as an event unless the last one said the same.
func (o *Operator) classEventOnce(ctx context.Context, item *unstructured.Unstructured, eventType, reason, message string) {
	key, value := string(item.GetUID())+"/class", reason+"\x00"+message
	if o.reported[key] == value {
		return
	}
	if o.reported == nil {
		o.reported = make(map[string]string)
	}
	o.reported[key] = value
	o.classEvents(item).Event(ctx, eventType, reason, message)
}
