// Copyright 2026 VDURA Inc.
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

package panapi

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// FakeClient is an in-memory stand-in for Client, used by csi-sanity runs that have no realm.
// It keeps the same contract as Client: the same sentinel errors and the same id and name rules.
type FakeClient struct {
	mu      sync.Mutex
	nextID  int64
	volumes map[int64]*Volume
}

// NewFakeClient returns an empty FakeClient.
func NewFakeClient() *FakeClient {
	return &FakeClient{nextID: 1, volumes: make(map[int64]*Volume)}
}

// CreateVolume stores a new volume, or returns ErrorAlreadyExist when the name is taken.
func (f *FakeClient) CreateVolume(_ context.Context, p CreateVolumeParams, _ map[string]string) (*Volume, error) {
	req, err := buildCreateRequest(p)
	if err != nil {
		return nil, err
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	for _, vol := range f.volumes {
		if vol.Pathname == req.Pathname {
			return nil, fmt.Errorf("%w: %s", ErrorAlreadyExist, req.Pathname)
		}
	}

	soft, hard := p.SoftQuotaBytes, p.HardQuotaBytes
	vol := &Volume{
		ID:             f.nextID,
		Pathname:       req.Pathname,
		Status:         "online",
		StoragesetID:   req.StoragesetID,
		SoftQuotaBytes: &soft,
		HardQuotaBytes: &hard,
	}
	f.volumes[vol.ID] = vol
	f.nextID++

	stored := *vol
	return &stored, nil
}

// GetVolume returns the volume with the given CSI volume ID, or ErrorNotFound.
func (f *FakeClient) GetVolume(_ context.Context, volumeID string, _ map[string]string) (*Volume, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	vol, err := f.find(volumeID)
	if err != nil {
		return nil, err
	}
	stored := *vol
	return &stored, nil
}

// GetVolumeByName returns the volume with the given name, or ErrorNotFound.
func (f *FakeClient) GetVolumeByName(_ context.Context, name string, _ map[string]string) (*Volume, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	want := strings.TrimPrefix(name, "/")
	for _, vol := range f.volumes {
		if vol.Name() == want {
			stored := *vol
			return &stored, nil
		}
	}
	return nil, fmt.Errorf("%w: no volume named %s", ErrorNotFound, name)
}

// ExpandVolume raises the quotas of a volume the way Client does.
func (f *FakeClient) ExpandVolume(_ context.Context, volumeID string, sizeBytes int64, _ map[string]string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	vol, err := f.find(volumeID)
	if err != nil {
		return err
	}

	size := sizeBytes
	vol.SoftQuotaBytes = &size
	if hard := vol.HardQuota(); hard > 0 && hard < sizeBytes {
		vol.HardQuotaBytes = &size
	}
	return nil
}

// DeleteVolume removes a volume, or returns ErrorNotFound.
func (f *FakeClient) DeleteVolume(_ context.Context, volumeID string, _ map[string]string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	vol, err := f.find(volumeID)
	if err != nil {
		return err
	}
	delete(f.volumes, vol.ID)
	return nil
}

// find returns the stored volume for a CSI volume ID. The caller holds the lock.
func (f *FakeClient) find(volumeID string) (*Volume, error) {
	id, isID := ParseVolumeID(volumeID)
	if !isID {
		want := strings.TrimPrefix(volumeID, "/")
		for _, vol := range f.volumes {
			if vol.Name() == want {
				return vol, nil
			}
		}
		return nil, fmt.Errorf("%w: no volume named %s", ErrorNotFound, volumeID)
	}
	vol, ok := f.volumes[id]
	if !ok {
		return nil, fmt.Errorf("%w: volume %d", ErrorNotFound, id)
	}
	return vol, nil
}
