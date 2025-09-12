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
	"flag"
	"os"

	"github.com/panasasinc/panfs-container-storage-interface-oss/pkg/driver"
	"github.com/panasasinc/panfs-container-storage-interface-oss/pkg/pancli"

	"k8s.io/klog/v2"
)

var version = "unversioned"

// config holds the configuration for the CSI driver.
type config struct {
	endpoint   string
	driverName string
	sanity     bool
}

var (
	cfg config
	log klog.Logger
)

// init initializes the command-line flags and logging.
func init() {
	// init klog flags. See klog docs for details
	klog.InitFlags(nil)

	flag.StringVar(&cfg.endpoint, "endpoint", "/tmp/csi.sock", "CSI endpoint")
	flag.StringVar(&cfg.driverName, "driverName", driver.DefaultDriverName, "Name of CSI driver")
	flag.Parse()

	log = klog.NewKlogr()
	log.Info("Klog logger initialized", "verbosity", flag.Lookup("v").Value.String())
}

// main is the entry point for the CSI driver application.
func main() {
	defer klog.Flush()

	if os.Getenv("CSI_SANITY_MODE") == "true" {
		cfg.sanity = true
	}

	var panfs driver.StorageProviderClient
	var mounter driver.PanMounter
	if cfg.sanity {
		klog.Info("CSI sanity mode enabled: using mock pancli and mounter")
		panfs = pancli.NewFakePancliSSHClient()
		mounter = driver.NewPanFSFakeMounter()
	} else {
		klog.Info("Starting driver in default operation mode")
		panfs = pancli.NewPancliSSHClient(pancli.NewSSHClient())
		mounter = driver.NewPanFSMounter()
	}

	d := driver.CreateDriver(version, cfg.driverName, cfg.endpoint, panfs, log, mounter)

	err := d.Run()
	if err != nil {
		klog.Exit(err)
		os.Exit(1)
	}
}
