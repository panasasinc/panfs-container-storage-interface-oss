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

package main

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/panasasinc/panfs-container-storage-interface-oss/pkg/storageclass"
	"github.com/panasasinc/panfs-container-storage-interface-oss/pkg/tenantnet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

const (
	testOperatorNamespace = "csi-panfs"
	testOperatorName      = "panfs-tenant-operator"
)

func operatorDeployment(available corev1.ConditionStatus) *appsv1.Deployment {
	replicas := int32(1)
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: testOperatorName, Namespace: testOperatorNamespace},
		Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
		Status: appsv1.DeploymentStatus{AvailableReplicas: 1, Conditions: []appsv1.DeploymentCondition{
			{Type: appsv1.DeploymentAvailable, Status: available, Message: "minimum replicas"},
		}},
	}
}

func definition(name, established string) *unstructured.Unstructured {
	item := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apiextensions.k8s.io/v1",
		"kind":       "CustomResourceDefinition",
		"metadata":   map[string]any{"name": name},
		"status": map[string]any{"conditions": []any{
			map[string]any{"type": "Established", "status": established, "message": "names accepted"},
		}},
	}}
	return item
}

// resource returns a TenantNetwork or a PanfsStorageClass with a generation and the status of the operator.
func resource(gvr schema.GroupVersionResource, kind, namespace, name string, generation, observed int64,
	phase, message string) *unstructured.Unstructured {
	item := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": gvr.GroupVersion().String(),
		"kind":       kind,
		"metadata":   map[string]any{"name": name, "namespace": namespace, "generation": generation},
	}}
	if observed > 0 {
		item.Object["status"] = map[string]any{"observedGeneration": observed, "phase": phase, "message": message}
	}
	return item
}

func newHealthCheck(deployment *appsv1.Deployment, items ...runtime.Object) (*HealthCheck, *bytes.Buffer) {
	kinds := map[schema.GroupVersionResource]string{
		tenantnet.TenantNetworkGVR:        tenantnet.TenantNetworkKind + "List",
		storageclass.PanfsStorageClassGVR: storageclass.PanfsStorageClassKind + "List",
		crdGVR:                            "CustomResourceDefinitionList",
	}
	var kube *kubefake.Clientset
	if deployment != nil {
		kube = kubefake.NewClientset(deployment)
	} else {
		kube = kubefake.NewClientset()
	}
	out := &bytes.Buffer{}
	return &HealthCheck{
		Kube:       kube,
		Dynamic:    dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), kinds, items...),
		Namespace:  testOperatorNamespace,
		Deployment: testOperatorName,
		Timeout:    0,
		Interval:   time.Millisecond,
		Out:        out,
	}, out
}

func definitions() []runtime.Object {
	return []runtime.Object{
		definition("tenantnetworks.panfs.vdura.com", "True"),
		definition("panfsstorageclasses.panfs.vdura.com", "True"),
	}
}

func TestHealthCheckPassesAndListsResourcesThatAreNotReady(t *testing.T) {
	items := append(definitions(),
		resource(tenantnet.TenantNetworkGVR, tenantnet.TenantNetworkKind, "acme", "acme", 1, 1, "Ready", ""),
		resource(storageclass.PanfsStorageClassGVR, storageclass.PanfsStorageClassKind, "default", "sc-2", 1, 1,
			"Error", `namespace "default" has no TenantNetwork`),
	)
	health, out := newHealthCheck(operatorDeployment(corev1.ConditionTrue), items...)

	require.NoError(t, health.Run(context.Background()))
	assert.Contains(t, out.String(), "OK: the operator is available")
	assert.Contains(t, out.String(), "processed all 2 resource(s)")
	assert.Contains(t, out.String(), `PanfsStorageClass default/sc-2: phase Error: namespace "default" has no TenantNetwork`)
	assert.NotContains(t, out.String(), "TenantNetwork acme/acme")
}

func TestHealthCheckFailsWhenTheOperatorHasNotProcessedAResource(t *testing.T) {
	items := append(definitions(),
		resource(tenantnet.TenantNetworkGVR, tenantnet.TenantNetworkKind, "acme", "acme", 3, 2, "Ready", ""),
		resource(storageclass.PanfsStorageClassGVR, storageclass.PanfsStorageClassKind, "acme", "new", 1, 0, "", ""),
	)
	health, out := newHealthCheck(operatorDeployment(corev1.ConditionTrue), items...)

	require.Error(t, health.Run(context.Background()))
	assert.Contains(t, out.String(), "FAILED:")
	assert.Contains(t, out.String(), "TenantNetwork acme/acme: the operator has not processed generation 3 (status shows 2)")
	assert.Contains(t, out.String(), "PanfsStorageClass acme/new: the operator has not processed generation 1 (status shows 0)")
}

func TestHealthCheckFailsWhenTheDeploymentIsNotAvailable(t *testing.T) {
	health, out := newHealthCheck(operatorDeployment(corev1.ConditionFalse), definitions()...)

	require.Error(t, health.Run(context.Background()))
	assert.Contains(t, out.String(), "the Deployment csi-panfs/panfs-tenant-operator is not available: minimum replicas")
}

func TestHealthCheckFailsWhenTheDeploymentIsScaledToZero(t *testing.T) {
	// Kubernetes reports a Deployment with zero replicas as Available; the operator does not run then.
	deployment := operatorDeployment(corev1.ConditionTrue)
	zero := int32(0)
	deployment.Spec.Replicas = &zero
	deployment.Status.AvailableReplicas = 0
	health, out := newHealthCheck(deployment, definitions()...)

	require.Error(t, health.Run(context.Background()))
	assert.Contains(t, out.String(), "the Deployment csi-panfs/panfs-tenant-operator has no available replica (0 of 0 desired)")
}

func TestHealthCheckFailsWhenTheDeploymentIsMissing(t *testing.T) {
	health, out := newHealthCheck(nil, definitions()...)

	require.Error(t, health.Run(context.Background()))
	assert.Contains(t, out.String(), "cannot read the Deployment csi-panfs/panfs-tenant-operator")
}

func TestHealthCheckFailsWhenADefinitionIsMissingOrNotEstablished(t *testing.T) {
	health, out := newHealthCheck(operatorDeployment(corev1.ConditionTrue),
		definition("tenantnetworks.panfs.vdura.com", "False"))

	require.Error(t, health.Run(context.Background()))
	assert.Contains(t, out.String(), "the resource definition tenantnetworks.panfs.vdura.com is not established: names accepted")
	assert.Contains(t, out.String(), "cannot read the resource definition panfsstorageclasses.panfs.vdura.com")
}

func TestHealthCheckWaitsUntilTheOperatorHasProcessedTheResource(t *testing.T) {
	stale := resource(tenantnet.TenantNetworkGVR, tenantnet.TenantNetworkKind, "acme", "acme", 2, 1, "Ready", "")
	health, out := newHealthCheck(operatorDeployment(corev1.ConditionTrue), append(definitions(), stale)...)
	health.Timeout = 5 * time.Second

	// The operator catches up a moment after the check starts.
	go func() {
		time.Sleep(20 * time.Millisecond)
		current := stale.DeepCopy()
		require.NoError(t, unstructured.SetNestedField(current.Object, int64(2), "status", "observedGeneration"))
		_, _ = health.Dynamic.Resource(tenantnet.TenantNetworkGVR).Namespace("acme").
			Update(context.Background(), current, metav1.UpdateOptions{})
	}()

	require.NoError(t, health.Run(context.Background()))
	assert.Contains(t, out.String(), "OK: the operator is available")
}
