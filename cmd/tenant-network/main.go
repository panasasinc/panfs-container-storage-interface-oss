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

// Command panfs-tenant-network puts the Kubernetes node it runs on onto the network of a PanFS tenant.
//
// It attaches the node to the tenant subnet through PanAPI, builds the tenant interface, and sets the
// node label panfs.vdura.com/tenant-network while a tenant director answers.
//
// With --operator the same program runs once per cluster instead: it watches the TenantNetwork resources
// and creates, for each of them, a DaemonSet that runs this program on the nodes of the tenant.
// It also watches the PanfsStorageClass resources and creates a StorageClass for each of them.
package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/panasasinc/panfs-container-storage-interface-oss/pkg/panapi"
	"github.com/panasasinc/panfs-container-storage-interface-oss/pkg/tenantnet"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
)

var version = "unversioned"

func main() {
	klog.InitFlags(nil)

	var cfg tenantnet.Config
	var firewalldZone, dbusSocket string
	flag.StringVar(&cfg.Tenant, "tenant", os.Getenv("TENANT"), "Name of the PanFS tenant (env TENANT)")
	flag.StringVar(&cfg.NodeName, "node-name", os.Getenv("NODE_NAME"), "Name of this Kubernetes node (env NODE_NAME)")
	flag.StringVar(&cfg.HostIP, "host-ip", os.Getenv("HOST_IP"), "Address of this node on the realm underlay (env HOST_IP)")
	flag.StringVar(&cfg.SecretDir, "secret-dir", "/etc/panfs-tenant-network/secret",
		"Directory with the mounted Secret: the keys api, token and, optionally, ca_cert")
	flag.StringVar(&cfg.ReadyLabelKey, "ready-label", tenantnet.ReadyLabelKey,
		"Key of the node label that is set while the tenant network works")
	flag.StringVar(&cfg.ReadyValue, "ready-value", os.Getenv("READY_VALUE"),
		"Value of the ready label (env READY_VALUE); the default is the tenant name")
	flag.StringVar(&cfg.ReadyFile, "ready-file", "/tmp/ready", "File that exists while the tenant network works")
	flag.DurationVar(&cfg.Interval, "interval", 30*time.Second, "Time between two checks of the tenant network")
	flag.StringVar(&firewalldZone, "firewalld-zone", "trusted",
		"firewalld zone of the tenant interface; an empty value leaves firewalld alone")
	flag.StringVar(&dbusSocket, "dbus-socket", "/run/dbus/system_bus_socket",
		"Path of the system D-Bus socket of the host, used to reach firewalld")
	flag.StringVar(&cfg.Namespace, "namespace", os.Getenv("POD_NAMESPACE"), "Namespace of the DaemonSet (env POD_NAMESPACE)")
	flag.StringVar(&cfg.DaemonSet, "daemonset", os.Getenv("DAEMONSET_NAME"),
		"Name of the DaemonSet this pod belongs to (env DAEMONSET_NAME); used to tell a removal from a restart")
	flag.StringVar(&cfg.VLANParent, "vlan-parent", "",
		"Interface that carries the VLAN when the tenant subnet is a VLAN segment; "+
			"the default is the interface with the underlay address of the node")
	flag.BoolVar(&cfg.DetachOnRemoval, "detach-on-removal", true,
		"Remove the tenant interface and detach the node from the subnet when the node leaves the tenant")
	operator := flag.Bool("operator", false,
		"Run as the operator: create one DaemonSet of this program for every TenantNetwork resource")
	var options tenantnet.NodeOptions
	var pullSecrets string
	flag.StringVar(&options.Image, "node-image", os.Getenv("NODE_IMAGE"),
		"Operator: image of the pods that the operator creates (env NODE_IMAGE)")
	flag.StringVar(&options.PullPolicy, "node-image-pull-policy", "IfNotPresent", "Operator: image pull policy of the pods")
	flag.StringVar(&pullSecrets, "node-image-pull-secrets", "",
		"Operator: comma-separated names of image pull secrets of the pods; they must exist in every tenant namespace")
	flag.StringVar(&options.ClusterRole, "node-cluster-role", "",
		"Operator: name of the ClusterRole that lets a pod read and patch nodes")
	flag.StringVar(&options.PriorityClassName, "node-priority-class", "", "Operator: priority class of the pods")
	flag.StringVar(&options.CSIDriverNamespace, "csi-driver-namespace", "",
		"Operator: namespace of the PanFS CSI driver, for the StorageClasses the operator creates; "+
			"the default is the namespace that the CSIDriver object names")
	resync := flag.Duration("resync", 15*time.Second, "Operator: time between two passes over the TenantNetwork resources")
	flag.Parse()
	defer klog.Flush()

	log := klog.NewKlogr()
	log.Info("starting panfs-tenant-network", "version", version, "tenant", cfg.Tenant, "node", cfg.NodeName,
		"hostIP", cfg.HostIP, "interval", cfg.Interval.String())

	kubeConfig, err := rest.InClusterConfig()
	if err != nil {
		klog.Exitf("cannot read the in-cluster configuration: %v", err)
	}
	kube, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		klog.Exitf("cannot create the Kubernetes client: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	if *operator {
		if options.Image == "" || options.ClusterRole == "" {
			klog.Exit("the node image and the node cluster role are required for the operator")
		}
		if cfg.Interval < time.Second || *resync < time.Second {
			klog.Exit("the interval and the resync time must be one second or longer")
		}
		dynamicClient, err := dynamic.NewForConfig(kubeConfig)
		if err != nil {
			klog.Exitf("cannot create the dynamic Kubernetes client: %v", err)
		}
		for _, name := range strings.Split(pullSecrets, ",") {
			if name = strings.TrimSpace(name); name != "" {
				options.ImagePullSecrets = append(options.ImagePullSecrets, name)
			}
		}
		options.Interval, options.FirewalldZone, options.DetachOnRemoval = cfg.Interval, firewalldZone, cfg.DetachOnRemoval
		options.VLANParent = cfg.VLANParent
		options.LogLevel = flag.Lookup("v").Value.String()
		log.Info("running as the operator", "nodeImage", options.Image, "resync", resync.String())
		(&tenantnet.Operator{
			Kube:      kube,
			Dynamic:   dynamicClient,
			API:       panapi.NewClient(),
			Options:   options,
			Resync:    *resync,
			TenantTTL: 5 * time.Minute,
			Log:       log,
		}).Run(ctx)
		return
	}

	if cfg.Tenant == "" || cfg.NodeName == "" || cfg.HostIP == "" {
		klog.Exit("the tenant, the node name and the host IP are required")
	}
	if cfg.Interval < time.Second {
		klog.Exit("the interval must be one second or longer")
	}

	// The operator names the TenantNetwork this pod belongs to; its events go there.
	var events *tenantnet.EventSink
	if name := os.Getenv("TENANT_NETWORK_NAME"); name != "" && cfg.Namespace != "" {
		events = tenantnet.NewTenantNetworkEvents(kube, cfg.Namespace, name, os.Getenv("TENANT_NETWORK_UID"),
			"panfs-tenant-network", cfg.NodeName, log)
	}

	runner := tenantnet.ExecRunner{}
	reconciler := &tenantnet.Reconciler{
		Config:   cfg,
		API:      panapi.NewClient(),
		Run:      runner,
		Firewall: &tenantnet.Firewalld{Run: runner, Socket: dbusSocket, Zone: firewalldZone},
		Kube:     kube,
		Events:   events,
		Log:      log,
	}

	reconciler.Loop(ctx)
}
