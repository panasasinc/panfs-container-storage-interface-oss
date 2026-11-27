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

// Command panfs-storageclass-operator runs once per cluster. It watches the PanfsStorageClass resources and
// creates, for each of them, a StorageClass of the PanFS CSI driver. It also lets the CSI controller read
// the realm Secret that the StorageClass names.
package main

import (
	"context"
	"flag"
	"os/signal"
	"syscall"
	"time"

	"github.com/panasasinc/panfs-container-storage-interface-oss/pkg/storageclass"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
)

var version = "unversioned"

func main() {
	klog.InitFlags(nil)

	var options storageclass.Options
	flag.StringVar(&options.CSIDriverNamespace, "csi-driver-namespace", "",
		"Namespace of the PanFS CSI driver; the default is the namespace that the CSIDriver object names")
	flag.StringVar(&options.CSIControllerServiceAccount, "csi-controller-service-account", "",
		"ServiceAccount of the CSI controller, which reads the realm Secret of a StorageClass; "+
			"the default is <namespace of the driver>-controller")
	resync := flag.Duration("resync", 15*time.Second, "Time between two passes over the PanfsStorageClass resources")
	flag.Parse()
	defer klog.Flush()

	log := klog.NewKlogr()
	log.Info("starting panfs-storageclass-operator", "version", version, "resync", resync.String())
	if *resync < time.Second {
		klog.Exit("the resync time must be one second or longer")
	}

	kubeConfig, err := rest.InClusterConfig()
	if err != nil {
		klog.Exitf("cannot read the in-cluster configuration: %v", err)
	}
	kube, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		klog.Exitf("cannot create the Kubernetes client: %v", err)
	}
	dynamicClient, err := dynamic.NewForConfig(kubeConfig)
	if err != nil {
		klog.Exitf("cannot create the dynamic Kubernetes client: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	(&storageclass.Controller{
		Kube:    kube,
		Dynamic: dynamicClient,
		Options: options,
		Resync:  *resync,
		Log:     log,
	}).Run(ctx)
}
