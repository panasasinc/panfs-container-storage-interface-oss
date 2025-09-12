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

// Package driver provides an implementation of the Container Storage Interface (CSI) for PanFS.
// It defines the main Driver type, interfaces for storage provider and mounter operations,
// and constants used for volume management and configuration.
package driver

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/panasasinc/panfs-container-storage-interface-oss/pkg/pancli"
	"github.com/panasasinc/panfs-container-storage-interface-oss/pkg/utils"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"k8s.io/klog/v2"
)

//go:generate mockgen -source=driver.go -destination=mock/mock_driver.go -package=mock StorageProviderClient PanMounter

// StorageProviderClient defines an interface for managing volumes with a storage provider.
type StorageProviderClient interface {
	CreateVolume(volumeName string, params *pancli.VolumeCreateParams, secret map[string]string) (*utils.Volume, error)
	DeleteVolume(volID string, secret map[string]string) error
	ExpandVolume(volumeName string, targetSize int64, secret map[string]string) error
	ListVolumes(secret map[string]string) (*utils.VolumeList, error)
	GetVolume(volumeName string, secret map[string]string) (*utils.Volume, error)
}

// PanMounter defines the interface for mounting and unmounting PanFS volumes.
type PanMounter interface {
	Mount(source string, target string, options []string) error
	BindMount(source string, target string, options []string) error
	Unmount(target string) error
}

// Driver represents the CSI driver for PanFS, implementing identity, controller, and node services.
type Driver struct {
	Version string
	Name    string

	endpoint  string
	host      string
	log       klog.Logger
	mounterV2 PanMounter
	panfs     StorageProviderClient
	csi.UnimplementedIdentityServer
	csi.UnimplementedControllerServer
	csi.UnimplementedNodeServer
}

// Exportable constants
const (
	// EphemeralK8SVolumeContext is a volume context key which indicating that k8s requests ephemeral volume. CSI PanFS
	// plugin does not support ephemeral volumes for now
	EphemeralK8SVolumeContext = "csi.storage.k8s.io/ephemeral"
	PanFSFilesystemType       = "panfs"
	VendorPrefix              = "panfs.csi.vdura.com"
)

// Volume parameters constants
const (
	DefaultDriverName string = "com.vdura.csi.panfs"
	bladeSet                 = VendorPrefix + "bladeset"
	recoveryPriority         = VendorPrefix + "recoverypriority"
	efsa                     = VendorPrefix + "efsa"
	volService               = VendorPrefix + "volservice"
	layout                   = VendorPrefix + "layout"
	maxWidth                 = VendorPrefix + "maxwidth"
	stripeUnit               = VendorPrefix + "stripeunit"
	rgWidth                  = VendorPrefix + "rgwidth"
	rgDepth                  = VendorPrefix + "rgdepth"
	user                     = VendorPrefix + "user"
	group                    = VendorPrefix + "group"
	uPerm                    = VendorPrefix + "uperm"
	gPerm                    = VendorPrefix + "gperm"
	oPerm                    = VendorPrefix + "operm"

	realmIP    = "realm_ip"
	sshUser    = "user"
	password   = "password"
	privateKey = "private_key"
)

// CreateDriver initializes a new Driver instance with the provided configuration and dependencies.
//
// Parameters:
//
//	version    - The version string of the driver.
//	driverName - The name of the CSI driver.
//	endpoint   - The gRPC endpoint address to listen on.
//	panfs      - The StorageProviderClient implementation for PanFS operations.
//	log        - The logger instance for logging.
//	mounterV2  - The PanMounter implementation for mount operations.
//
// Returns:
//
//	*Driver - A pointer to the initialized Driver instance, or nil if hostname retrieval fails.
func CreateDriver(
	version, driverName, endpoint string,
	panfs StorageProviderClient,
	log klog.Logger,
	mounterV2 PanMounter,
) *Driver {
	log.Info("creating driver", "driver_name", driverName, "endpoint", endpoint, "version", version)
	host, err := os.Hostname()
	if err != nil {
		log.Error(err, "failed to get hostname of the node")
		return nil
	}

	return &Driver{
		Version:   version,
		Name:      driverName,
		endpoint:  endpoint,
		mounterV2: mounterV2,
		log:       log,
		host:      host,
		panfs:     panfs,
	}
}

// Run starts the gRPC server and listens for incoming CSI requests.
//
// Returns:
//
//	error - Returns an error if the server fails to start, listen, or shut down gracefully.
//
// Error Cases:
//   - Failure to remove the endpoint address before starting.
//   - Failure to listen on the endpoint address.
//   - Failure to serve or gracefully stop the gRPC server.
func (d *Driver) Run() error {
	d.log.Info("starting gRPC server")

	if err := os.Remove(d.endpoint); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove address %s: %v", d.endpoint, err)
	}

	lis, err := net.Listen("unix", d.endpoint)
	if err != nil {
		return fmt.Errorf("failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	csi.RegisterIdentityServer(grpcServer, d)
	csi.RegisterControllerServer(grpcServer, d)
	csi.RegisterNodeServer(grpcServer, d)

	reflection.Register(grpcServer)

	shutdownError := make(chan error)

	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		s := <-quit

		d.log.Info("shutting down server", "signal", s.String())

		grpcServer.GracefulStop()

		shutdownError <- nil
	}()

	d.log.Info("successfully registered services", "address", d.endpoint)

	err = grpcServer.Serve(lis)
	if !errors.Is(err, grpc.ErrServerStopped) {
		return err
	}

	err = <-shutdownError
	if err != nil {
		return err
	}

	d.log.Info("gRPC server stopped")

	return nil
}
