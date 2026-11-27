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

package storageclass

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/klog/v2"

	"github.com/panasasinc/panfs-container-storage-interface-oss/pkg/tenantnet"
)

const (
	testNamespace = "tenant-ns"
	testClass     = "fast"
	testReader    = secretReaderPrefix + testClass
)

// newTenantNetwork returns a TenantNetwork named acme with the given spec.
func newTenantNetwork(spec map[string]interface{}) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": tenantnet.TenantNetworkGVR.GroupVersion().String(),
		"kind":       tenantnet.TenantNetworkKind,
		"metadata":   map[string]interface{}{"name": "acme", "namespace": testNamespace, "uid": "uid-1"},
		"spec":       spec,
	}}
}

// newClass returns a PanfsStorageClass named fast in the given namespace.
func newClass(namespace string, spec map[string]interface{}) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": PanfsStorageClassGVR.GroupVersion().String(),
		"kind":       PanfsStorageClassKind,
		"metadata":   map[string]interface{}{"name": testClass, "namespace": namespace, "uid": "class-" + namespace},
		"spec":       spec,
	}}
}

// readyNetwork returns the TenantNetwork acme with the Secret realm, in the given phase.
func readyNetwork(phase string) *unstructured.Unstructured {
	network := newTenantNetwork(map[string]interface{}{"realmSecretName": "realm"})
	network.Object["status"] = map[string]interface{}{
		"phase":           phase,
		"message":         "the tenant network works on 1 of 2 nodes",
		"desiredNodes":    int64(2),
		"readyNodes":      int64(1),
		"managedTopology": map[string]interface{}{"key": tenantnet.ReadyLabelKey, "value": testNamespace},
	}
	return network
}

// newClassOperator returns an operator over fake clients that hold the given custom resources, a realm Secret
// and the CSIDriver object of a driver in the namespace csi-panfs.
func newClassOperator(items ...runtime.Object) (*Controller, *fake.Clientset) {
	kinds := map[schema.GroupVersionResource]string{
		tenantnet.TenantNetworkGVR: tenantnet.TenantNetworkKind + "List",
		PanfsStorageClassGVR:       PanfsStorageClassKind + "List",
	}
	kube := fake.NewSimpleClientset(
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "realm", Namespace: testNamespace}},
		&storagev1.CSIDriver{ObjectMeta: metav1.ObjectMeta{
			Name: Provisioner, Annotations: map[string]string{CSIDriverNamespaceAnnotation: "csi-panfs"},
		}},
	)
	return &Controller{
		Kube:    kube,
		Dynamic: dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), kinds, items...),
		Log:     klog.NewKlogr(),
	}, kube
}

// storedClass returns the PanfsStorageClass as the fake API server holds it, and its status.
func storedClass(t *testing.T, o *Controller, namespace string) (*unstructured.Unstructured, PanfsStorageClassStatus) {
	t.Helper()
	item, err := o.Dynamic.Resource(PanfsStorageClassGVR).Namespace(namespace).Get(context.Background(), testClass, metav1.GetOptions{})
	require.NoError(t, err)
	return item, storageClassStatusOf(item)
}

// updateClassSpec replaces the spec of the stored PanfsStorageClass.
func updateClassSpec(t *testing.T, o *Controller, spec map[string]interface{}) {
	t.Helper()
	item, _ := storedClass(t, o, testNamespace)
	item.Object["spec"] = spec
	_, err := o.Dynamic.Resource(PanfsStorageClassGVR).Namespace(testNamespace).Update(context.Background(), item, metav1.UpdateOptions{})
	require.NoError(t, err)
}

func assertNoStorageClass(t *testing.T, kube *fake.Clientset) {
	t.Helper()
	_, err := kube.StorageV1().StorageClasses().Get(context.Background(), testClass, metav1.GetOptions{})
	assert.True(t, apierrors.IsNotFound(err), "no StorageClass is expected")
	_, err = kube.RbacV1().Roles(testNamespace).Get(context.Background(), testReader, metav1.GetOptions{})
	assert.True(t, apierrors.IsNotFound(err), "no Role is expected")
}

