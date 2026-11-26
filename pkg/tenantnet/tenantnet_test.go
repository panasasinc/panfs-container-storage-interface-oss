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
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/panasasinc/panfs-container-storage-interface-oss/pkg/panapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/klog/v2"
)

// fakeRunner answers the read commands from fixed output and records every other command.
type fakeRunner struct {
	// outputs maps a command line prefix to its output.
	outputs map[string]string
	// failures maps a command line prefix to an error.
	failures map[string]error
	// changes are the recorded commands that are not reads.
	changes []string
}

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	line := name + " " + strings.Join(args, " ")
	for prefix, err := range f.failures {
		if strings.HasPrefix(line, prefix) {
			return nil, err
		}
	}
	for prefix, out := range f.outputs {
		if strings.HasPrefix(line, prefix) {
			return []byte(out), nil
		}
	}
	// ping only reads; it is not a change.
	if name != "ping" {
		f.changes = append(f.changes, line)
	}
	return nil, nil
}

const (
	readLinks = "ip -json -details addr show"
	readFDB   = "bridge -json fdb show"
)

func testAttachment() *panapi.Attachment {
	return &panapi.Attachment{
		ID: "a1", SubnetID: "s1", UnderlayIP: "10.0.0.5", Hostname: "node-1",
		OverlayIP: "192.168.0.58", OverlayPrefix: 16, MTU: 1450,
		SegmentType: panapi.SegmentTypeVXLAN, SegmentID: 5001,
		Tunnel: &panapi.AttachmentTunnel{MAC: "02:00:5E:10:00:A4", UDPPort: 4789, VTEPs: []string{"10.0.1.1", "10.0.1.2"}},
	}
}

// matchingLink is the `ip` output of an interface that already matches testAttachment.
const matchingLink = `[{"ifname":"lo","flags":["UP"],"mtu":65536},
{"ifname":"vxlan5001","flags":["BROADCAST","UP","LOWER_UP"],"mtu":1450,"address":"02:00:5e:10:00:a4",
 "linkinfo":{"info_kind":"vxlan","info_data":{"id":5001,"local":"10.0.0.5","port":4789}},
 "addr_info":[{"family":"inet","local":"192.168.0.58","prefixlen":16},{"family":"inet6","local":"fe80::1","prefixlen":64}]}]`

const matchingFDB = `[{"mac":"00:00:00:00:00:00","dst":"10.0.1.1","flags":["self"],"state":"permanent"},
{"mac":"00:00:00:00:00:00","dst":"10.0.1.2","flags":["self"],"state":"permanent"},
{"mac":"aa:bb:cc:dd:ee:ff","dst":"10.0.1.9","flags":["self"]}]`

