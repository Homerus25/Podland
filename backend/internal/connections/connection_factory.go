package connections

import (
	"context"
	"fmt"
	"os"

	"github.com/yourorg/podland/backend/internal/encryption"
	"github.com/yourorg/podland/backend/internal/models"
	"go.podman.io/podman/v6/pkg/bindings"
)

// ConnectionFactory creates new Podman connections to machines
type ConnectionFactory struct {
	machineRepo *models.MachineRepository
	encryption  *encryption.EncryptionService
}

// NewConnectionFactory creates a new connection factory
type ConnectionFactoryOptions struct {
	MachineRepo *models.MachineRepository
	Encryption  *encryption.EncryptionService
}

func NewConnectionFactory(opts ConnectionFactoryOptions) *ConnectionFactory {
	return &ConnectionFactory{
		machineRepo: opts.MachineRepo,
		encryption:  opts.Encryption,
	}
}

// CreateConnection creates a new Podman connection to a machine
func (f *ConnectionFactory) CreateConnection(
	ctx context.Context,
	machineID, userID string,
) (*bindings.Connection, error) {
	// Get machine details
	machine, err := f.machineRepo.GetByID(machineID)
	if err != nil {
		return nil, fmt.Errorf("failed to get machine details: %w", err)
	}
	if machine == nil {
		return nil, fmt.Errorf("machine not found")
	}

	// Decrypt SSH key
	sshKey, err := f.encryption.Decrypt(machine.SSHKeyEncrypted)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt SSH key: %w", err)
	}

	// Dial Podman over SSH using the decrypted key.
	connCtx, err := f.dialPodmanSSH(ctx, machine.SSHUsername, machine.Hostname, machine.Port, machine.PodmanSocketPath, sshKey)
	if err != nil {
		return nil, err
	}

	// Get the actual connection from the context
	conn, err := bindings.GetClient(connCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to get Podman client: %w", err)
	}

	return conn, nil
}

// CreateConnectionContext creates a new Podman connection context to a machine
func (f *ConnectionFactory) CreateConnectionContext(
	ctx context.Context,
	machineID, userID string,
) (context.Context, error) {
	// Get machine details
	machine, err := f.machineRepo.GetByID(machineID)
	if err != nil {
		return nil, fmt.Errorf("failed to get machine details: %w", err)
	}
	if machine == nil {
		return nil, fmt.Errorf("machine not found")
	}

	// Decrypt SSH key
	sshKey, err := f.encryption.Decrypt(machine.SSHKeyEncrypted)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt SSH key: %w", err)
	}

	return f.dialPodmanSSH(ctx, machine.SSHUsername, machine.Hostname, machine.Port, machine.PodmanSocketPath, sshKey)
}

// TestConnection probes reachability of a not-yet-persisted machine during
// registration (description_backend.md §Machine Registration step 5). It
// reuses dialPodmanSSH so the registration probe follows the identical
// dial path as a runtime connection — the only difference is that the key
// is still plaintext at this point (the documented flow runs the probe
// before encrypting). Nothing is cached: the probe only proves the
// target is reachable; the runtime builds its own cached connection on
// first use.
func (f *ConnectionFactory) TestConnection(
	ctx context.Context,
	hostname string,
	port int,
	sshUsername string,
	sshPrivateKey string,
	podmanSocketPath string,
) error {
	// The probe's connection context is discarded: the probe only proves
	// reachability. The runtime builds its own (cached) connection on
	// first use; the SSH client lifetime here is owned by the bindings
	// transport, torn down when the returned context is GC'd.
	_, err := f.dialPodmanSSH(ctx, sshUsername, hostname, port, podmanSocketPath, sshPrivateKey)
	return err
}

// dialPodmanSSH writes the plaintext SSH key to a temp file and opens an
// eager Podman-over-SSH connection context. bindings.NewConnectionWithOptions
// is NOT lazy: it calls ssh.Dial (TCP + SSH auth) and then pings the podman
// socket (GET /_ping), so a single call surfaces all three registration
// failure modes — bad host, bad credentials, and wrong socket path. Shared
// by the runtime path (CreateConnection / CreateConnectionContext, after
// decrypting the stored key) and the registration probe (TestConnection,
// plaintext key) so the probe exercises the same code as production.
func (f *ConnectionFactory) dialPodmanSSH(
	ctx context.Context,
	sshUsername, hostname string,
	port int,
	socketPath, rawKey string,
) (context.Context, error) {
	keyPath, err := writeKeyTempFile(rawKey)
	if err != nil {
		return nil, err
	}
	defer os.Remove(keyPath)

	connCtx, err := bindings.NewConnectionWithOptions(ctx, bindings.Options{
		URI:      fmt.Sprintf("ssh://%s@%s:%d%s", sshUsername, hostname, port, socketPath),
		Identity: keyPath,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Podman connection context: %w", err)
	}
	return connCtx, nil
}

// writeKeyTempFile writes the plaintext SSH key to a uniquely-named temp
// file and returns its path. The caller MUST os.Remove the path when done
// (the bindings dial reads the file during SSH auth). All error paths
// clean the file up so a partial write never leaves a stale key behind.
func writeKeyTempFile(rawKey string) (string, error) {
	file, err := os.CreateTemp("", "ssh_key_*")
	if err != nil {
		return "", err
	}
	name := file.Name()
	if _, err := file.WriteString(rawKey); err != nil {
		_ = os.Remove(name)
		return "", err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(name)
		return "", err
	}
	return name, nil
}