func TestStorageClassFromATenantNetwork(t *testing.T) {
	ctx := context.Background()
	o, kube := newClassOperator(readyNetwork(tenantnet.PhaseReady), newClass(testNamespace, map[string]interface{}{"tenantNetwork": "acme"}))

	o.ReconcileAll(ctx)

	class, err := kube.StorageV1().StorageClasses().Get(ctx, testClass, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, Provisioner, class.Provisioner)
	// Only the secret references: without parameters in the resource, the driver defaults apply.
	assert.Equal(t, map[string]string{
		"csi.storage.k8s.io/provisioner-secret-name":            "realm",
		"csi.storage.k8s.io/provisioner-secret-namespace":       testNamespace,
		"csi.storage.k8s.io/node-publish-secret-name":           "realm",
		"csi.storage.k8s.io/node-publish-secret-namespace":      testNamespace,
		"csi.storage.k8s.io/controller-expand-secret-name":      "realm",
		"csi.storage.k8s.io/controller-expand-secret-namespace": testNamespace,
	}, class.Parameters)
	assert.Equal(t, []corev1.TopologySelectorTerm{{MatchLabelExpressions: []corev1.TopologySelectorLabelRequirement{
		{Key: tenantnet.ReadyLabelKey, Values: []string{testNamespace}},
	}}}, class.AllowedTopologies)
	assert.Equal(t, storagev1.VolumeBindingImmediate, *class.VolumeBindingMode)
	assert.Equal(t, corev1.PersistentVolumeReclaimDelete, *class.ReclaimPolicy)
	assert.True(t, *class.AllowVolumeExpansion)
	assert.Equal(t, testNamespace+"/"+testClass, class.Annotations[StorageClassOwnerAnnotation])
	assert.Equal(t, "false", class.Annotations[DefaultClassAnnotation])

	role, err := kube.RbacV1().Roles(testNamespace).Get(ctx, testReader, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, []string{"realm"}, role.Rules[0].ResourceNames)
	assert.Equal(t, []string{"get"}, role.Rules[0].Verbs)
	binding, err := kube.RbacV1().RoleBindings(testNamespace).Get(ctx, testReader, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "csi-panfs-controller", binding.Subjects[0].Name)
	assert.Equal(t, "csi-panfs", binding.Subjects[0].Namespace)

	item, status := storedClass(t, o, testNamespace)
	assert.Equal(t, []string{StorageClassFinalizer}, item.GetFinalizers())
	assert.Equal(t, tenantnet.PhaseReady, status.Phase)
	assert.Equal(t, testClass, status.StorageClass)
	assert.Equal(t, "the StorageClass fast is in place", status.Message)
}

func TestStorageClassParametersAndSettingsReachTheStorageClass(t *testing.T) {
	ctx := context.Background()
	o, kube := newClassOperator(readyNetwork(tenantnet.PhaseReady), newClass(testNamespace, map[string]interface{}{
		"tenantNetwork": "acme",
		"parameters": map[string]interface{}{
			"panfs.csi.vdura.com/layout":   "raid6+",
			"panfs.csi.vdura.com/maxwidth": int64(3),
		},
		"reclaimPolicy":        "Retain",
		"allowVolumeExpansion": false,
		"mountOptions":         []interface{}{"noatime"},
		"default":              true,
	}))

	o.ReconcileAll(ctx)

	class, err := kube.StorageV1().StorageClasses().Get(ctx, testClass, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "raid6+", class.Parameters["panfs.csi.vdura.com/layout"])
	assert.Equal(t, "3", class.Parameters["panfs.csi.vdura.com/maxwidth"])
	assert.Len(t, class.Parameters, 8)
	assert.Equal(t, corev1.PersistentVolumeReclaimRetain, *class.ReclaimPolicy)
	assert.False(t, *class.AllowVolumeExpansion)
	assert.Equal(t, []string{"noatime"}, class.MountOptions)
	assert.Equal(t, "true", class.Annotations[DefaultClassAnnotation])
}

