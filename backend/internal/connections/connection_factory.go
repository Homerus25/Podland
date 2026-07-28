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

	// Write SSH key to a temporary file
	file, err := os.CreateTemp("", "ssh_key_*")
	if err != nil {
		return nil, err
	}
	defer os.Remove(file.Name()) // Clean up the file when done

	_, err = file.WriteString(sshKey)
	if err != nil {
		return nil, err
	}

	// Close the file to ensure all data is written
	if err := file.Close(); err != nil {
		return nil, err
	}

	// Create new Podman connection context
	connCtx, err := bindings.NewConnectionWithOptions(ctx, bindings.Options{
		URI:      fmt.Sprintf("ssh://%s@%s:%d%s", machine.SSHUsername, machine.Hostname, machine.Port, machine.PodmanSocketPath),
		Identity: file.Name(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Podman connection context: %w", err)
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

	// Write SSH key to a temporary file
	file, err := os.CreateTemp("", "ssh_key_*")
	if err != nil {
		return nil, err
	}
	defer os.Remove(file.Name()) // Clean up the file when done

	_, err = file.WriteString(sshKey)
	if err != nil {
		return nil, err
	}

	// Close the file to ensure all data is written
	if err := file.Close(); err != nil {
		return nil, err
	}

	// Create new Podman connection context
	connCtx, err := bindings.NewConnectionWithOptions(ctx, bindings.Options{
		URI:      fmt.Sprintf("ssh://%s@%s:%d%s", machine.SSHUsername, machine.Hostname, machine.Port, machine.PodmanSocketPath),
		Identity: file.Name(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Podman connection context: %w", err)
	}

	return connCtx, nil
}