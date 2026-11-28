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
	"context"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/panasasinc/panfs-container-storage-interface-oss/pkg/storageclass"
	"github.com/panasasinc/panfs-container-storage-interface-oss/pkg/tenantnet"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

// crdGVR is the resource of the CustomResourceDefinitions. The dynamic client reads them, so the check needs no
// client of its own for the apiextensions API group.
var crdGVR = schema.GroupVersionResource{Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions"}

// served describes one resource kind of the operator for the health check.
type served struct {
	kind string
	crd  string
	gvr  schema.GroupVersionResource
}

var servedKinds = []served{
	{kind: tenantnet.TenantNetworkKind, crd: "tenantnetworks.panfs.vdura.com", gvr: tenantnet.TenantNetworkGVR},
	{kind: storageclass.PanfsStorageClassKind, crd: "panfsstorageclasses.panfs.vdura.com", gvr: storageclass.PanfsStorageClassGVR},
}

// HealthCheck answers whether the operator works, for "helm test" of the operator chart. It fails when the
// operator Deployment is not available, when a resource definition is missing or not established, or when the
// operator has not processed a resource. A resource that the operator processed and found wrong (phase Pending or
// Error) does not fail the check: it is listed, because its cause is in the resource, not in the operator.
type HealthCheck struct {
	Kube       kubernetes.Interface
	Dynamic    dynamic.Interface
	Namespace  string
	Deployment string
	Timeout    time.Duration
	Interval   time.Duration
	Out        io.Writer
}

// checkResult is the outcome of one pass.
type checkResult struct {
	problems  []string // what makes the operator unhealthy
	notReady  []string // resources the operator processed and did not find ready
	resources int      // resources seen
}

// Run repeats the check until it passes or the timeout ends, so that a resource that was created a moment ago gets
// the time of one pass of the operator. It prints the report of the last pass and returns an error when it failed.
func (h *HealthCheck) Run(ctx context.Context) error {
	deadline := time.Now().Add(h.Timeout)
	var result checkResult
	for {
		result = h.pass(ctx)
		if len(result.problems) == 0 || !time.Now().Before(deadline) {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(h.Interval):
		}
	}
	h.report(result)
	if len(result.problems) > 0 {
		return fmt.Errorf("the PanFS Tenant operator is not healthy: %d problem(s)", len(result.problems))
	}
	return nil
}

func (h *HealthCheck) pass(ctx context.Context) checkResult {
	var result checkResult
	if problem := h.checkDeployment(ctx); problem != "" {
		result.problems = append(result.problems, problem)
	}
	for _, kind := range servedKinds {
		if problem := h.checkDefinition(ctx, kind); problem != "" {
			result.problems = append(result.problems, problem)
			continue
		}
		list, err := h.Dynamic.Resource(kind.gvr).Namespace(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
		if err != nil {
			result.problems = append(result.problems, fmt.Sprintf("cannot list the %s resources: %v", kind.kind, err))
			continue
		}
		for i := range list.Items {
			result.resources++
			item := &list.Items[i]
			name := fmt.Sprintf("%s %s/%s", kind.kind, item.GetNamespace(), item.GetName())
			observed, _, _ := unstructured.NestedInt64(item.Object, "status", "observedGeneration")
			phase, _, _ := unstructured.NestedString(item.Object, "status", "phase")
			message, _, _ := unstructured.NestedString(item.Object, "status", "message")
			switch {
			case observed != item.GetGeneration():
				result.problems = append(result.problems, fmt.Sprintf(
					"%s: the operator has not processed generation %d (status shows %d)", name, item.GetGeneration(), observed))
			case phase != tenantnet.PhaseReady:
				result.notReady = append(result.notReady, fmt.Sprintf("%s: phase %s: %s", name, phase, message))
			}
		}
	}
	sort.Strings(result.problems)
	sort.Strings(result.notReady)
	return result
}

func (h *HealthCheck) checkDeployment(ctx context.Context) string {
	deployment, err := h.Kube.AppsV1().Deployments(h.Namespace).Get(ctx, h.Deployment, metav1.GetOptions{})
	if err != nil {
		return fmt.Sprintf("cannot read the Deployment %s/%s: %v", h.Namespace, h.Deployment, err)
	}
	for _, condition := range deployment.Status.Conditions {
		if condition.Type == appsv1.DeploymentAvailable {
			if condition.Status == corev1.ConditionTrue {
				return ""
			}
			return fmt.Sprintf("the Deployment %s/%s is not available: %s", h.Namespace, h.Deployment, condition.Message)
		}
	}
	return fmt.Sprintf("the Deployment %s/%s reports no availability yet", h.Namespace, h.Deployment)
}

func (h *HealthCheck) checkDefinition(ctx context.Context, kind served) string {
	crd, err := h.Dynamic.Resource(crdGVR).Get(ctx, kind.crd, metav1.GetOptions{})
	if err != nil {
		return fmt.Sprintf("cannot read the resource definition %s: %v", kind.crd, err)
	}
	conditions, _, _ := unstructured.NestedSlice(crd.Object, "status", "conditions")
	for _, value := range conditions {
		condition, _ := value.(map[string]any)
		if condition["type"] == "Established" {
			if condition["status"] == "True" {
				return ""
			}
			return fmt.Sprintf("the resource definition %s is not established: %v", kind.crd, condition["message"])
		}
	}
	return fmt.Sprintf("the resource definition %s is not established yet", kind.crd)
}

func (h *HealthCheck) report(result checkResult) {
	h.printf("PanFS Tenant operator health check (Deployment %s/%s)\n", h.Namespace, h.Deployment)
	if len(result.problems) == 0 {
		h.printf("OK: the operator is available, both resource definitions are established, "+
			"and it has processed all %d resource(s).\n", result.resources)
	} else {
		h.printf("FAILED:\n")
		for _, problem := range result.problems {
			h.printf("  - %s\n", problem)
		}
	}
	if len(result.notReady) > 0 {
		h.printf("Resources that are not ready (their status says why; this does not fail the check):\n")
		for _, line := range result.notReady {
			h.printf("  - %s\n", line)
		}
	}
}

// printf writes one part of the report. The report goes to the log of the test pod, and a failed write there
// leaves nothing to report to, so the error is not checked.
func (h *HealthCheck) printf(format string, args ...any) {
	_, _ = fmt.Fprintf(h.Out, format, args...)
}
