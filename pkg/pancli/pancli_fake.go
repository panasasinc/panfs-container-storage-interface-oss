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

package pancli

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/panasasinc/panfs-container-storage-interface-oss/pkg/utils"
)

// Log represents a record of a fake PanFS CLI action for testing purposes.
type Log struct {
	Action string
	Args   []string
}

// NewFakePancliSSHClient creates a new FakePancliSSHClient for testing purposes.
// Returns an initialized client with an empty volume list.
//
// Returns:
//
//	*FakePancliSSHClient - The initialized fake client.
func NewFakePancliSSHClient() *FakePancliSSHClient {
	return &FakePancliSSHClient{
		Volumes: make([]*utils.Volume, 0),
	}
}

// FakePancliSSHClient simulates a PanFS SSH client for testing purposes.
type FakePancliSSHClient struct {
	Volumes   []*utils.Volume
	ActionLog []Log
}

// CreateVolume creates a volume in the fake client.
// Returns an error if the volume already exists.
//
// Parameters:
//
//	volumeName - The name of the volume to create.
//	params     - The volume creation parameters.
//	_          - Unused secrets map.
//
// Returns:
//
//	*utils.Volume - The created volume object.
//	error         - Error if volume exists.
func (c *FakePancliSSHClient) CreateVolume(volumeName string, params *VolumeCreateParams, _ map[string]string) (*utils.Volume, error) {
	if _, err := c.getVolume(volumeName); err == nil {
		// no error means volume already exists
		return nil, ErrorAlreadyExist
	}

	bsetName := (*params)[utils.VolumeProvisioningContext.BladeSet.Key]
	if bsetName == "" {
		bsetName = "Set 1"
	}

	soft := (*params)[utils.VolumeProvisioningContext.Soft.Key]
	if soft == "" {
		soft = "0"
	}

	hard := (*params)[utils.VolumeProvisioningContext.Hard.Key]
	if hard == "" {
		hard = "0"
	}

	encryption := (*params)[utils.VolumeProvisioningContext.Encryption.Key]
	if encryption == "" {
		encryption = "off"
	}

	vol := &utils.Volume{
		Name: utils.VolumeName(volumeName),
		Bset: utils.Bladeset{
			ID:   "1",
			Name: bsetName,
		},
		State:      "Online",
		Soft:       utils.BytesStringToGB(soft),
		Hard:       utils.BytesStringToGB(hard),
		ID:         uuid.New().String(),
		Encryption: encryption,
	}
	c.Volumes = append(c.Volumes, vol)
	return vol, nil
}

// getVolume retrieves a volume by name from the fake client.
// Returns an error if not found.
//
// Parameters:
//
//	volumeName - The name of the volume to retrieve.
//
// Returns:
//
//	*utils.Volume - The found volume object.
//	error         - Error if not found.
func (c *FakePancliSSHClient) getVolume(volumeName string) (*utils.Volume, error) {
	for _, vol := range c.Volumes {
		if vol.Name == utils.VolumeName(volumeName) {
			return vol, nil
		}
	}
	return nil, fmt.Errorf("%w: %s", ErrorNotFound, volumeName)
}

// DeleteVolume deletes a volume by ID from the fake client.
// Returns an error if not found.
//
// Parameters:
//
//	volID - The ID of the volume to delete.
//	_     - Unused secrets map.
//
// Returns:
//
//	error - Error if not found.
func (c *FakePancliSSHClient) DeleteVolume(volID string, _ map[string]string) error {
	for i, vol := range c.Volumes {
		if vol.ID == volID {
			c.Volumes = append(c.Volumes[:i], c.Volumes[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("%w: %s", ErrorNotFound, "")
}

// ExpandVolume expands a volume to the target size in the fake client.
// Returns an error if not found.
//
// Parameters:
//
//	volumeName  - The name of the volume to expand.
//	targetSize  - The target size in bytes.
//	_           - Unused secrets map.
//
// Returns:
//
//	error - Error if not found.
func (c *FakePancliSSHClient) ExpandVolume(volumeName string, targetSize int64, _ map[string]string) error {
	vol, err := c.getVolume(volumeName)
	if err != nil {
		return err
	}
	vol.Soft = utils.BytesToGB(targetSize)
	return nil
}

// ListVolumes returns an empty volume list in the fake client.
//
// Parameters:
//
//	_ - Unused secrets map.
//
// Returns:
//
//	*utils.VolumeList - An empty volume list.
//	error             - Always nil.
func (c *FakePancliSSHClient) ListVolumes(_ map[string]string) (*utils.VolumeList, error) {
	return &utils.VolumeList{}, nil
}

// GetVolume retrieves a volume by name from the fake client.
//
// Parameters:
//
//	volumeName - The name of the volume to retrieve.
//	_          - Unused secrets map.
//
// Returns:
//
//	*utils.Volume - The found volume object.
//	error         - Error if not found.
func (c *FakePancliSSHClient) GetVolume(volumeName string, _ map[string]string) (*utils.Volume, error) {
	return c.getVolume(volumeName)
}
