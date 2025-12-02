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

// Package pancli provides SSH-based client implementations and utilities for interacting with PanFS storage systems.
// It defines types and functions for volume management, SSH command execution, and parameter handling.
package pancli

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/panasasinc/panfs-container-storage-interface-oss/pkg/utils"
	"golang.org/x/crypto/ssh"
	"k8s.io/klog/v2"
)

//go:generate mockgen -source=pancli_ssh.go -destination=mock/mock_runner.go -package=mock PancliRunner

// VolumeCreateParams represents the parameters for creating a volume.
type VolumeCreateParams map[string]string

// getOptionalParameters constructs a list of optional parameters for the volume creation command.
//
// Parameters:
//
//	params - The volume creation parameters.
//
// Returns:
//
//	[]string - Slice of command-line arguments.
func getOptionalParameters(params VolumeCreateParams) []string {
	opts := []string{}

	soft := utils.VolumeParameters.GetSCKey("soft")
	hard := utils.VolumeParameters.GetSCKey("hard")
	for key, value := range params {
		if value == "" {
			continue
		}

		keyParam := utils.VolumeParameters.GetSCKey(key)
		if keyParam == soft || keyParam == hard {
			// convert size from bytes to gigabytes for soft and hard quota parameters
			sizeBytes, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				continue
			}
			value = fmt.Sprintf("%.2f", utils.BytesToGB(sizeBytes))
		}

		if fmtStr := utils.VolumeParameters.GetFmt(keyParam); fmtStr != "" {
			opts = append(opts, fmt.Sprintf(fmtStr, value))
		}
	}

	return opts
}

// SSHRunner defines an interface for running commands over SSH.
type SSHRunner interface {
	RunCommand(secrets map[string]string, args ...string) ([]byte, error)
}

// SSHClient manages SSH connections and command execution.
type SSHClient struct {
	// cache for SSH connections to avoid creating a new connection for each command.
	// key is the realm address, value is the SSH client.
	clients map[string]*ssh.Client
	sync.Mutex
}

// NewSSHClient creates a new SSHClient instance for managing SSH connections.
//
// Returns:
//
//	*SSHClient - The initialized SSHClient.
func NewSSHClient() *SSHClient {
	return &SSHClient{
		clients: make(map[string]*ssh.Client),
	}
}

// RunCommand executes a command over SSH using the provided secrets and arguments.
// Returns the command output or an error.
//
// Parameters:
//
//	secrets - Map of authentication secrets.
//	args    - Command-line arguments to execute.
//
// Returns:
//
//	[]byte - Command output.
//	error  - Error if command fails or output indicates an error.
func (s *SSHClient) RunCommand(secrets map[string]string, args ...string) ([]byte, error) {
	conn, err := s.getSSHConnection(secrets)
	if err != nil {
		return nil, err
	}

	session, err := conn.NewSession()
	if err != nil {
		return nil, err
	}
	defer func() { _ = session.Close() }()

	cmd := strings.Join(args, " ")
	output, err := session.CombinedOutput(cmd)
	if err != nil {
		return nil, err
	}

	err = parseErrorString(string(output))
	if err != nil {
		return nil, err
	}

	return output, nil
}