func TestEnsureInterface(t *testing.T) {
	ctx := context.Background()

	t.Run("a matching interface is not touched", func(t *testing.T) {
		run := &fakeRunner{outputs: map[string]string{readLinks: matchingLink, readFDB: matchingFDB}}
		name, changed, err := EnsureInterface(ctx, run, testAttachment())
		require.NoError(t, err)
		assert.Equal(t, "vxlan5001", name)
		assert.False(t, changed)
		assert.Empty(t, run.changes)
	})

	t.Run("a missing interface is created", func(t *testing.T) {
		run := &fakeRunner{outputs: map[string]string{readLinks: `[{"ifname":"lo"}]`, readFDB: `[]`}}
		_, changed, err := EnsureInterface(ctx, run, testAttachment())
		require.NoError(t, err)
		assert.True(t, changed)
		assert.Equal(t, []string{
			"ip link add vxlan5001 type vxlan id 5001 dstport 4789 local 10.0.0.5",
			"ip link set vxlan5001 address 02:00:5e:10:00:a4",
			"ip link set vxlan5001 mtu 1450",
			"ip link set vxlan5001 up",
			"ip addr add 192.168.0.58/16 dev vxlan5001",
			"bridge fdb append 00:00:00:00:00:00 dev vxlan5001 dst 10.0.1.1",
			"bridge fdb append 00:00:00:00:00:00 dev vxlan5001 dst 10.0.1.2",
		}, run.changes)
	})

	t.Run("mutable settings are corrected in place", func(t *testing.T) {
		link := strings.NewReplacer(`"mtu":1450`, `"mtu":1500`, `192.168.0.58`, `192.168.0.99`).Replace(matchingLink)
		fdb := `[{"mac":"00:00:00:00:00:00","dst":"10.0.1.1"},{"mac":"00:00:00:00:00:00","dst":"10.0.1.7"}]`
		run := &fakeRunner{outputs: map[string]string{readLinks: link, readFDB: fdb}}
		_, changed, err := EnsureInterface(ctx, run, testAttachment())
		require.NoError(t, err)
		assert.True(t, changed)
		assert.Equal(t, []string{
			"ip link set vxlan5001 mtu 1450",
			"ip addr del 192.168.0.99/16 dev vxlan5001",
			"ip addr add 192.168.0.58/16 dev vxlan5001",
			"bridge fdb del 00:00:00:00:00:00 dev vxlan5001 dst 10.0.1.7",
			"bridge fdb append 00:00:00:00:00:00 dev vxlan5001 dst 10.0.1.2",
		}, run.changes)
	})

	t.Run("a changed address takes the interface down first", func(t *testing.T) {
		link := strings.Replace(matchingLink, "02:00:5e:10:00:a4", "02:00:00:00:00:01", 1)
		run := &fakeRunner{outputs: map[string]string{readLinks: link, readFDB: matchingFDB}}
		_, _, err := EnsureInterface(ctx, run, testAttachment())
		require.NoError(t, err)
		assert.Equal(t, []string{
			"ip link set vxlan5001 down",
			"ip link set vxlan5001 address 02:00:5e:10:00:a4",
			"ip link set vxlan5001 up",
		}, run.changes)
	})

	t.Run("a changed local address means a new interface", func(t *testing.T) {
		link := strings.Replace(matchingLink, `"local":"10.0.0.5"`, `"local":"10.0.0.6"`, 1)
		run := &fakeRunner{outputs: map[string]string{readLinks: link, readFDB: `[]`}}
		_, changed, err := EnsureInterface(ctx, run, testAttachment())
		require.NoError(t, err)
		assert.True(t, changed)
		assert.Equal(t, "ip link del vxlan5001", run.changes[0])
		assert.Contains(t, run.changes[1], "ip link add vxlan5001 type vxlan")
	})

	t.Run("a bad record runs no command", func(t *testing.T) {
		for name, edit := range map[string]func(*panapi.Attachment){
			"segment type": func(a *panapi.Attachment) { a.SegmentType = "none" },
			"no tunnel":    func(a *panapi.Attachment) { a.Tunnel = nil },
			"mac":          func(a *panapi.Attachment) { a.Tunnel.MAC = "x; reboot" },
			"vtep":         func(a *panapi.Attachment) { a.Tunnel.VTEPs = []string{"10.0.1.1 dev eth0"} },
			"no vtep":      func(a *panapi.Attachment) { a.Tunnel.VTEPs = nil },
			"overlay":      func(a *panapi.Attachment) { a.OverlayIP = "not-an-ip" },
			"prefix":       func(a *panapi.Attachment) { a.OverlayPrefix = 64 },
			"segment id":   func(a *panapi.Attachment) { a.SegmentID = 1 << 24 },
		} {
			a := testAttachment()
			edit(a)
			run := &fakeRunner{}
			_, _, err := EnsureInterface(ctx, run, a)
			assert.Error(t, err, name)
			assert.Empty(t, run.changes, name)
		}
	})
}

func TestPing(t *testing.T) {
	ctx := context.Background()

	run := &fakeRunner{failures: map[string]error{"ping -c 3 -W 2 -I vxlan1 192.168.0.2": errors.New("timeout")}}
	answered, err := Ping(ctx, run, "vxlan1", []string{"192.168.0.2", "192.168.0.3"})
	require.NoError(t, err)
	assert.Equal(t, "192.168.0.3", answered)

	_, err = Ping(ctx, &fakeRunner{failures: map[string]error{"ping": errors.New("timeout")}}, "vxlan1", []string{"192.168.0.2"})
	assert.Error(t, err)

	_, err = Ping(ctx, &fakeRunner{}, "vxlan1", nil)
	assert.Error(t, err)
}

