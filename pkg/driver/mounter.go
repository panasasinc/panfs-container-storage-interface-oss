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

package driver

import (
	"fmt"
	"os"

	"k8s.io/mount-utils"
)

//go:generate mockgen -source=mounter.go -destination=mock/mock_kmounter.go -package=mock kMounter
//lint:ignore U1000 This interface is intentionally kept for future use and should be ignored by the linter
type kMounter interface {
	mount.Interface
}

// PanFSMounter provides methods to mount PanFS volumes.
type PanFSMounter struct {
	mounter mount.Interface
}

// Mount mounts the PanFS volume at the target path with the given options.
// Creates the target directory if it does not exist and performs the mount operation.
//
// Parameters:
//
//	source  - The source path to mount.
//	target  - The target mount point.
//	options - Slice of mount options.
//
// Returns:
//
//	error - Returns an error if mount fails or target cannot be created.
func (p *PanFSMounter) Mount(source, target string, options []string) error {
	// Custom mount logic can be added here if needed
	notMnt, err := p.mounter.IsLikelyNotMountPoint(target)
	if err != nil {
		if os.IsNotExist(err) {
			err = makeDir(target)
			if err != nil {
				return err
			}
		} else {
			return fmt.Errorf("failed to check mount path: %w", err)
		}
	}

	if notMnt {
		err = p.mounter.Mount(source, target, "panfs", options)
		if err != nil {
			return err
		}
	}
	return nil
}

// BindMount is an alias for Mount with "bind" option.
// BindMount performs a bind mount of the source to the target with the given options.
// Adds the "bind" option and calls Mount.
//
// Parameters:
//
//	source  - The source path to bind mount.
//	target  - The target mount point.
//	options - Slice of mount options.
//
// Returns:
//
//	error - Returns an error if bind mount fails.
func (p *PanFSMounter) BindMount(source, target string, options []string) error {
	options = append(options, "bind")
	return p.Mount(source, target, options)
}

// Unmount unmounts the PanFS volume from the target path.
//
// Parameters:
//
//	target - The target mount point to unmount.
//
// Returns:
//
//	error - Returns an error if unmount fails.
func (p *PanFSMounter) Unmount(target string) error {
	return mount.CleanupMountPoint(target, p.mounter, false)
}

// NewPanFSMounter creates a new PanFSMounter instance using the default mount interface.
//
// Returns:
//
//	*PanFSMounter - The initialized PanFSMounter.
func NewPanFSMounter() *PanFSMounter {
	return &PanFSMounter{
		mounter: mount.New(""),
	}
}

// PanFSFakeMounter is a fake mounter for PanFS used in tests.
type PanFSFakeMounter struct {
	fakeMounter *mount.FakeMounter
}

// NewPanFSFakeMounter creates a new PanFSFakeMounter for testing purposes.
// Uses a FakeMounter and a real mount cleanup function.
//
// Returns:
//
//	*PanFSFakeMounter - The initialized PanFSFakeMounter.
func NewPanFSFakeMounter() *PanFSFakeMounter {
	return &PanFSFakeMounter{
		fakeMounter: &mount.FakeMounter{
			MountPoints: nil,
			UnmountFunc: func(path string) error {
				// use here a real mount clean up func to be sure that mount paths created by tests are deleted
				return mount.CleanupMountPoint(path, mount.New(""), false)
			},
		},
	}
}

// Mount mounts the PanFS volume at the target path using the fake mounter for tests.
// Creates the target directory if it does not exist and performs the mount operation if not already mounted.
//
// Parameters:
//
//	source  - The source path to mount.
//	target  - The target mount point.
//	options - Slice of mount options.
//
// Returns:
//
//	error - Returns an error if mount fails or target cannot be created.
func (p *PanFSFakeMounter) Mount(source, target string, options []string) error {
	realMounter := mount.New("")
	isMnt, err := realMounter.IsMountPoint(target)
	if err != nil {
		if os.IsNotExist(err) {
			err = makeDir(target)
			if err != nil {
				return err
			}
		}
	}
	// target is not mounted - do mount
	if !isMnt {
		return p.fakeMounter.Mount(source, target, "panfs", options)
	}
	// target is already mounted - do nothing
	return nil
}

// BindMount performs a bind mount of the source to the target with the given options using the fake mounter.
// Adds the "bind" option and calls Mount.
//
// Parameters:
//
//	source  - The source path to bind mount.
//	target  - The target mount point.
//	options - Slice of mount options.
//
// Returns:
//
//	error - Returns an error if bind mount fails.
func (p *PanFSFakeMounter) BindMount(source, target string, options []string) error {
	options = append(options, "bind")
	return p.Mount(source, target, options)
}

// Unmount unmounts the PanFS volume from the target path using the fake mounter.
//
// Parameters:
//
//	target - The target mount point to unmount.
//
// Returns:
//
//	error - Returns an error if unmount fails.
func (p *PanFSFakeMounter) Unmount(target string) error {
	return p.fakeMounter.Unmount(target)
}
