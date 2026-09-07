package connections

import (
	"context"
	"fmt"
	"os"

	"go.podman.io/podman/v6/pkg/bindings"
)

// dialForMachine is the production dial seam: repo lookup → key decrypt →
// podman-over-SSH dial with a close handle. It is wired in
// NewConnectionManager and replaced by a fake in tests.
func (m *ConnectionManager) dialForMachine(ctx context.Context, machineID string) (context.Context, bindings.CloseFunc, error) {
	machine, err := m.machineRepo.GetByID(machineID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get machine details: %w", err)
	}
	if machine == nil {
		return nil, nil, fmt.Errorf("machine not found")
	}

	sshKey, err := m.encryption.Decrypt(machine.SSHKeyEncrypted)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decrypt SSH key: %w", err)
	}

	return dialCredentials(ctx, machine.Hostname, machine.Port, machine.SSHUsername, sshKey, machine.PodmanSocketPath)
}

// dialCredentials dials a podman socket over SSH using raw credentials and
// returns the live connection context plus the teardown handle. Shared by
// the runtime dial seam (after decrypting the stored key) and the
// registration probe (TestConnection, plaintext key) so the probe
// exercises the identical dial path as production.
//
// writeKeyTempFile + bindings.NewConnectionWithHandle is NOT lazy: the
// bindings call ssh.Dial (TCP + SSH auth) and then ping the podman socket
// (GET /_ping), so a single call surfaces all three registration failure
// modes — bad host, bad credentials, and wrong socket path.
func dialCredentials(
	ctx context.Context,
	hostname string,
	port int,
	sshUsername string,
	rawKey string,
	podmanSocketPath string,
) (context.Context, bindings.CloseFunc, error) {
	keyPath, err := writeKeyTempFile(rawKey)
	if err != nil {
		return nil, nil, err
	}
	defer os.Remove(keyPath)

	connCtx, closeFn, err := bindings.NewConnectionWithHandle(ctx, bindings.Options{
		URI:      fmt.Sprintf("ssh://%s@%s:%d%s", sshUsername, hostname, port, podmanSocketPath),
		Identity: keyPath,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create Podman connection context: %w", err)
	}
	return connCtx, closeFn, nil
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
