package machines

// EncryptionServiceInterface defines the interface for encryption services
// This allows mocking the EncryptionService in tests
type EncryptionServiceInterface interface {
	Encrypt(data string) ([]byte, error)
	Decrypt(data []byte) (string, error)
	EncryptSSHKey(privateKey string) ([]byte, string, error)
	DecryptSSHKey(encryptedKey []byte) (string, error)
}

// The connection seam is connections.Manager (internal/connections): every
// domain, machines included, depends on that interface. SSH-exec capability
// lives in internal/sshexec, not here: the existing machines tests already
// import internal/testutils, and MockSSHExec (which lives in testutils)
// needs to reference the ExecOptions / ExecResult types — so those types
// must live in a package that testutils can import without
// testutils→machines→testutils looping. Consumers (e.g. the quadlet
// lifecycle service in 070-06) depend on sshexec.SSHExecInterface directly.
