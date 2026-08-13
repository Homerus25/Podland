package machines

import (
	"context"
)

// ConnectionManagerInterface defines the interface for connection management
// This allows mocking the ConnectionManager in tests
type ConnectionManagerInterface interface {
	GetConnection(machineID, userID string) (context.Context, error)
	Release(machineID string)
	// TestConnection probes an SSH/podman connection using raw credentials,
	// without requiring the machine to be persisted first. It is called from
	// MachineService.Create during registration (description_backend.md
	// §Machine Registration step 5): the machine is not yet in the database,
	// so GetConnection (which looks up by machineID) cannot be used. The
	// implementation is responsible for bounding the probe by the configured
	// MACHINE_CONNECTION_TEST_TIMEOUT_SECONDS.
	//
	// The credentials are passed as primitives rather than a struct so this
	// interface stays self-contained in the machines package (no dependency
	// on the connections package), matching the primitive-typed
	// GetConnection/Release above.
	TestConnection(
		ctx context.Context,
		hostname string,
		port int,
		sshUsername string,
		sshPrivateKey string,
		podmanSocketPath string,
	) error
}

// EncryptionServiceInterface defines the interface for encryption services
// This allows mocking the EncryptionService in tests
type EncryptionServiceInterface interface {
	Encrypt(data string) ([]byte, error)
	Decrypt(data []byte) (string, error)
	EncryptSSHKey(privateKey string) ([]byte, string, error)
	DecryptSSHKey(encryptedKey []byte) (string, error)
}

// SSH-exec capability lives in internal/sshexec, not here. Co-locating
// SSHExecInterface / ExecOptions / ExecResult with ConnectionManagerInterface
// in this package would create an import cycle: the existing machines tests
// already import internal/testutils, and MockSSHExec (which lives in
// testutils per the issue spec) needs to reference the ExecOptions /
// ExecResult types — so those types must live in a package that testutils
// can import without testutils→machines→testutils looping.
//
// Consumers (e.g. the quadlet lifecycle service in 070-06) depend on
// sshexec.SSHExecInterface directly. The "separate from
// ConnectionManagerInterface" property called out in the issue is preserved:
// images/containers/quadlets services that depend on
// machines.ConnectionManagerInterface are completely unaffected.