func TestStorageClassWithoutATenantNetwork(t *testing.T) {
	ctx := context.Background()
	o, kube := newClassOperator(newClass(testNamespace, map[string]interface{}{"realmSecretName": "realm"}))

	o.ReconcileAll(ctx)

	class, err := kube.StorageV1().StorageClasses().Get(ctx, testClass, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Empty(t, class.AllowedTopologies)
	assert.Equal(t, storagev1.VolumeBindingWaitForFirstConsumer, *class.VolumeBindingMode)
	assert.Equal(t, "realm", class.Parameters["csi.storage.k8s.io/provisioner-secret-name"])
	_, status := storedClass(t, o, testNamespace)
	assert.Equal(t, tenantnet.PhaseReady, status.Phase)
}

func TestStorageClassIsNotCreatedWithoutItsSources(t *testing.T) {
	tests := []struct {
		name    string
		objects []runtime.Object
		spec    map[string]interface{}
		phase   string
		message string
	}{
		{
			name:    "the TenantNetwork is missing",
			spec:    map[string]interface{}{"tenantNetwork": "acme"},
			phase:   tenantnet.PhaseError,
			message: `the TenantNetwork "acme" does not exist in namespace "tenant-ns"`,
		},
		{
			name:    "the TenantNetwork is not ready",
			objects: []runtime.Object{readyNetwork(tenantnet.PhasePending)},
			spec:    map[string]interface{}{"tenantNetwork": "acme"},
			phase:   tenantnet.PhasePending,
			message: `waiting for the TenantNetwork "acme" to be ready (Pending: the tenant network works on 1 of 2 nodes)`,
		},
		{
			name:    "the Secret is missing",
			spec:    map[string]interface{}{"realmSecretName": "other"},
			phase:   tenantnet.PhaseError,
			message: `the Secret "other" does not exist in namespace "tenant-ns"`,
		},
		{
			name:    "both sources are set",
			objects: []runtime.Object{readyNetwork(tenantnet.PhaseReady)},
			spec:    map[string]interface{}{"tenantNetwork": "acme", "realmSecretName": "realm"},
			phase:   tenantnet.PhaseError,
			message: "exactly one of spec.tenantNetwork and spec.realmSecretName is required",
		},
		{
			name:    "no source is set",
			spec:    map[string]interface{}{},
			phase:   tenantnet.PhaseError,
			message: "exactly one of spec.tenantNetwork and spec.realmSecretName is required",
		},
		{
			name:    "the binding mode does not work with a tenant network",
			objects: []runtime.Object{readyNetwork(tenantnet.PhaseReady)},
			spec:    map[string]interface{}{"tenantNetwork": "acme", "volumeBindingMode": "WaitForFirstConsumer"},
			phase:   tenantnet.PhaseError,
			message: "spec.volumeBindingMode WaitForFirstConsumer cannot be used with spec.tenantNetwork; " +
				"use Immediate, which is the default: the volume is bound to the nodes of the tenant network",
		},
		{
			name: "a parameter of the operator is set",
			spec: map[string]interface{}{"realmSecretName": "realm", "parameters": map[string]interface{}{
				"csi.storage.k8s.io/provisioner-secret-name": "other",
			}},
			phase:   tenantnet.PhaseError,
			message: `the parameter "csi.storage.k8s.io/provisioner-secret-name" in spec.parameters is set by the operator; remove it`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			o, kube := newClassOperator(append(test.objects, newClass(testNamespace, test.spec))...)

			o.ReconcileAll(context.Background())

			assertNoStorageClass(t, kube)
			item, status := storedClass(t, o, testNamespace)
			assert.Equal(t, test.phase, status.Phase)
			assert.Equal(t, test.message, status.Message)
			assert.Empty(t, status.StorageClass)
			assert.Empty(t, item.GetFinalizers())
		})
	}
}

