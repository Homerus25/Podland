package hosts

import (
	"github.com/Homerus25/podland/backend/internal/models"
)

// HostRepository handles database operations for hosts
type HostRepositoryInterface interface {
	Create(host *models.Host) error
	GetByID(id string) (*models.Host, error)
	Update(host *models.Host) error
	Delete(id string) error
	ListAccessible(userID string) ([]models.AccessibleHost, error)
	NameExistsForUser(userID, name string) (bool, error)
}

// The production persistence implementation satisfies the seam by
// construction, not by the silence of app.go's wiring.
var _ HostRepositoryInterface = (*models.HostRepository)(nil)

// sshKeyEncryptor is the hosts module's narrow consumer-side seam onto
// the encryption service: host registration must turn the request's
// plaintext SSH key into ciphertext plus fingerprint, but none of the
// service's other capability (general-purpose encrypt/decrypt, SSH-key
// decryption) is this module's business. The real encryption.Service
// satisfies it (wired in internal/app); connections and sshexec consume
// the full service directly, not this seam.
type sshKeyEncryptor interface {
	EncryptSSHKey(privateKey string) ([]byte, string, error)
}

// sshInvalidator is the hosts module's narrow consumer-side seam onto
// the SSH-exec pool: host deletion must tear down the host's pooled
// SSH client the same way it tears down the cached podman connection, but
// none of the pool's other capability is this module's business. The real
// sshexec.Service satisfies it with its public RemoveHost (wired in
// internal/app); the shared sshexec.SSHExecInterface is deliberately not
// widened.
type sshInvalidator interface {
	RemoveHost(hostID string)
}

// The connection seam is connections.Manager (internal/connections): every
// domain, hosts included, depends on that interface. SSH-exec capability
// lives in internal/sshexec, not here: the existing hosts tests already
// import internal/testutils, and MockSSHExec (which lives in testutils)
// needs to reference the ExecOptions / ExecResult types — so those types
// must live in a package that testutils can import without
// testutils→hosts→testutils looping. Consumers (e.g. the quadlet
// lifecycle service in 070-06) depend on sshexec.SSHExecInterface directly.