// getSSHConnection establishes or retrieves a cached SSH connection using secrets.
// Returns an SSH client or error if authentication fails.
//
// Parameters:
//
//	secrets - Map of authentication secrets.
//
// Returns:
//
//	*ssh.Client - The SSH client connection.
//	error       - Error if connection fails.
func (s *SSHClient) getSSHConnection(secrets map[string]string) (*ssh.Client, error) {
	realm, ok := secrets[utils.RealmConnectionContext.RealmAddress]
	if !ok {
		return nil, fmt.Errorf("missing %s in secrets", utils.RealmConnectionContext.RealmAddress)
	}

	// acquire a lock to ensure thread safety when accessing the clients map
	s.Lock()
	defer s.Unlock()

	// check if there is a connection in the cache
	if client, exists := s.clients[realm]; exists {
		// check if connection is alive by sending a simple command
		if _, _, err := client.SendRequest("ping", false, nil); err == nil {
			// connection is alive and can be reused
			return client, nil
		}
		_ = client.Close()
		s.clients[realm] = nil // Remove dead connection from cache
	}

	// If no cached connection or the cached connection is dead, create a new one
	user, ok := secrets[utils.RealmConnectionContext.Username]
	if !ok {
		return nil, fmt.Errorf("missing user in secrets")
	}

	password, ok := secrets[utils.RealmConnectionContext.Password]
	if !ok {
		password = "" // Default to empty if not provided
	}

	privateKey, ok := secrets[utils.RealmConnectionContext.PrivateKey]
	if !ok {
		privateKey = "" // Default to empty if not provided
	}

	privateKeyPassphrase, ok := secrets[utils.RealmConnectionContext.PrivateKeyPassphrase]
	if !ok {
		privateKeyPassphrase = "" // Default to empty if not provided
	}

	if password == "" && privateKey == "" {
		// If neither password nor private key is provided, return an error.
		return nil, fmt.Errorf("no valid authentication method provided in secrets, either password or public key is required")
	}

	config := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         30 * time.Second, // Connection establishment timeout
	}

	// Add private key authentication if provided
	if privateKey != "" {
		var signer ssh.Signer
		var err error

		if privateKeyPassphrase == "" {
			signer, err = ssh.ParsePrivateKey([]byte(privateKey))
		} else {
			signer, err = ssh.ParsePrivateKeyWithPassphrase([]byte(privateKey), []byte(privateKeyPassphrase))
		}

		if err != nil {
			return nil, fmt.Errorf("failed to parse SSH private key: %v, check passphrase for the key", err)
		}

		config.Auth = append(config.Auth, ssh.PublicKeys(signer))
	}

	// Add password authentication if provided
	if password != "" {
		// Standard password authentication
		config.Auth = append(config.Auth, ssh.Password(password))

		// Keyboard-interactive for servers that require it
		config.Auth = append(config.Auth, ssh.KeyboardInteractive(
			func(user, instruction string, questions []string, echos []bool) (answers []string, err error) {
				for range questions {
					answers = append(answers, password)
				}
				return answers, nil
			},
		))
	}

	client, err := ssh.Dial("tcp", realm+":22", config)
	if err == nil {
		s.clients[realm] = client // Put new connection into the cache
	}
	return client, err
}

// PancliSSHClient implements the PancliClient interface for SSH-based communication with the PanFS realm.
type PancliSSHClient struct {
	pancli SSHRunner
}

var llog klog.Logger = klog.NewKlogr()

// NewPancliSSHClient creates a new instance of PancliSSHClient with the provided SSHRunner.
//
// Parameters:
//
//	runner - The SSHRunner implementation.
//
// Returns:
//
//	*PancliSSHClient - The initialized PancliSSHClient.
func NewPancliSSHClient(runner SSHRunner) *PancliSSHClient {
	return &PancliSSHClient{
		pancli: runner,
	}
}

// CreateVolume creates a volume using the provided arguments and returns the created volume object.
// Runs the volume creation command and retrieves the volume details.
//
// Parameters:
//
//	volumeName - The name of the volume to create.
//	params     - The volume creation parameters.
//	secrets    - Map of authentication secrets.
//
// Returns:
//
//	*utils.Volume - The created volume object.
//	error         - Error if creation or retrieval fails.
func (p *PancliSSHClient) CreateVolume(volumeName string, params VolumeCreateParams, secrets map[string]string) (*utils.Volume, error) {
	cmd := []string{"volume", "create", volumeName}

	optionalParams := getOptionalParameters(params)
	if len(optionalParams) != 0 {
		cmd = append(cmd, optionalParams...)
	}

	llog.V(5).Info("CreateVolume executes:", "command", strings.Join(cmd, " "))
	if _, err := p.pancli.RunCommand(secrets, cmd...); err != nil {
		return nil, err
	}

	volume, err := p.GetVolume(volumeName, secrets)
	if err != nil {
		return nil, err
	}

	return volume, nil
}