func TestStorageClassFailsWithoutTheDriver(t *testing.T) {
	ctx := context.Background()
	o, kube := newClassOperator(newClass(testNamespace, map[string]interface{}{"realmSecretName": "realm"}))
	require.NoError(t, kube.StorageV1().CSIDrivers().Delete(ctx, Provisioner, metav1.DeleteOptions{}))

	o.ReconcileAll(ctx)

	assertNoStorageClass(t, kube)
	_, status := storedClass(t, o, testNamespace)
	assert.Equal(t, tenantnet.PhaseError, status.Phase)
	assert.Contains(t, status.Message, "the PanFS CSI driver is not installed")

	// The option of the operator names the namespace of a driver that has no annotation.
	o.Options.CSIDriverNamespace = "storage"
	o.ReconcileAll(ctx)
	binding, err := kube.RbacV1().RoleBindings(testNamespace).Get(ctx, testReader, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "storage", binding.Subjects[0].Namespace)
	// The driver chart names the ServiceAccount after its namespace.
	assert.Equal(t, "storage-controller", binding.Subjects[0].Name)

	o.Options.CSIControllerServiceAccount = "custom"
	o.ReconcileAll(ctx)
	binding, err = kube.RbacV1().RoleBindings(testNamespace).Get(ctx, testReader, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "custom", binding.Subjects[0].Name)
}

func TestStorageClassKeepsWorkingWhenTheTenantNetworkIsNotReady(t *testing.T) {
	ctx := context.Background()
	network := readyNetwork(tenantnet.PhaseReady)
	o, kube := newClassOperator(network, newClass(testNamespace, map[string]interface{}{"tenantNetwork": "acme"}))
	o.ReconcileAll(ctx)

	// A node restarts: the TenantNetwork is Pending for a while.
	stored, err := o.Dynamic.Resource(tenantnet.TenantNetworkGVR).Namespace(testNamespace).Get(ctx, "acme", metav1.GetOptions{})
	require.NoError(t, err)
	require.NoError(t, unstructured.SetNestedField(stored.Object, tenantnet.PhasePending, "status", "phase"))
	_, err = o.Dynamic.Resource(tenantnet.TenantNetworkGVR).Namespace(testNamespace).Update(ctx, stored, metav1.UpdateOptions{})
	require.NoError(t, err)
	o.ReconcileAll(ctx)

	class, err := kube.StorageV1().StorageClasses().Get(ctx, testClass, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Len(t, class.AllowedTopologies, 1)
	_, status := storedClass(t, o, testNamespace)
	assert.Equal(t, tenantnet.PhaseReady, status.Phase)
	assert.Equal(t, `the StorageClass fast is in place; the TenantNetwork "acme" is not ready `+
		`(Pending: the tenant network works on 1 of 2 nodes)`, status.Message)
}

func TestStorageClassRefusesAChangeOfAFixedField(t *testing.T) {
	tests := []struct {
		name    string
		spec    map[string]interface{}
		message string
	}{
		{
			name: "parameters",
			spec: map[string]interface{}{"realmSecretName": "realm", "parameters": map[string]interface{}{"panfs.csi.vdura.com/layout": "raid5+"}},
			message: `spec.parameters cannot change after the StorageClass was created; the StorageClass "fast" is unchanged. ` +
				"Undo the change, or delete this resource and create it again",
		},
		{
			name: "reclaim policy and binding mode",
			spec: map[string]interface{}{"realmSecretName": "realm", "reclaimPolicy": "Retain", "volumeBindingMode": "Immediate"},
			message: "spec.reclaimPolicy (the StorageClass has Delete) and spec.volumeBindingMode (the StorageClass has WaitForFirstConsumer) " +
				`cannot change after the StorageClass was created; the StorageClass "fast" is unchanged. ` +
				"Undo the change, or delete this resource and create it again",
		},
		{
			name: "the Secret",
			spec: map[string]interface{}{"realmSecretName": "second"},
			message: "the Secret (spec.tenantNetwork or spec.realmSecretName) cannot change after the StorageClass was created; " +
				`the StorageClass "fast" is unchanged. Undo the change, or delete this resource and create it again`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			o, kube := newClassOperator(newClass(testNamespace, map[string]interface{}{"realmSecretName": "realm"}))
			_, err := kube.CoreV1().Secrets(testNamespace).Create(ctx,
				&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "second", Namespace: testNamespace}}, metav1.CreateOptions{})
			require.NoError(t, err)
			o.ReconcileAll(ctx)
			before, err := kube.StorageV1().StorageClasses().Get(ctx, testClass, metav1.GetOptions{})
			require.NoError(t, err)

			updateClassSpec(t, o, test.spec)
			o.ReconcileAll(ctx)

			after, err := kube.StorageV1().StorageClasses().Get(ctx, testClass, metav1.GetOptions{})
			require.NoError(t, err)
			assert.Equal(t, before, after)
			_, status := storedClass(t, o, testNamespace)
			assert.Equal(t, tenantnet.PhaseError, status.Phase)
			assert.Equal(t, test.message, status.Message)
			assert.Equal(t, testClass, status.StorageClass)
		})
	}
}