func TestFirewalldEnsureZone(t *testing.T) {
	ctx := context.Background()
	socket := filepath.Join(t.TempDir(), "system_bus_socket")
	require.NoError(t, os.WriteFile(socket, nil, 0o600))
	const hasOwner = "dbus-send --bus=unix:path=" + "SOCKET" + " --print-reply=literal --reply-timeout=10000 --dest=org.freedesktop.DBus"
	owner := strings.Replace(hasOwner, "SOCKET", socket, 1)
	getZone := "dbus-send --bus=unix:path=" + socket +
		" --print-reply=literal --reply-timeout=10000 --dest=org.fedoraproject.FirewallD1 /org/fedoraproject/FirewallD1 org.fedoraproject.FirewallD1.zone.getZoneOfInterface"

	t.Run("no zone configured", func(t *testing.T) {
		run := &fakeRunner{}
		changed, err := (&Firewalld{Run: run, Socket: socket}).EnsureZone(ctx, "vxlan1")
		require.NoError(t, err)
		assert.False(t, changed)
		assert.Empty(t, run.changes)
	})

	t.Run("no D-Bus socket on the host", func(t *testing.T) {
		run := &fakeRunner{}
		changed, err := (&Firewalld{Run: run, Socket: socket + ".missing", Zone: "trusted"}).EnsureZone(ctx, "vxlan1")
		require.NoError(t, err)
		assert.False(t, changed)
		assert.Empty(t, run.changes)
	})

	t.Run("firewalld does not run", func(t *testing.T) {
		run := &fakeRunner{outputs: map[string]string{owner: "   boolean false\n"}}
		changed, err := (&Firewalld{Run: run, Socket: socket, Zone: "trusted"}).EnsureZone(ctx, "vxlan1")
		require.NoError(t, err)
		assert.False(t, changed)
		assert.Empty(t, run.changes)
	})

	t.Run("the zone is already right", func(t *testing.T) {
		run := &fakeRunner{outputs: map[string]string{owner: "   boolean true\n", getZone: "   trusted\n"}}
		changed, err := (&Firewalld{Run: run, Socket: socket, Zone: "trusted"}).EnsureZone(ctx, "vxlan1")
		require.NoError(t, err)
		assert.False(t, changed)
		assert.Empty(t, run.changes)
	})

	t.Run("the interface is moved", func(t *testing.T) {
		run := &fakeRunner{outputs: map[string]string{owner: "   boolean true\n", getZone: "\n"}}
		changed, err := (&Firewalld{Run: run, Socket: socket, Zone: "trusted"}).EnsureZone(ctx, "vxlan1")
		require.NoError(t, err)
		assert.True(t, changed)
		require.Len(t, run.changes, 1)
		assert.True(t, strings.HasSuffix(run.changes[0], "zone.changeZoneOfInterface string:trusted string:vxlan1"))
	})

	t.Run("a bad zone name is refused", func(t *testing.T) {
		_, err := (&Firewalld{Run: &fakeRunner{}, Socket: socket, Zone: "trusted string:x"}).EnsureZone(ctx, "vxlan1")
		assert.Error(t, err)
	})
}

// fakeAPI is a PanAPI with one tenant network.
type fakeAPI struct {
	network *panapi.TenantNetwork
	err     error
	// detached holds the ids of the attachments that were removed.
	detached []string
}

func (f *fakeAPI) FindAttachment(context.Context, string, string, map[string]string) (*panapi.Attachment, error) {
	if len(f.detached) > 0 {
		return nil, nil
	}
	return testAttachment(), nil
}

func (f *fakeAPI) DetachHost(_ context.Context, _, attachmentID string, _ map[string]string) error {
	f.detached = append(f.detached, attachmentID)
	return nil
}

func (f *fakeAPI) GetTenantNetwork(context.Context, map[string]string) (*panapi.TenantNetwork, error) {
	return f.network, f.err
}

func (f *fakeAPI) AttachHost(context.Context, string, string, string, map[string]string) (*panapi.Attachment, error) {
	return testAttachment(), nil
}

