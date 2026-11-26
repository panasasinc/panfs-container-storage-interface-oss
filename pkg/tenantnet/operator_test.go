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
	"errors"
	"testing"
	"time"

	"github.com/panasasinc/panfs-container-storage-interface-oss/pkg/panapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/klog/v2"
)

const (
	testNamespace = "tenant-ns"
	testObject    = namePrefix + "acme"
)

// newTenantNetwork returns a TenantNetwork named acme with the given spec.
func newTenantNetwork(spec map[string]interface{}) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": TenantNetworkGVR.GroupVersion().String(),
		"kind":       TenantNetworkKind,
		"metadata":   map[string]interface{}{"name": "acme", "namespace": testNamespace, "uid": "uid-1"},
		"spec":       spec,
	}}
}

// newTestOperator returns an operator over fake clients that hold the TenantNetwork and a realm Secret.
func newTestOperator(item *unstructured.Unstructured, api *fakeAPI) (*Operator, *fake.Clientset) {
	kinds := map[schema.GroupVersionResource]string{TenantNetworkGVR: TenantNetworkKind + "List"}
	kube := fake.NewSimpleClientset(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "realm", Namespace: testNamespace, ResourceVersion: "1"},
		Data:       map[string][]byte{"api": []byte("realm.example"), "token": []byte("secret-token\n")},
	})
	return &Operator{
		Kube:    kube,
		Dynamic: dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), kinds, item),
		API:     api,
		Options: NodeOptions{
			Image: "registry.example/panfs-tenant-network:1", PullPolicy: "IfNotPresent", ImagePullSecrets: []string{"pull"},
			Interval: 30 * time.Second, FirewalldZone: "trusted", DetachOnRemoval: true, LogLevel: "2", ClusterRole: "node-role",
		},
		Resync:    time.Second,
		TenantTTL: time.Minute,
		Log:       klog.NewKlogr(),
	}, kube
}

// stored returns the TenantNetwork as the fake API server holds it, and its status.
func stored(t *testing.T, o *Operator) (*unstructured.Unstructured, TenantNetworkStatus) {
	t.Helper()
	item, err := o.Dynamic.Resource(TenantNetworkGVR).Namespace(testNamespace).Get(context.Background(), "acme", metav1.GetOptions{})
	require.NoError(t, err)
	return item, statusOf(item)
}

