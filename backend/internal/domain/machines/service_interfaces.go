package machines

import (
	"context"
)

// ConnectionManagerInterface defines the interface for connection management
// This allows mocking the ConnectionManager in tests
type ConnectionManagerInterface interface {
	GetConnection(machineID, userID string) (context.Context, error)
	Release(machineID string)
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
