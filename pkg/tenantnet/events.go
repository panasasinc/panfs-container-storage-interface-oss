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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

// EventTypeError is the type of the events that report a failure.
//
// Kubernetes itself knows two event types, Normal and Warning, and reports its own failures as Warning.
// This type is used on purpose instead: a TenantNetwork that cannot be served, or a node whose tenant network
// does not work, should read as an error in "kubectl describe". The price is known: the API accepts the type,
// but "kubectl get events --types" does not, and tools that look for Warning events do not see these.
const EventTypeError = "Error"

// Reasons of the events on a TenantNetwork.
const (
	ReasonSecretFound      = "SecretFound"
	ReasonDaemonSetCreated = "DaemonSetCreated"
	ReasonDaemonSetUpdated = "DaemonSetUpdated"
	ReasonReady            = "Ready"
	ReasonPending          = "Pending"
	ReasonRemoving         = "Removing"

	ReasonInvalidSpec         = "InvalidSpec"
	ReasonSecretNotFound      = "SecretNotFound"
	ReasonPanAPIError         = "PanAPIError"
	ReasonTenantAlreadyServed = "TenantAlreadyServed"
	ReasonNodeSelectorOverlap = "NodeSelectorOverlap"
	ReasonReconcileFailed     = "ReconcileFailed"

	ReasonNodeAttached        = "NodeAttached"
	ReasonInterfaceConfigured = "InterfaceConfigured"
	ReasonFirewallZoneSet     = "FirewallZoneSet"
	ReasonNodeReady           = "NodeReady"
	ReasonNodeNotReady        = "NodeNotReady"
	ReasonNodeStopped         = "NodeStopped"
	ReasonNodeDetached        = "NodeDetached"
)

// reasonedError is an error with the reason of the event that reports it.
type reasonedError struct {
	reason string
	err    error
}

func (e *reasonedError) Error() string { return e.err.Error() }
func (e *reasonedError) Unwrap() error { return e.err }

// withReason attaches an event reason to an error.
func withReason(reason string, err error) error {
	return &reasonedError{reason: reason, err: err}
}

// reasonOf returns the event reason of an error.
func reasonOf(err error) string {
	var reasoned *reasonedError
	if errors.As(err, &reasoned) {
		return reasoned.reason
	}
	return ReasonReconcileFailed
}

// EventSink writes Kubernetes events about one object. The callers write an event only when something changed:
// a pass that finds everything in place is silent.
type EventSink struct {
	Kube kubernetes.Interface
	// Ref is the object the events are about.
	Ref corev1.ObjectReference
	// Component and Host name the writer; kubectl shows them in the column "From".
	Component string
	Host      string
	Log       klog.Logger
}

// tenantNetworkRef returns the reference to a TenantNetwork.
func tenantNetworkRef(namespace, name string, uid types.UID) corev1.ObjectReference {
	return corev1.ObjectReference{
		APIVersion: TenantNetworkGVR.GroupVersion().String(), Kind: TenantNetworkKind,
		Namespace: namespace, Name: name, UID: uid,
	}
}

// NewTenantNetworkEvents returns a sink for events about a TenantNetwork.
func NewTenantNetworkEvents(kube kubernetes.Interface, namespace, name, uid, component, host string, log klog.Logger) *EventSink {
	return &EventSink{Kube: kube, Ref: tenantNetworkRef(namespace, name, types.UID(uid)), Component: component, Host: host, Log: log}
}

// Event writes one event. A failure is logged and otherwise ignored: an event is a report, not a step of the work.
func (e *EventSink) Event(ctx context.Context, eventType, reason, message string) {
	if e == nil || e.Kube == nil {
		return
	}
	now := metav1.Now()
	event := &corev1.Event{
		ObjectMeta:     metav1.ObjectMeta{GenerateName: e.Ref.Name + ".", Namespace: e.Ref.Namespace},
		InvolvedObject: e.Ref,
		Type:           eventType,
		Reason:         reason,
		Message:        message,
		Source:         corev1.EventSource{Component: e.Component, Host: e.Host},
		FirstTimestamp: now,
		LastTimestamp:  now,
		Count:          1,
	}
	if _, err := e.Kube.CoreV1().Events(e.Ref.Namespace).Create(ctx, event, metav1.CreateOptions{}); err != nil {
		e.Log.V(2).Info("cannot write an event", "reason", reason, "error", err.Error())
	}
}