func TestStorageClassAppliesAChangeOfAFreeField(t *testing.T) {
	ctx := context.Background()
	o, kube := newClassOperator(newClass(testNamespace, map[string]interface{}{"realmSecretName": "realm"}))
	o.ReconcileAll(ctx)

	updateClassSpec(t, o, map[string]interface{}{
		"realmSecretName": "realm", "allowVolumeExpansion": false, "mountOptions": []interface{}{"noatime"}, "default": true,
	})
	o.ReconcileAll(ctx)

	class, err := kube.StorageV1().StorageClasses().Get(ctx, testClass, metav1.GetOptions{})
	require.NoError(t, err)
	assert.False(t, *class.AllowVolumeExpansion)
	assert.Equal(t, []string{"noatime"}, class.MountOptions)
	assert.Equal(t, "true", class.Annotations[DefaultClassAnnotation])
	_, status := storedClass(t, o, testNamespace)
	assert.Equal(t, tenantnet.PhaseReady, status.Phase)
}

func TestStorageClassNameIsTakenOnce(t *testing.T) {
	ctx := context.Background()
	first := newClass(testNamespace, map[string]interface{}{"realmSecretName": "realm"})
	first.SetCreationTimestamp(metav1.Unix(100, 0))
	second := newClass("other-ns", map[string]interface{}{"realmSecretName": "realm"})
	second.SetCreationTimestamp(metav1.Unix(200, 0))
	o, kube := newClassOperator(second, first)
	_, err := kube.CoreV1().Secrets("other-ns").Create(ctx,
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "realm", Namespace: "other-ns"}}, metav1.CreateOptions{})
	require.NoError(t, err)

	o.ReconcileAll(ctx)

	class, err := kube.StorageV1().StorageClasses().Get(ctx, testClass, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, testNamespace+"/"+testClass, class.Annotations[StorageClassOwnerAnnotation])
	item, status := storedClass(t, o, "other-ns")
	assert.Equal(t, tenantnet.PhaseError, status.Phase)
	assert.Equal(t, `the StorageClass "fast" already belongs to PanfsStorageClass tenant-ns/fast; `+
		"a StorageClass has no namespace, so the name must be unique in the cluster", status.Message)
	assert.Empty(t, item.GetFinalizers())

	// Deleting the refused resource leaves the StorageClass of the first one alone.
	require.NoError(t, o.Dynamic.Resource(PanfsStorageClassGVR).Namespace("other-ns").Delete(ctx, testClass, metav1.DeleteOptions{}))
	o.ReconcileAll(ctx)
	_, err = kube.StorageV1().StorageClasses().Get(ctx, testClass, metav1.GetOptions{})
	require.NoError(t, err)
}