func TestOperatorCreatesTheDaemonSetOfTheTenant(t *testing.T) {
	ctx := context.Background()
	api := &fakeAPI{network: &panapi.TenantNetwork{TenantName: "acme"}}
	o, kube := newTestOperator(newTenantNetwork(map[string]interface{}{"realmSecretName": "realm"}), api)

	o.ReconcileAll(ctx)

	daemonSet, err := kube.AppsV1().DaemonSets(testNamespace).Get(ctx, testObject, metav1.GetOptions{})
	require.NoError(t, err)
	pod := daemonSet.Spec.Template.Spec
	// No selector in the spec: the pods run on all nodes.
	assert.Empty(t, pod.NodeSelector)
	assert.Equal(t, "realm", pod.Volumes[0].Secret.SecretName)
	assert.Equal(t, testObject, pod.ServiceAccountName)
	assert.Equal(t, []corev1.LocalObjectReference{{Name: "pull"}}, pod.ImagePullSecrets)
	assert.Equal(t, "registry.example/panfs-tenant-network:1", pod.Containers[0].Image)
	assert.Contains(t, pod.Containers[0].Args, "--firewalld-zone=trusted")
	assert.Contains(t, pod.Containers[0].Args, "--detach-on-removal=true")
	assert.Contains(t, pod.Containers[0].Env, corev1.EnvVar{Name: "TENANT", Value: "acme"})
	assert.Contains(t, pod.Containers[0].Env, corev1.EnvVar{Name: "READY_VALUE", Value: testNamespace})
	assert.Contains(t, pod.Containers[0].Env, corev1.EnvVar{Name: "DAEMONSET_NAME", Value: testObject})
	assert.Equal(t, "uid-1", string(daemonSet.OwnerReferences[0].UID))

	_, err = kube.CoreV1().ServiceAccounts(testNamespace).Get(ctx, testObject, metav1.GetOptions{})
	require.NoError(t, err)
	role, err := kube.RbacV1().Roles(testNamespace).Get(ctx, testObject, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, []string{testObject}, role.Rules[0].ResourceNames)
	_, err = kube.RbacV1().RoleBindings(testNamespace).Get(ctx, testObject, metav1.GetOptions{})
	require.NoError(t, err)
	clusterBinding, err := kube.RbacV1().ClusterRoleBindings().Get(ctx, testNamespace+"-"+testObject, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "node-role", clusterBinding.RoleRef.Name)

	item, status := stored(t, o)
	assert.Equal(t, []string{Finalizer}, item.GetFinalizers())
	assert.Equal(t, PhasePending, status.Phase)
	assert.Equal(t, "acme", status.Tenant)
	assert.Equal(t, testObject, status.DaemonSet)
	assert.Equal(t, "no node to serve: the selector is for all nodes", status.Message)

	// The nodes report in: the status follows the DaemonSet.
	daemonSet.Status.DesiredNumberScheduled, daemonSet.Status.NumberReady = 3, 2
	_, err = kube.AppsV1().DaemonSets(testNamespace).UpdateStatus(ctx, daemonSet, metav1.UpdateOptions{})
	require.NoError(t, err)
	o.ReconcileAll(ctx)
	_, status = stored(t, o)
	assert.Equal(t, PhasePending, status.Phase)
	assert.Equal(t, "the tenant network works on 2 of 3 nodes", status.Message)

	daemonSet.Status.NumberReady = 3
	_, err = kube.AppsV1().DaemonSets(testNamespace).UpdateStatus(ctx, daemonSet, metav1.UpdateOptions{})
	require.NoError(t, err)
	o.ReconcileAll(ctx)
	_, status = stored(t, o)
	assert.Equal(t, TenantNetworkStatus{
		Phase: PhaseReady, Tenant: "acme", DaemonSet: testObject, DesiredNodes: 3, ReadyNodes: 3,
		ReadyLabel: ReadyLabelKey + "=" + testNamespace,
	}, status)
}

func TestOperatorUpdatesTheDaemonSetOnlyWhenTheWantedOneChanges(t *testing.T) {
	ctx := context.Background()
	api := &fakeAPI{network: &panapi.TenantNetwork{TenantName: "acme"}}
	o, kube := newTestOperator(newTenantNetwork(map[string]interface{}{"realmSecretName": "realm"}), api)
	updates := func() (n int) {
		for _, action := range kube.Actions() {
			if action.Matches("update", "daemonsets") {
				n++
			}
		}
		return n
	}

	o.ReconcileAll(ctx)
	o.ReconcileAll(ctx)
	assert.Zero(t, updates())

	// The spec of the tenant replaces a default of the operator.
	item, _ := stored(t, o)
	require.NoError(t, unstructured.SetNestedField(item.Object, "", "spec", "firewalldZone"))
	require.NoError(t, unstructured.SetNestedField(item.Object, false, "spec", "detachOnRemoval"))
	require.NoError(t, unstructured.SetNestedField(item.Object, "10s", "spec", "interval"))
	require.NoError(t, unstructured.SetNestedField(item.Object, "bond0", "spec", "vlanParentInterface"))
	_, err := o.Dynamic.Resource(TenantNetworkGVR).Namespace(testNamespace).Update(ctx, item, metav1.UpdateOptions{})
	require.NoError(t, err)
	o.ReconcileAll(ctx)
	assert.Equal(t, 1, updates())

	daemonSet, err := kube.AppsV1().DaemonSets(testNamespace).Get(ctx, testObject, metav1.GetOptions{})
	require.NoError(t, err)
	args := daemonSet.Spec.Template.Spec.Containers[0].Args
	assert.Contains(t, args, "--firewalld-zone=")
	assert.Contains(t, args, "--detach-on-removal=false")
	assert.Contains(t, args, "--interval=10s")
	assert.Contains(t, args, "--vlan-parent=bond0")
}

