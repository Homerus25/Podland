package machines

import (
	"github.com/Homerus25/podland/backend/internal/models"
)

// MachineRepository handles database operations for machines
type MachineRepositoryInterface interface {
	Create(machine *models.Machine) error
	GetByID(id string) (*models.Machine, error)
	Update(machine *models.Machine) error
	Delete(id string) error
	ListAccessible(userID string) ([]models.AccessibleMachine, error)
	NameExistsForUser(userID, name string) (bool, error)
}

// The production persistence implementation satisfies the seam by
// construction, not by the silence of app.go's wiring.
var _ MachineRepositoryInterface = (*models.MachineRepository)(nil)

// sshKeyEncryptor is the machines module's narrow consumer-side seam onto
// the encryption service: machine registration must turn the request's
// plaintext SSH key into ciphertext plus fingerprint, but none of the
// service's other capability (general-purpose encrypt/decrypt, SSH-key
// decryption) is this module's business. The real encryption.Service
// satisfies it (wired in internal/app); connections and sshexec consume
// the full service directly, not this seam.
type sshKeyEncryptor interface {
	EncryptSSHKey(privateKey string) ([]byte, string, error)
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