func TestStorageClassOfSomebodyElseIsNotTakenOver(t *testing.T) {
	ctx := context.Background()
	o, kube := newClassOperator(newClass(testNamespace, map[string]interface{}{"realmSecretName": "realm"}))
	foreign := &storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: testClass}, Provisioner: "example.com/other"}
	_, err := kube.StorageV1().StorageClasses().Create(ctx, foreign, metav1.CreateOptions{})
	require.NoError(t, err)

	o.ReconcileAll(ctx)

	class, err := kube.StorageV1().StorageClasses().Get(ctx, testClass, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "example.com/other", class.Provisioner)
	_, status := storedClass(t, o, testNamespace)
	assert.Equal(t, tenantnet.PhaseError, status.Phase)
	assert.Contains(t, status.Message, "already exists and was not created by the operator")
}

// deleteClass marks the stored PanfsStorageClass as being deleted, like the API server does while a finalizer is set.
func deleteClass(t *testing.T, o *Controller) {
	t.Helper()
	item, _ := storedClass(t, o, testNamespace)
	now := metav1.Now()
	item.SetDeletionTimestamp(&now)
	_, err := o.Dynamic.Resource(PanfsStorageClassGVR).Namespace(testNamespace).Update(context.Background(), item, metav1.UpdateOptions{})
	require.NoError(t, err)
}

func TestStorageClassIsDeletedWithItsResource(t *testing.T) {
	ctx := context.Background()
	o, kube := newClassOperator(newClass(testNamespace, map[string]interface{}{"realmSecretName": "realm"}))
	o.ReconcileAll(ctx)

	deleteClass(t, o)
	o.ReconcileAll(ctx)

	_, err := kube.StorageV1().StorageClasses().Get(ctx, testClass, metav1.GetOptions{})
	assert.True(t, apierrors.IsNotFound(err))
	_, err = kube.RbacV1().Roles(testNamespace).Get(ctx, testReader, metav1.GetOptions{})
	assert.True(t, apierrors.IsNotFound(err))
	_, err = kube.RbacV1().RoleBindings(testNamespace).Get(ctx, testReader, metav1.GetOptions{})
	assert.True(t, apierrors.IsNotFound(err))
	item, _ := storedClass(t, o, testNamespace)
	assert.Empty(t, item.GetFinalizers())
}

func TestStorageClassLeavesTheSecretReaderToItsVolumes(t *testing.T) {
	ctx := context.Background()
	o, kube := newClassOperator(newClass(testNamespace, map[string]interface{}{"realmSecretName": "realm"}))
	o.ReconcileAll(ctx)
	volume := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: "pvc-1"},
		Spec: corev1.PersistentVolumeSpec{
			StorageClassName:       testClass,
			PersistentVolumeSource: corev1.PersistentVolumeSource{CSI: &corev1.CSIPersistentVolumeSource{Driver: Provisioner}},
		},
	}
	_, err := kube.CoreV1().PersistentVolumes().Create(ctx, volume, metav1.CreateOptions{})
	require.NoError(t, err)

	deleteClass(t, o)
	o.ReconcileAll(ctx)

	_, err = kube.StorageV1().StorageClasses().Get(ctx, testClass, metav1.GetOptions{})
	assert.True(t, apierrors.IsNotFound(err))
	// The CSI controller still reads the Secret to expand and to delete the volume.
	_, err = kube.RbacV1().Roles(testNamespace).Get(ctx, testReader, metav1.GetOptions{})
	require.NoError(t, err)
	_, err = kube.RbacV1().RoleBindings(testNamespace).Get(ctx, testReader, metav1.GetOptions{})
	require.NoError(t, err)
	item, _ := storedClass(t, o, testNamespace)
	assert.Empty(t, item.GetFinalizers())
}