func TestOperatorReportsErrors(t *testing.T) {
	ctx := context.Background()
	cases := map[string]struct {
		spec    map[string]interface{}
		api     *fakeAPI
		message string
	}{
		"no secret name": {
			spec: map[string]interface{}{}, api: &fakeAPI{}, message: "spec.realmSecretName is required",
		},
		"missing secret": {
			spec: map[string]interface{}{"realmSecretName": "other"}, api: &fakeAPI{}, message: `Secret "other" does not exist`,
		},
		"PanAPI refuses the token": {
			spec: map[string]interface{}{"realmSecretName": "realm"}, api: &fakeAPI{err: errors.New("unauthenticated")},
			message: "unauthenticated",
		},
		"bad VLAN parent": {
			spec: map[string]interface{}{"realmSecretName": "realm", "vlanParentInterface": "eth0; reboot"},
			api:  &fakeAPI{}, message: "spec.vlanParentInterface is not a valid interface name",
		},
		"bad interval": {
			spec: map[string]interface{}{"realmSecretName": "realm", "interval": "soon"}, api: &fakeAPI{}, message: "spec.interval",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			o, kube := newTestOperator(newTenantNetwork(tc.spec), tc.api)
			o.ReconcileAll(ctx)

			_, status := stored(t, o)
			assert.Equal(t, PhaseError, status.Phase)
			assert.Contains(t, status.Message, tc.message)
			_, err := kube.AppsV1().DaemonSets(testNamespace).Get(ctx, testObject, metav1.GetOptions{})
			assert.True(t, apierrors.IsNotFound(err))
		})
	}
}

func TestOperatorAsksPanAPIAgainWhenTheSecretChanges(t *testing.T) {
	ctx := context.Background()
	api := &fakeAPI{network: &panapi.TenantNetwork{TenantName: "acme"}}
	o, kube := newTestOperator(newTenantNetwork(map[string]interface{}{"realmSecretName": "realm"}), api)
	o.ReconcileAll(ctx)

	// The same Secret: the stored answer is used.
	api.network = &panapi.TenantNetwork{TenantName: "other"}
	o.ReconcileAll(ctx)
	_, status := stored(t, o)
	assert.Equal(t, "acme", status.Tenant)

	secret, err := kube.CoreV1().Secrets(testNamespace).Get(ctx, "realm", metav1.GetOptions{})
	require.NoError(t, err)
	secret.ResourceVersion = "2"
	_, err = kube.CoreV1().Secrets(testNamespace).Update(ctx, secret, metav1.UpdateOptions{})
	require.NoError(t, err)
	o.ReconcileAll(ctx)
	_, status = stored(t, o)
	assert.Equal(t, "other", status.Tenant)
	daemonSet, err := kube.AppsV1().DaemonSets(testNamespace).Get(ctx, testObject, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Contains(t, daemonSet.Spec.Template.Spec.Containers[0].Env, corev1.EnvVar{Name: "TENANT", Value: "other"})
}

func TestOperatorRemovesTheDaemonSetBeforeTheTenantNetwork(t *testing.T) {
	ctx := context.Background()
	api := &fakeAPI{network: &panapi.TenantNetwork{TenantName: "acme"}}
	o, kube := newTestOperator(newTenantNetwork(map[string]interface{}{"realmSecretName": "realm"}), api)
	o.ReconcileAll(ctx)

	item, _ := stored(t, o)
	now := metav1.Now()
	item.SetDeletionTimestamp(&now)
	_, err := o.Dynamic.Resource(TenantNetworkGVR).Namespace(testNamespace).Update(ctx, item, metav1.UpdateOptions{})
	require.NoError(t, err)

	// First pass: the DaemonSet is deleted, the TenantNetwork waits.
	o.ReconcileAll(ctx)
	_, err = kube.AppsV1().DaemonSets(testNamespace).Get(ctx, testObject, metav1.GetOptions{})
	assert.True(t, apierrors.IsNotFound(err))
	item, status := stored(t, o)
	assert.Equal(t, []string{Finalizer}, item.GetFinalizers())
	assert.Equal(t, PhaseRemoving, status.Phase)

	// Second pass: the pods are gone, so the cluster permission and the finalizer are removed.
	o.ReconcileAll(ctx)
	_, err = kube.RbacV1().ClusterRoleBindings().Get(ctx, testNamespace+"-"+testObject, metav1.GetOptions{})
	assert.True(t, apierrors.IsNotFound(err))
	item, _ = stored(t, o)
	assert.Empty(t, item.GetFinalizers())
}

func TestOperatorServesATenantThroughOneTenantNetwork(t *testing.T) {
	ctx := context.Background()
	api := &fakeAPI{network: &panapi.TenantNetwork{TenantName: "acme"}}
	first := newTenantNetwork(map[string]interface{}{"realmSecretName": "realm"})
	first.SetCreationTimestamp(metav1.NewTime(time.Unix(100, 0)))
	o, kube := newTestOperator(first, api)
	add := func(namespace, name string, created int64) {
		item := newTenantNetwork(map[string]interface{}{"realmSecretName": "realm"})
		item.SetNamespace(namespace)
		item.SetName(name)
		item.SetCreationTimestamp(metav1.NewTime(time.Unix(created, 0)))
		_, err := o.Dynamic.Resource(TenantNetworkGVR).Namespace(namespace).Create(ctx, item, metav1.CreateOptions{})
		require.NoError(t, err)
	}
	// A second one in the same namespace, and one for the same tenant in another namespace.
	add(testNamespace, "acme-again", 200)
	add("other-ns", "acme-elsewhere", 300)
	_, err := kube.CoreV1().Secrets("other-ns").Create(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "realm", Namespace: "other-ns"},
		Data:       map[string][]byte{"api": []byte("realm.example"), "token": []byte("secret-token")},
	}, metav1.CreateOptions{})
	require.NoError(t, err)

	o.ReconcileAll(ctx)

	_, status := stored(t, o)
	assert.Equal(t, PhasePending, status.Phase)
	message := func(namespace, name string) string {
		item, err := o.Dynamic.Resource(TenantNetworkGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
		require.NoError(t, err)
		assert.Equal(t, PhaseError, statusOf(item).Phase)
		return statusOf(item).Message
	}
	assert.Contains(t, message(testNamespace, "acme-again"), `namespace "tenant-ns" already has TenantNetwork acme`)
	assert.Contains(t, message("other-ns", "acme-elsewhere"), "already served by TenantNetwork "+testNamespace+"/acme")
	_, err = kube.AppsV1().DaemonSets(testNamespace).Get(ctx, namePrefix+"acme-again", metav1.GetOptions{})
	assert.True(t, apierrors.IsNotFound(err))
	_, err = kube.AppsV1().DaemonSets("other-ns").Get(ctx, namePrefix+"acme-elsewhere", metav1.GetOptions{})
	assert.True(t, apierrors.IsNotFound(err))
}