func newTestReconciler(t *testing.T, api API, run Runner) (*Reconciler, *fake.Clientset) {
	t.Helper()
	dir := t.TempDir()
	secretDir := filepath.Join(dir, "secret")
	require.NoError(t, os.Mkdir(secretDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(secretDir, "api"), []byte("https://realm.example\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(secretDir, "token"), []byte("t0ken\n"), 0o600))

	kube := fake.NewSimpleClientset(
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-1", Labels: map[string]string{TenantLabelKey: "acme"}}},
		&appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{Name: "panfs-tenant-network", Namespace: "tenant-ns"},
			Spec: appsv1.DaemonSetSpec{Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{NodeSelector: map[string]string{TenantLabelKey: "acme"}},
			}},
		},
	)
	return &Reconciler{
		Config: Config{
			Tenant: "acme", NodeName: "node-1", HostIP: "10.0.0.5", SecretDir: secretDir,
			ReadyLabelKey: ReadyLabelKey, ReadyFile: filepath.Join(dir, "ready"), Interval: time.Hour,
			Namespace: "tenant-ns", DaemonSet: "panfs-tenant-network", DetachOnRemoval: true,
		},
		API: api, Run: run, Kube: kube, Log: klog.NewKlogr(),
	}, kube
}

func nodeLabels(t *testing.T, kube *fake.Clientset) map[string]string {
	t.Helper()
	node, err := kube.CoreV1().Nodes().Get(context.Background(), "node-1", metav1.GetOptions{})
	require.NoError(t, err)
	return node.Labels
}

func TestReconcilerReportsState(t *testing.T) {
	ctx := context.Background()
	api := &fakeAPI{network: &panapi.TenantNetwork{TenantName: "acme", SubnetID: "s1", Directors: []string{"192.168.0.2"}}}
	run := &fakeRunner{outputs: map[string]string{readLinks: matchingLink, readFDB: matchingFDB}}
	r, kube := newTestReconciler(t, api, run)

	// The network works: the label and the ready file appear.
	r.step(ctx)
	assert.Equal(t, "acme", nodeLabels(t, kube)[ReadyLabelKey])
	assert.FileExists(t, r.Config.ReadyFile)

	// The director stops answering: both go away, and the intent label stays.
	run.failures = map[string]error{"ping": errors.New("timeout")}
	r.step(ctx)
	labels := nodeLabels(t, kube)
	assert.NotContains(t, labels, ReadyLabelKey)
	assert.Equal(t, "acme", labels[TenantLabelKey])
	assert.NoFileExists(t, r.Config.ReadyFile)

	// It answers again: the node is ready again.
	run.failures = nil
	r.step(ctx)
	assert.Equal(t, "acme", nodeLabels(t, kube)[ReadyLabelKey])
}

func TestReconcilerRefusesTokenOfAnotherTenant(t *testing.T) {
	api := &fakeAPI{network: &panapi.TenantNetwork{TenantName: "otto", SubnetID: "s1", Directors: []string{"192.168.0.2"}}}
	run := &fakeRunner{outputs: map[string]string{readLinks: matchingLink, readFDB: matchingFDB}}
	r, kube := newTestReconciler(t, api, run)

	r.step(context.Background())
	assert.NotContains(t, nodeLabels(t, kube), ReadyLabelKey)
	assert.Empty(t, run.changes)
}

func TestLoopRemovesLabelOnShutdown(t *testing.T) {
	api := &fakeAPI{network: &panapi.TenantNetwork{TenantName: "acme", SubnetID: "s1", Directors: []string{"192.168.0.2"}}}
	run := &fakeRunner{outputs: map[string]string{readLinks: matchingLink, readFDB: matchingFDB}}
	r, kube := newTestReconciler(t, api, run)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { r.Loop(ctx); close(done) }()

	require.Eventually(t, func() bool { return nodeLabels(t, kube)[ReadyLabelKey] == "acme" }, 5*time.Second, 10*time.Millisecond)
	cancel()
	<-done
	assert.NotContains(t, nodeLabels(t, kube), ReadyLabelKey)
	assert.NoFileExists(t, r.Config.ReadyFile)

	// A plain restart: the interface and the attachment stay.
	assert.Empty(t, api.detached)
	assert.Empty(t, run.changes)
}

func TestShutdownDetachesNodeThatLeftTheTenant(t *testing.T) {
	ctx := context.Background()
	newParts := func(t *testing.T) (*Reconciler, *fake.Clientset, *fakeAPI, *fakeRunner) {
		api := &fakeAPI{network: &panapi.TenantNetwork{TenantName: "acme", SubnetID: "s1", Directors: []string{"192.168.0.2"}}}
		run := &fakeRunner{outputs: map[string]string{readLinks: matchingLink, readFDB: matchingFDB}}
		r, kube := newTestReconciler(t, api, run)
		r.step(ctx)
		return r, kube, api, run
	}

	t.Run("the tenant label was removed", func(t *testing.T) {
		r, kube, api, run := newParts(t)
		_, err := kube.CoreV1().Nodes().Patch(ctx, "node-1", types.MergePatchType,
			[]byte(`{"metadata":{"labels":{"`+TenantLabelKey+`":null}}}`), metav1.PatchOptions{})
		require.NoError(t, err)

		r.shutdown()
		assert.Equal(t, []string{"a1"}, api.detached)
		assert.Equal(t, []string{"ip link del vxlan5001"}, run.changes)
		assert.NotContains(t, nodeLabels(t, kube), ReadyLabelKey)
	})

	t.Run("the DaemonSet was deleted", func(t *testing.T) {
		r, kube, api, run := newParts(t)
		require.NoError(t, kube.AppsV1().DaemonSets("tenant-ns").Delete(ctx, "panfs-tenant-network", metav1.DeleteOptions{}))

		r.shutdown()
		assert.Equal(t, []string{"a1"}, api.detached)
		assert.Equal(t, []string{"ip link del vxlan5001"}, run.changes)
	})

	t.Run("detach on removal is off", func(t *testing.T) {
		r, kube, api, run := newParts(t)
		r.Config.DetachOnRemoval = false
		require.NoError(t, kube.AppsV1().DaemonSets("tenant-ns").Delete(ctx, "panfs-tenant-network", metav1.DeleteOptions{}))

		r.shutdown()
		assert.Empty(t, api.detached)
		assert.Empty(t, run.changes)
		assert.NotContains(t, nodeLabels(t, kube), ReadyLabelKey)
	})
}

// eventTypes returns the type of every event in a namespace, by reason.
func eventTypes(t *testing.T, kube *fake.Clientset, namespace string) map[string]string {
	t.Helper()
	types := make(map[string]string)
	for _, action := range kube.Actions() {
		if create, ok := action.(k8stesting.CreateAction); ok && action.Matches("create", "events") && action.GetNamespace() == namespace {
			event := create.GetObject().(*corev1.Event)
			types[event.Reason] = event.Type
		}
	}
	return types
}

// eventReasons returns the reasons of the events in a namespace, in the order they were written.
func eventReasons(t *testing.T, kube *fake.Clientset, namespace string) []string {
	t.Helper()
	var reasons []string
	for _, action := range kube.Actions() {
		if create, ok := action.(k8stesting.CreateAction); ok && action.Matches("create", "events") && action.GetNamespace() == namespace {
			reasons = append(reasons, create.GetObject().(*corev1.Event).Reason)
		}
	}
	return reasons
}

func TestReconcilerReportsEventsAndNodeAnnotations(t *testing.T) {
	ctx := context.Background()
	api := &fakeAPI{network: &panapi.TenantNetwork{TenantName: "acme", SubnetID: "s1", Directors: []string{"192.168.0.2"}}}
	run := &fakeRunner{outputs: map[string]string{readLinks: matchingLink, readFDB: matchingFDB}}
	r, kube := newTestReconciler(t, api, run)
	r.Events = NewTenantNetworkEvents(kube, "tenant-ns", "acme", "uid-1", "panfs-tenant-network", "node-1", r.Log)
	annotations := func() map[string]string {
		node, err := kube.CoreV1().Nodes().Get(ctx, "node-1", metav1.GetOptions{})
		require.NoError(t, err)
		return node.Annotations
	}

	// Two passes that find everything in place: each change is reported once.
	r.step(ctx)
	r.step(ctx)
	assert.Equal(t, []string{ReasonNodeAttached, ReasonNodeReady}, eventReasons(t, kube, "tenant-ns"))
	assert.Equal(t, testAttachment().OverlayIP, annotations()[OverlayAnnotation])
	assert.Equal(t, InterfaceName(testAttachment().SegmentID), annotations()[InterfaceAnnotation])
	assert.NotContains(t, annotations(), MessageAnnotation)

	events, err := kube.CoreV1().Events("tenant-ns").List(ctx, metav1.ListOptions{})
	require.NoError(t, err)
	assert.Equal(t, TenantNetworkKind, events.Items[0].InvolvedObject.Kind)
	assert.Equal(t, "acme", events.Items[0].InvolvedObject.Name)
	assert.Equal(t, "node-1", events.Items[0].Source.Host)

	// The director stops answering on a node that was ready: one warning at once, and the reason is on the node.
	run.failures = map[string]error{"ping": errors.New("timeout")}
	r.step(ctx)
	r.step(ctx)
	assert.Equal(t, []string{ReasonNodeAttached, ReasonNodeReady, ReasonNodeNotReady}, eventReasons(t, kube, "tenant-ns"))
	assert.Contains(t, annotations()[MessageAnnotation], "no tenant director answers")
	// A failure is an event of the type Error; everything else is Normal.
	assert.Equal(t, map[string]string{
		ReasonNodeAttached: corev1.EventTypeNormal, ReasonNodeReady: corev1.EventTypeNormal, ReasonNodeNotReady: EventTypeError,
	}, eventTypes(t, kube, "tenant-ns"))

	// The node leaves the tenant: it is detached, and its annotations are removed.
	node, err := kube.CoreV1().Nodes().Get(ctx, "node-1", metav1.GetOptions{})
	require.NoError(t, err)
	delete(node.Labels, TenantLabelKey)
	_, err = kube.CoreV1().Nodes().Update(ctx, node, metav1.UpdateOptions{})
	require.NoError(t, err)
	r.shutdown()
	reasons := eventReasons(t, kube, "tenant-ns")
	assert.Equal(t, ReasonNodeDetached, reasons[len(reasons)-1])
	assert.NotContains(t, annotations(), OverlayAnnotation)
	assert.NotContains(t, annotations(), InterfaceAnnotation)
	assert.NotContains(t, annotations(), MessageAnnotation)
}

func TestShutdownFollowsTheNodeSelectorOfTheDaemonSet(t *testing.T) {
	ctx := context.Background()
	api := &fakeAPI{network: &panapi.TenantNetwork{TenantName: "acme", SubnetID: "s1", Directors: []string{"192.168.0.2"}}}
	run := &fakeRunner{outputs: map[string]string{readLinks: matchingLink, readFDB: matchingFDB}}
	r, kube := newTestReconciler(t, api, run)
	setSelector := func(selector map[string]string) {
		daemonSet, err := kube.AppsV1().DaemonSets("tenant-ns").Get(ctx, "panfs-tenant-network", metav1.GetOptions{})
		require.NoError(t, err)
		daemonSet.Spec.Template.Spec.NodeSelector = selector
		_, err = kube.AppsV1().DaemonSets("tenant-ns").Update(ctx, daemonSet, metav1.UpdateOptions{})
		require.NoError(t, err)
	}

	// The selector moved to another label after this pod started: the node left the tenant,
	// although it still has the label of the tenant.
	setSelector(map[string]string{"pool": "storage"})
	removed, reason := r.removedFromTenant(ctx)
	assert.True(t, removed)
	assert.Equal(t, "the node has no label pool=storage", reason)

	setSelector(map[string]string{TenantLabelKey: "acme"})
	removed, _ = r.removedFromTenant(ctx)
	assert.False(t, removed)

	// No selector means all nodes: a node never leaves the tenant by its labels.
	setSelector(nil)
	node, err := kube.CoreV1().Nodes().Get(ctx, "node-1", metav1.GetOptions{})
	require.NoError(t, err)
	node.Labels = nil
	_, err = kube.CoreV1().Nodes().Update(ctx, node, metav1.UpdateOptions{})
	require.NoError(t, err)
	removed, _ = r.removedFromTenant(ctx)
	assert.False(t, removed)
}

func TestReconcilerWarnsAboutAStartingNodeOnlyAfterASecondFailure(t *testing.T) {
	ctx := context.Background()
	api := &fakeAPI{network: &panapi.TenantNetwork{TenantName: "acme", SubnetID: "s1", Directors: []string{"192.168.0.2"}}}
	run := &fakeRunner{
		outputs:  map[string]string{readLinks: matchingLink, readFDB: matchingFDB},
		failures: map[string]error{"ping": errors.New("timeout")},
	}
	r, kube := newTestReconciler(t, api, run)
	r.Events = NewTenantNetworkEvents(kube, "tenant-ns", "acme", "uid-1", "panfs-tenant-network", "node-1", r.Log)

	// The first check of a new interface often fails: no warning yet.
	r.step(ctx)
	assert.Equal(t, []string{ReasonNodeAttached}, eventReasons(t, kube, "tenant-ns"))

	// It fails again: now it is worth a warning, once.
	r.step(ctx)
	r.step(ctx)
	assert.Equal(t, []string{ReasonNodeAttached, ReasonNodeNotReady}, eventReasons(t, kube, "tenant-ns"))
}

func TestReconcilerSetsTheReadyValueItWasGiven(t *testing.T) {
	api := &fakeAPI{network: &panapi.TenantNetwork{TenantName: "acme", SubnetID: "s1", Directors: []string{"192.168.0.2"}}}
	run := &fakeRunner{outputs: map[string]string{readLinks: matchingLink, readFDB: matchingFDB}}
	r, kube := newTestReconciler(t, api, run)
	r.Config.ReadyValue = "tenant-ns"

	r.step(context.Background())
	assert.Equal(t, "tenant-ns", nodeLabels(t, kube)[ReadyLabelKey])
}

// vlanAttachment is the record of a host on a VLAN segment: no tunnel recipe.
func vlanAttachment() *panapi.Attachment {
	return &panapi.Attachment{
		ID: "a1", SubnetID: "s1", UnderlayIP: "10.0.0.5", Hostname: "node-1",
		OverlayIP: "192.168.0.58", OverlayPrefix: 16, MTU: 1500,
		SegmentType: panapi.SegmentTypeVLAN, SegmentID: 123,
	}
}

// hostLinks is the `ip` output of a node with its underlay address on bond0 and no tenant interface.
const hostLinks = `[{"ifname":"lo","flags":["UP"],"mtu":65536},
{"ifname":"eth1","flags":["UP"],"mtu":1500},
{"ifname":"bond0","flags":["UP"],"mtu":1500,"addr_info":[{"family":"inet","local":"10.0.0.5","prefixlen":24}]}]`

// matchingVLANLink adds the interface that already matches vlanAttachment.
var matchingVLANLink = strings.TrimSuffix(hostLinks, "]") + `,
{"ifname":"vlan123","link":"bond0","flags":["BROADCAST","UP","LOWER_UP"],"mtu":1500,"address":"aa:bb:cc:00:00:01",
 "linkinfo":{"info_kind":"vlan","info_data":{"protocol":"802.1Q","id":123}},
 "addr_info":[{"family":"inet","local":"192.168.0.58","prefixlen":16}]}]`

func TestEnsureVLANInterface(t *testing.T) {
	ctx := context.Background()

	t.Run("a matching interface is not touched", func(t *testing.T) {
		run := &fakeRunner{outputs: map[string]string{readLinks: matchingVLANLink}}
		name, changed, err := EnsureInterface(ctx, run, vlanAttachment())
		require.NoError(t, err)
		assert.Equal(t, "vlan123", name)
		assert.False(t, changed)
		assert.Empty(t, run.changes)
	})

	t.Run("a missing interface is created on the interface with the underlay address", func(t *testing.T) {
		run := &fakeRunner{outputs: map[string]string{readLinks: hostLinks}}
		_, changed, err := EnsureInterface(ctx, run, vlanAttachment())
		require.NoError(t, err)
		assert.True(t, changed)
		// No MAC address and no flood list: the VLAN fabric forwards and learns on its own.
		assert.Equal(t, []string{
			"ip link add link bond0 name vlan123 type vlan id 123",
			"ip link set vlan123 mtu 1500",
			"ip link set vlan123 up",
			"ip addr add 192.168.0.58/16 dev vlan123",
		}, run.changes)
	})

	t.Run("a named parent replaces the search by address", func(t *testing.T) {
		run := &fakeRunner{outputs: map[string]string{readLinks: hostLinks}}
		_, _, err := EnsureTenantInterface(ctx, run, vlanAttachment(), "eth1")
		require.NoError(t, err)
		assert.Equal(t, "ip link add link eth1 name vlan123 type vlan id 123", run.changes[0])
	})

	t.Run("mutable settings are corrected in place", func(t *testing.T) {
		link := strings.NewReplacer(`"mtu":1500,"address"`, `"mtu":1400,"address"`, `192.168.0.58`, `192.168.0.99`).Replace(matchingVLANLink)
		run := &fakeRunner{outputs: map[string]string{readLinks: link}}
		_, changed, err := EnsureInterface(ctx, run, vlanAttachment())
		require.NoError(t, err)
		assert.True(t, changed)
		assert.Equal(t, []string{
			"ip link set vlan123 mtu 1500",
			"ip addr del 192.168.0.99/16 dev vlan123",
			"ip addr add 192.168.0.58/16 dev vlan123",
		}, run.changes)
	})

	t.Run("an interface on another parent or VLAN id is built again", func(t *testing.T) {
		for name, replace := range map[string][]string{
			"parent":  {`"link":"bond0"`, `"link":"eth1"`},
			"VLAN id": {`"id":123`, `"id":124`},
			"kind":    {`"info_kind":"vlan"`, `"info_kind":"dummy"`},
		} {
			t.Run(name, func(t *testing.T) {
				run := &fakeRunner{outputs: map[string]string{readLinks: strings.Replace(matchingVLANLink, replace[0], replace[1], 1)}}
				_, changed, err := EnsureInterface(ctx, run, vlanAttachment())
				require.NoError(t, err)
				assert.True(t, changed)
				assert.Equal(t, "ip link del vlan123", run.changes[0])
				assert.Equal(t, "ip link add link bond0 name vlan123 type vlan id 123", run.changes[1])
			})
		}
	})

	t.Run("a record or a parent that cannot be used is refused before any command", func(t *testing.T) {
		cases := map[string]struct {
			change func(a *panapi.Attachment)
			parent string
			links  string
			want   string
		}{
			"VLAN id 0":          {change: func(a *panapi.Attachment) { a.SegmentID = 0 }, links: hostLinks, want: "not a VLAN id"},
			"VLAN id too large":  {change: func(a *panapi.Attachment) { a.SegmentID = 4095 }, links: hostLinks, want: "not a VLAN id"},
			"unknown segment":    {change: func(a *panapi.Attachment) { a.SegmentType = "none" }, links: hostLinks, want: `segment type "none" is not supported`},
			"parent not a name":  {parent: "eth0; reboot", links: hostLinks, want: "not a valid interface name"},
			"parent missing":     {parent: "eth9", links: hostLinks, want: `"eth9" does not exist on the node`},
			"no underlay on any": {links: `[{"ifname":"lo"}]`, want: "no interface of the node has the underlay address 10.0.0.5"},
		}
		for name, tc := range cases {
			t.Run(name, func(t *testing.T) {
				run := &fakeRunner{outputs: map[string]string{readLinks: tc.links}}
				a := vlanAttachment()
				if tc.change != nil {
					tc.change(a)
				}
				_, _, err := EnsureTenantInterface(ctx, run, a, tc.parent)
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.want)
				assert.Empty(t, run.changes)
			})
		}
	})
}