// DeleteVolume deletes a volume by its ID and returns an error if the operation fails.
//
// Parameters:
//
//	volumeName - The name of the volume to delete.
//	secrets    - Map of authentication secrets.
//
// Returns:
//
//	error - Error if deletion fails.
func (p *PancliSSHClient) DeleteVolume(volumeName string, secrets map[string]string) error {
	llog.V(5).Info("DeleteVolume executes:", "command", strings.Join([]string{"volume", "delete", "-f", volumeName}, " "))
	_, err := p.pancli.RunCommand(secrets, "volume", "delete", "-f", volumeName)
	return err
}

// ExpandVolume expands the size of a volume to the specified size in bytes.
// Runs the volume set soft-quota command.
//
// Parameters:
//
//	volumeName - The name of the volume to expand.
//	sizeBytes  - The target size in bytes.
//	secrets    - Map of authentication secrets.
//
// Returns:
//
//	error - Error if expansion fails.
func (p *PancliSSHClient) ExpandVolume(volumeName string, sizeBytes int64, secrets map[string]string) error {
	// convert size from bytes to gigabytes
	sizeGBStr := strconv.FormatFloat(utils.BytesToGB(sizeBytes), 'f', 2, 64)

	llog.V(5).Info("ExpandVolume executes:", "command", strings.Join([]string{"volume", "set", "soft-quota", volumeName, sizeGBStr}, " "))
	_, err := p.pancli.RunCommand(secrets, "volume", "set", "soft-quota", volumeName, sizeGBStr)
	if err != nil {
		return err
	}

	return nil
}

// ListVolumes retrieves a list of all volumes and returns them as a VolumeList object.
// Runs the pasxml volumes command and parses the output.
//
// Parameters:
//
//	secrets - Map of authentication secrets.
//
// Returns:
//
//	*utils.VolumeList - The parsed volume list.
//	error             - Error if retrieval or parsing fails.
func (p *PancliSSHClient) ListVolumes(secrets map[string]string) (*utils.VolumeList, error) {
	llog.V(5).Info("ListVolumes executes:", "command", strings.Join([]string{"pasxml", "volumes"}, " "))
	out, err := p.pancli.RunCommand(secrets, "pasxml", "volumes")
	if err != nil {
		return nil, err
	}

	vols, err := utils.ParseListVolumes(out)
	if err != nil {
		return nil, fmt.Errorf("ListVolumes: Cannot parse pancli response: %v", err)
	}

	if len(vols.SupportedUrls.Urls) > 0 {
		return nil, ErrorInvalidArgument
	}

	return vols, nil
}

// GetVolume retrieves a specific volume by its name and returns it as a Volume object.
// Runs the pasxml volumes volume command and parses the output.
//
// Parameters:
//
//	volumeName - The name of the volume to retrieve.
//	secrets    - Map of authentication secrets.
//
// Returns:
//
//	*utils.Volume - The parsed volume object.
//	error         - Error if retrieval or parsing fails.
func (p *PancliSSHClient) GetVolume(volumeName string, secrets map[string]string) (*utils.Volume, error) {
	llog.V(5).Info("GetVolume executes:", "command", strings.Join([]string{"pasxml", "volumes", "volume", volumeName}, " "))
	out, err := p.pancli.RunCommand(secrets, "pasxml", "volumes", "volume", volumeName)
	if err != nil {
		return nil, err
	}

	vols, err := utils.ParseListVolumes(out)
	if err != nil {
		return nil, fmt.Errorf("GetVolume: Cannot parse pancli response: %v", err)
	}

	if len(vols.SupportedUrls.Urls) > 0 {
		return nil, ErrorInvalidArgument
	}

	if len(vols.Volumes) < 1 {
		return nil, ErrorNotFound
	}

	return &vols.Volumes[0], nil
}