func TestOperatorReportsTheNodesAndWritesEvents(t *testing.T) {
	ctx := context.Background()
	api := &fakeAPI{network: &panapi.TenantNetwork{TenantName: "acme"}}
	o, kube := newTestOperator(newTenantNetwork(map[string]interface{}{"realmSecretName": "realm"}), api)
	node := func(name string, labels, annotations map[string]string) {
		_, err := kube.CoreV1().Nodes().Create(ctx, &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels, Annotations: annotations},
		}, metav1.CreateOptions{})
		require.NoError(t, err)
	}
	node("node-b", map[string]string{TenantLabelKey: "acme"},
		map[string]string{OverlayAnnotation: "192.168.0.58", InterfaceAnnotation: "vxlan7", MessageAnnotation: "no tenant director answers"})
	// The value of the ready label is the namespace of the TenantNetwork.
	node("node-a", map[string]string{TenantLabelKey: "acme", ReadyLabelKey: testNamespace},
		map[string]string{OverlayAnnotation: "192.168.0.57", InterfaceAnnotation: "vxlan7"})
	node("node-c", map[string]string{TenantLabelKey: "acme"}, nil)

	o.ReconcileAll(ctx)
	o.ReconcileAll(ctx)

	_, status := stored(t, o)
	assert.Equal(t, map[string]NodeStatus{
		"node-a": {State: NodeStateReady, OverlayIP: "192.168.0.57"},
		"node-b": {State: NodeStateNotReady, OverlayIP: "192.168.0.58", Message: "no tenant director answers"},
		"node-c": {State: NodeStateNotReady, Message: "the pod of the node has not reported yet"},
	}, status.Nodes)
	assert.Equal(t, "vxlan7", status.Interface)

	// Two passes over an unchanged state: every event is written once.
	assert.Equal(t, []string{ReasonSecretFound, ReasonDaemonSetCreated, ReasonPending}, eventReasons(t, kube, testNamespace))

	// A failure is a warning with its own reason; the return to the old state is reported again.
	require.NoError(t, kube.CoreV1().Secrets(testNamespace).Delete(ctx, "realm", metav1.DeleteOptions{}))
	o.ReconcileAll(ctx)
	o.ReconcileAll(ctx)
	reasons := eventReasons(t, kube, testNamespace)
	assert.Equal(t, ReasonSecretNotFound, reasons[len(reasons)-1])
	assert.Len(t, reasons, 4)
}