// vlanAPI is a PanAPI whose tenant subnet is a VLAN segment.
type vlanAPI struct{ fakeAPI }

func (f *vlanAPI) AttachHost(context.Context, string, string, string, map[string]string) (*panapi.Attachment, error) {
	return vlanAttachment(), nil
}

func (f *vlanAPI) FindAttachment(context.Context, string, string, map[string]string) (*panapi.Attachment, error) {
	if len(f.detached) > 0 {
		return nil, nil
	}
	return vlanAttachment(), nil
}

func TestReconcilerOnAVLANSegment(t *testing.T) {
	ctx := context.Background()
	api := &vlanAPI{fakeAPI{network: &panapi.TenantNetwork{TenantName: "acme", SubnetID: "s1", Directors: []string{"192.168.0.2"}}}}
	run := &fakeRunner{outputs: map[string]string{readLinks: hostLinks}}
	r, kube := newTestReconciler(t, api, run)
	r.Events = NewTenantNetworkEvents(kube, "tenant-ns", "acme", "uid-1", "panfs-tenant-network", "node-1", r.Log)

	// The node becomes ready through a VLAN interface, and it reports the interface like on a VXLAN segment.
	r.step(ctx)
	assert.Equal(t, "acme", nodeLabels(t, kube)[ReadyLabelKey])
	assert.Equal(t, "ip link add link bond0 name vlan123 type vlan id 123", run.changes[0])
	node, err := kube.CoreV1().Nodes().Get(ctx, "node-1", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "vlan123", node.Annotations[InterfaceAnnotation])
	assert.Equal(t, []string{ReasonNodeAttached, ReasonInterfaceConfigured, ReasonNodeReady}, eventReasons(t, kube, "tenant-ns"))
	// The fake API server gives every generated event the same empty name, so read the messages from the calls.
	var messages []string
	for _, action := range kube.Actions() {
		if create, ok := action.(k8stesting.CreateAction); ok && action.Matches("create", "events") {
			messages = append(messages, create.GetObject().(*corev1.Event).Message)
		}
	}
	assert.Contains(t, messages[1], "interface vlan123 was created or corrected (VLAN 123, MTU 1500)")

	// The node leaves the tenant: the VLAN interface is removed and the node is detached.
	run.outputs[readLinks] = matchingVLANLink
	run.changes = nil
	delete(node.Labels, TenantLabelKey)
	_, err = kube.CoreV1().Nodes().Update(ctx, node, metav1.UpdateOptions{})
	require.NoError(t, err)
	r.shutdown()
	assert.Contains(t, run.changes, "ip link del vlan123")
	assert.Equal(t, []string{"a1"}, api.detached)
}

func TestShutdownDoesNotDetachWhenItCannotReadTheCluster(t *testing.T) {
	forbidden := apierrors.NewForbidden(schema.GroupResource{Resource: "nodes"}, "node-1", errors.New("no permission"))
	cases := map[string]struct{ verb, resource string }{
		"the node cannot be read":      {"get", "nodes"},
		"the DaemonSet cannot be read": {"get", "daemonsets"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			api := &fakeAPI{network: &panapi.TenantNetwork{TenantName: "acme", SubnetID: "s1", Directors: []string{"192.168.0.2"}}}
			run := &fakeRunner{outputs: map[string]string{readLinks: matchingLink, readFDB: matchingFDB}}
			r, kube := newTestReconciler(t, api, run)
			// The permissions of the pod are gone, for example while the operator is installed again.
			kube.PrependReactor(tc.verb, tc.resource, func(k8stesting.Action) (bool, runtime.Object, error) {
				return true, nil, forbidden
			})

			removed, _ := r.removedFromTenant(context.Background())
			assert.False(t, removed)
			r.shutdown()
			assert.Empty(t, api.detached)
			assert.Empty(t, run.changes)
		})
	}
}
