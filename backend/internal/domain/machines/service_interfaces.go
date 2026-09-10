package machines

// EncryptionServiceInterface defines the interface for encryption services
// This allows mocking the EncryptionService in tests
type EncryptionServiceInterface interface {
	Encrypt(data string) ([]byte, error)
	Decrypt(data []byte) (string, error)
	EncryptSSHKey(privateKey string) ([]byte, string, error)
	DecryptSSHKey(encryptedKey []byte) (string, error)
}

// sshInvalidator is the machines module's narrow consumer-side seam onto
// the SSH-exec pool: machine deletion must tear down the machine's pooled
// SSH client the same way it tears down the cached podman connection, but
// none of the pool's other capability is this module's business. The real
// sshexec.Service satisfies it with its public RemoveMachine (wired in
// internal/app); the shared sshexec.SSHExecInterface is deliberately not
// widened.
type sshInvalidator interface {
	RemoveMachine(machineID string)
}

// The connection seam is connections.Manager (internal/connections): every
// domain, machines included, depends on that interface. SSH-exec capability
// lives in internal/sshexec, not here: the existing machines tests already
// import internal/testutils, and MockSSHExec (which lives in testutils)
// needs to reference the ExecOptions / ExecResult types — so those types
// must live in a package that testutils can import without
// testutils→machines→testutils looping. Consumers (e.g. the quadlet
// lifecycle service in 070-06) depend on sshexec.SSHExecInterface directly.