func TestOperatorUsesTheNodeSelectorOfTheSpec(t *testing.T) {
	ctx := context.Background()
	api := &fakeAPI{network: &panapi.TenantNetwork{TenantName: "acme"}}
	selector := map[string]interface{}{"pool": "storage", "zone": "a"}
	o, kube := newTestOperator(newTenantNetwork(map[string]interface{}{"realmSecretName": "realm", "nodeSelector": selector}), api)
	for name, labels := range map[string]map[string]string{
		"node-in":         {"pool": "storage", "zone": "a"},
		"node-other-zone": {"pool": "storage", "zone": "b"},
		"node-by-tenant":  {TenantLabelKey: "acme"},
	} {
		_, err := kube.CoreV1().Nodes().Create(ctx, &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels}}, metav1.CreateOptions{})
		require.NoError(t, err)
	}

	o.ReconcileAll(ctx)

	daemonSet, err := kube.AppsV1().DaemonSets(testNamespace).Get(ctx, testObject, metav1.GetOptions{})
	require.NoError(t, err)
	pod := daemonSet.Spec.Template.Spec
	assert.Equal(t, map[string]string{"pool": "storage", "zone": "a"}, pod.NodeSelector)
	_, status := stored(t, o)
	require.Len(t, status.Nodes, 1)
	assert.Contains(t, status.Nodes, "node-in")
	assert.Equal(t, "no node to serve: the selector is for the nodes with the label pool=storage,zone=a", status.Message)

	// Without the selector, every node is a node of the tenant.
	item, _ := stored(t, o)
	unstructured.RemoveNestedField(item.Object, "spec", "nodeSelector")
	_, err = o.Dynamic.Resource(TenantNetworkGVR).Namespace(testNamespace).Update(ctx, item, metav1.UpdateOptions{})
	require.NoError(t, err)
	o.ReconcileAll(ctx)
	_, status = stored(t, o)
	assert.Len(t, status.Nodes, 3)
	daemonSet, err = kube.AppsV1().DaemonSets(testNamespace).Get(ctx, testObject, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Empty(t, daemonSet.Spec.Template.Spec.NodeSelector)

	// A selector that is no label is refused.
	item, _ = stored(t, o)
	require.NoError(t, unstructured.SetNestedField(item.Object, map[string]interface{}{"not a key": "x"}, "spec", "nodeSelector"))
	_, err = o.Dynamic.Resource(TenantNetworkGVR).Namespace(testNamespace).Update(ctx, item, metav1.UpdateOptions{})
	require.NoError(t, err)
	o.ReconcileAll(ctx)
	_, status = stored(t, o)
	assert.Equal(t, PhaseError, status.Phase)
	assert.Contains(t, status.Message, "spec.nodeSelector is not valid")
}

func TestOperatorReplacesABindingToAnotherNodeRole(t *testing.T) {
	ctx := context.Background()
	api := &fakeAPI{network: &panapi.TenantNetwork{TenantName: "acme"}}
	o, kube := newTestOperator(newTenantNetwork(map[string]interface{}{"realmSecretName": "realm"}), api)
	o.ReconcileAll(ctx)

	// The operator is installed again under another name: the ClusterRole of the node pods has a new name.
	o.Options.ClusterRole = "new-node-role"
	o.ReconcileAll(ctx)

	clusterBinding, err := kube.RbacV1().ClusterRoleBindings().Get(ctx, testNamespace+"-"+testObject, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "new-node-role", clusterBinding.RoleRef.Name)
	assert.Equal(t, testObject, clusterBinding.Subjects[0].Name)
}