func TestStorageClassOfSomebodyElseSurvivesTheDeletionOfTheResource(t *testing.T) {
	ctx := context.Background()
	o, kube := newClassOperator(newClass(testNamespace, map[string]interface{}{"realmSecretName": "realm"}))
	o.ReconcileAll(ctx)
	// Somebody replaced the StorageClass of the operator with one of their own.
	require.NoError(t, kube.StorageV1().StorageClasses().Delete(ctx, testClass, metav1.DeleteOptions{}))
	foreign := &storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: testClass}, Provisioner: "example.com/other"}
	_, err := kube.StorageV1().StorageClasses().Create(ctx, foreign, metav1.CreateOptions{})
	require.NoError(t, err)

	deleteClass(t, o)
	o.ReconcileAll(ctx)

	class, err := kube.StorageV1().StorageClasses().Get(ctx, testClass, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "example.com/other", class.Provisioner)
	item, _ := storedClass(t, o, testNamespace)
	assert.Empty(t, item.GetFinalizers())
}

func TestStorageClassFinalizerComesBack(t *testing.T) {
	ctx := context.Background()
	o, _ := newClassOperator(newClass(testNamespace, map[string]interface{}{"realmSecretName": "realm"}))
	o.ReconcileAll(ctx)
	item, _ := storedClass(t, o, testNamespace)
	item.SetFinalizers(nil)
	_, err := o.Dynamic.Resource(PanfsStorageClassGVR).Namespace(testNamespace).Update(ctx, item, metav1.UpdateOptions{})
	require.NoError(t, err)

	o.ReconcileAll(ctx)

	item, _ = storedClass(t, o, testNamespace)
	assert.Equal(t, []string{StorageClassFinalizer}, item.GetFinalizers())
}

func TestStorageClassSecretReaderIsRemovedWithoutTheStorageClass(t *testing.T) {
	ctx := context.Background()
	o, kube := newClassOperator(newClass(testNamespace, map[string]interface{}{"realmSecretName": "realm"}))
	o.ReconcileAll(ctx)
	// Somebody deleted the StorageClass by hand.
	require.NoError(t, kube.StorageV1().StorageClasses().Delete(ctx, testClass, metav1.DeleteOptions{}))

	deleteClass(t, o)
	o.ReconcileAll(ctx)

	_, err := kube.RbacV1().Roles(testNamespace).Get(ctx, testReader, metav1.GetOptions{})
	assert.True(t, apierrors.IsNotFound(err))
	_, err = kube.RbacV1().RoleBindings(testNamespace).Get(ctx, testReader, metav1.GetOptions{})
	assert.True(t, apierrors.IsNotFound(err))
	item, _ := storedClass(t, o, testNamespace)
	assert.Empty(t, item.GetFinalizers())
}

func TestStorageClassSecretReaderKeepsTheSecretOfOlderVolumes(t *testing.T) {
	ctx := context.Background()
	o, kube := newClassOperator(newClass(testNamespace, map[string]interface{}{"realmSecretName": "second"}))
	_, err := kube.CoreV1().Secrets(testNamespace).Create(ctx,
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "second", Namespace: testNamespace}}, metav1.CreateOptions{})
	require.NoError(t, err)
	// The Role of an earlier resource with this name was kept for its volumes; it names the Secret realm.
	kept := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: testReader, Namespace: testNamespace},
		Rules: []rbacv1.PolicyRule{
			{APIGroups: []string{""}, Resources: []string{"secrets"}, ResourceNames: []string{"realm"}, Verbs: []string{"get"}},
		},
	}
	_, err = kube.RbacV1().Roles(testNamespace).Create(ctx, kept, metav1.CreateOptions{})
	require.NoError(t, err)

	o.ReconcileAll(ctx)

	role, err := kube.RbacV1().Roles(testNamespace).Get(ctx, testReader, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, []string{"realm", "second"}, role.Rules[0].ResourceNames)
}
