package machines

import (
	"context"
	stderrors "errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/Homerus25/podland/backend/internal/connections"
	"github.com/Homerus25/podland/backend/internal/contextutils"
	"github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/google/uuid"
)

// MachineService provides business logic for machines
type MachineService struct {
	machineRepo MachineRepositoryInterface
	accessRepo  models.MachineAccessRepositoryInterface
	conn        connections.Manager
	encryption  sshKeyEncryptor
	sshPool     sshInvalidator
}

// NewMachineService creates a new MachineService. sshPool is the SSH-exec
// pool behind the sshInvalidator seam; production passes the real
// sshexec.Service so machine deletion can invalidate its cached clients.
func NewMachineService(
	machineRepo MachineRepositoryInterface,
	accessRepo models.MachineAccessRepositoryInterface,
	conn connections.Manager,
	encryption sshKeyEncryptor,
	sshPool sshInvalidator,
) *MachineService {
	return &MachineService{
		machineRepo: machineRepo,
		accessRepo:  accessRepo,
		conn:        conn,
		encryption:  encryption,
		sshPool:     sshPool,
	}
}

// CreateMachineRequest represents a request to create a new machine
type CreateMachineRequest struct {
	Name             string `json:"name"`
	Description      string `json:"description"`
	Hostname         string `json:"hostname"`
	Port             int    `json:"port"`
	SSHUsername      string `json:"sshUsername"`
	SSHPrivateKey    string `json:"sshPrivateKey"`
	PodmanSocketPath string `json:"podmanSocketPath"`
}

// MachineResponse represents a machine in API responses. Permission is
// the caller's permission and is only populated by the list endpoint.
type MachineResponse struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Description       string `json:"description"`
	Hostname          string `json:"hostname"`
	Port              int    `json:"port"`
	SSHUsername       string `json:"sshUsername"`
	SSHKeyFingerprint string `json:"sshKeyFingerprint"`
	PodmanSocketPath  string `json:"podmanSocketPath"`
	OwnerID           string `json:"ownerId"`
	Permission        string `json:"permission,omitempty"`
	CreatedAt         string `json:"createdAt"`
	UpdatedAt         string `json:"updatedAt"`
}

// FromModel converts a Machine model to MachineResponse
func FromModel(machine *models.Machine) MachineResponse {
	return MachineResponse{
		ID:                machine.ID,
		Name:              machine.Name,
		Description:       machine.Description,
		Hostname:          machine.Hostname,
		Port:              machine.Port,
		SSHUsername:       machine.SSHUsername,
		SSHKeyFingerprint: machine.SSHKeyFingerprint,
		PodmanSocketPath:  machine.PodmanSocketPath,
		OwnerID:           machine.OwnerID,
		CreatedAt:         machine.CreatedAt.Format(time.RFC3339),
		UpdatedAt:         machine.UpdatedAt.Format(time.RFC3339),
	}
}

// MachineListResponse is one element of the list endpoint's response:
// the machine fields plus the per-caller enrichment (the caller's
// permission and ownership). IsOwner is a plain bool, never omitted —
// a grantee must read an explicit false. The single-machine endpoints
// answer MachineResponse and stay untouched.
type MachineListResponse struct {
	MachineResponse
	IsOwner bool `json:"isOwner"`
}

// FromAccessibleMachine builds the list endpoint's element for one
// accessible machine: the machine fields, the caller's permission, and
// the caller-owns-it fact computed for the requesting user.
func FromAccessibleMachine(machine models.AccessibleMachine, callerID string) MachineListResponse {
	response := MachineListResponse{MachineResponse: FromModel(machine.Machine)}
	response.Permission = machine.Permission
	response.IsOwner = machine.OwnerID == callerID
	return response
}

// ToMachineListResponse builds the list endpoint's response from the
// machines the service returns, computing every row's per-caller facts
// for the requesting user.
func ToMachineListResponse(machines []models.AccessibleMachine, callerID string) []MachineListResponse {
	responses := make([]MachineListResponse, len(machines))
	for i, machine := range machines {
		responses[i] = FromAccessibleMachine(machine, callerID)
	}
	return responses
}

// Create creates a new machine after validating the request and testing
// that the target is reachable over SSH + podman (description_backend.md
// §Machine Registration). The synchronous connection probe runs BEFORE
// the machine is persisted, so a bad host / bad credentials / wrong
// socket path is rejected at registration (400 CONNECTION_FAILED) instead
// of silently saving an unreachable machine that only fails at first use.
func (s *MachineService) Create(ctx context.Context, userID string, req *CreateMachineRequest) (*models.Machine, error) {
	// Validate required fields
	if req.Name == "" || req.Hostname == "" || req.SSHUsername == "" || req.SSHPrivateKey == "" {
		return nil, errors.ErrMissingRequiredFields
	}

	// Validate SSH private key format (basic PEM check)
	if !IsValidPEM(req.SSHPrivateKey) {
		return nil, errors.ErrInvalidSSHKey
	}

	// Check name uniqueness for user
	nameExists, err := s.machineRepo.NameExistsForUser(userID, req.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to check machine name uniqueness: %w", err)
	}
	if nameExists {
		return nil, errors.ErrMachineAlreadyExists
	}

	// Apply defaults BEFORE the probe so the connection test dials the
	// real (defaulted) port and socket the machine will ultimately use.
	if req.Port == 0 {
		req.Port = 22
	}
	if req.PodmanSocketPath == "" {
		req.PodmanSocketPath = "/run/podman/podman.sock"
	}

	// Build the machine model before verification: verifyKey probes the
	// machine's own (already defaulted) connection fields, so the probe
	// dials the real port and socket the machine will use, then fills in
	// the encrypted key and fingerprint. Nothing is persisted until the
	// probe has passed.
	now := time.Now().UTC()
	machine := &models.Machine{
		ID:               uuid.New().String(),
		Name:             req.Name,
		OwnerID:          userID,
		Description:      req.Description,
		Hostname:         req.Hostname,
		Port:             req.Port,
		SSHUsername:      req.SSHUsername,
		PodmanSocketPath: req.PodmanSocketPath,
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	// A registration always carries a key (validated above): the new key
	// must earn its place on the machine row before anything is persisted.
	if err := s.verifyKey(ctx, machine, req.SSHPrivateKey); err != nil {
		return nil, err
	}

	// Create machine in database
	if err := s.machineRepo.Create(machine); err != nil {
		return nil, fmt.Errorf("failed to create machine: %w", err)
	}

	// Grant owner admin access to the machine
	access := &models.MachineAccess{
		MachineID:  machine.ID,
		UserID:     userID,
		Permission: models.PermissionAdmin,
		GrantedAt:  now,
	}
	if err := s.accessRepo.Create(access); err != nil {
		// Rollback machine creation if access creation fails
		if delErr := s.machineRepo.Delete(machine.ID); delErr != nil {
			return nil, fmt.Errorf("failed to create access and rollback machine: %w (rollback: %v)", err, delErr)
		}
		return nil, fmt.Errorf("failed to grant owner access: %w", err)
	}

	return machine, nil
}

// IsValidPEM performs a basic check for PEM format
func IsValidPEM(key string) bool {
	// Basic PEM validation - should start with a valid BEGIN marker and contain END
	key = strings.TrimSpace(key)
	if len(key) < 20 {
		return false
	}

	// Check for PEM BEGIN headers
	sshKeyTypes := []string{
		"-----BEGIN RSA PRIVATE KEY-----",
		"-----BEGIN OPENSSH PRIVATE KEY-----",
		"-----BEGIN DSA PRIVATE KEY-----",
		"-----BEGIN EC PRIVATE KEY-----",
		"-----BEGIN ED25519 PRIVATE KEY-----",
	}

	for _, keyType := range sshKeyTypes {
		if strings.HasPrefix(key, keyType) {
			// Check if it contains a corresponding END marker
			endMarker := strings.Replace(keyType, "BEGIN", "END", 1)
			if strings.Contains(key, endMarker) {
				return true
			}
		}
	}

	return false
}

// UserHasAccess reports whether the user has an access row on the
// machine. A failing access lookup surfaces as an error — callers map it
// to 500 INTERNAL_ERROR — never as "no access": a broken database must
// not silently answer a 403 authorization decision.
func (s *MachineService) UserHasAccess(userID, machineID string) (bool, error) {
	return s.accessRepo.HasAccess(machineID, userID)
}

// GetMachineByID retrieves a machine by ID if the user has access
func (s *MachineService) GetMachineByID(userID, machineID string) (*models.Machine, error) {
	machine, err := s.machineRepo.GetByID(machineID)
	if err != nil {
		return nil, err
	}
	if machine == nil {
		return nil, errors.ErrMachineNotFound
	}

	// Check if user has access to the machine. A failing lookup is a
	// storage error (500), not a denial.
	hasAccess, err := s.UserHasAccess(userID, machineID)
	if err != nil {
		return nil, fmt.Errorf("failed to check machine access: %w", err)
	}
	if !hasAccess {
		return nil, errors.ErrAccessDenied
	}

	return machine, nil
}

// Machine status values returned by GetMachineStatus. There is no third
// server-side state: any probe failure is simply "offline".
const (
	StatusOnline  = "online"
	StatusOffline = "offline"
)

// GetMachineStatus performs a live probe of a machine the user can access
// and reports whether it is currently reachable. Access rules match
// GetMachineByID (404 for unknown machine, 403 without access; read level
// suffices). Offline is a valid answer, not an error: any probe failure —
// dial failure, auth failure, missing socket, ping failure, or timeout —
// yields StatusOffline. The detailed failure reason is logged server-side
// (with the requestId when present) and never returned to the caller,
// because SSH-level errors describe the owner's infrastructure and
// machines can be shared with read-only users.
func (s *MachineService) GetMachineStatus(ctx context.Context, userID, machineID string) (string, time.Time, error) {
	if _, err := s.GetMachineByID(userID, machineID); err != nil {
		return "", time.Time{}, err
	}

	checkedAt := time.Now().UTC()
	if err := s.conn.ProbeMachine(machineID, userID); err != nil {
		slog.ErrorContext(ctx, "machine status probe failed",
			"error", err,
			"machineID", machineID,
			"userID", userID,
			"requestID", contextutils.GetRequestID(ctx))
		return StatusOffline, checkedAt, nil
	}
	return StatusOnline, checkedAt, nil
}

// ListAllMachines retrieves all machines accessible to a user (owned + shared),
// each paired with the caller's permission. It is the list endpoint's one
// list method.
func (s *MachineService) ListAllMachines(userID string) ([]models.AccessibleMachine, error) {
	return s.machineRepo.ListAccessible(userID)
}

// Update applies a partial machine update (empty request fields keep the
// stored value). Only the owner may update; any other caller — including
// read- or write-level grantees — is denied.
func (s *MachineService) Update(ctx context.Context, userID, machineID string, req *CreateMachineRequest) (*models.Machine, error) {
	machine, err := s.machineRepo.GetByID(machineID)
	if err != nil {
		return nil, fmt.Errorf("failed to get machine: %w", err)
	}
	if machine == nil {
		return nil, errors.ErrMachineNotFound
	}

	// Update is owner-only: an access row alone (read or write) must not
	// allow rewriting connection details or credentials.
	if machine.OwnerID != userID {
		return nil, errors.ErrAccessDenied
	}

	if req.SSHPrivateKey != "" && !IsValidPEM(req.SSHPrivateKey) {
		return nil, errors.ErrInvalidSSHKey
	}

	if req.Name != "" {
		machine.Name = req.Name
	}
	if req.Description != "" {
		machine.Description = req.Description
	}
	if req.Hostname != "" {
		machine.Hostname = req.Hostname
	}
	if req.Port != 0 {
		machine.Port = req.Port
	}
	if req.SSHUsername != "" {
		machine.SSHUsername = req.SSHUsername
	}
	if req.PodmanSocketPath != "" {
		machine.PodmanSocketPath = req.PodmanSocketPath
	}

	// Probe parity with registration: a new SSH private key must earn its
	// place on the machine before anything is persisted. verifyKey probes
	// the merged machine — the request fields above already overrode the
	// stored ones — with the plaintext key, then encrypts and assigns in
	// place, all before the write, so a failed probe leaves the stored row
	// untouched. Updates without a key change skip the probe entirely.
	if req.SSHPrivateKey != "" {
		if err := s.verifyKey(ctx, machine, req.SSHPrivateKey); err != nil {
			return nil, err
		}
	}
	machine.UpdatedAt = time.Now().UTC()

	if err := s.machineRepo.Update(machine); err != nil {
		return nil, fmt.Errorf("failed to update machine: %w", err)
	}

	return machine, nil
}

// verifyKey earns a plaintext SSH key its place on the machine row. It
// probes the target over SSH + podman with the machine's final (defaulted
// for a new registration, merged for an update) connection fields and the
// plaintext key; on a passed probe it encrypts the key and assigns
// ciphertext and fingerprint on the machine. The probe runs against the
// plaintext key, before encryption, and callers persist only after this
// returns nil — so a failed probe leaves nothing behind. The probe is
// bounded by MACHINE_CONNECTION_TEST_TIMEOUT_SECONDS inside the connection
// manager: a deadline (the probe's own timeout, or the caller's context
// expiring) maps to CONNECTION_TEST_TIMEOUT; any other failure (bad host /
// credentials / socket) maps to CONNECTION_FAILED. Both are 400 — the
// target the client described is not usable.
func (s *MachineService) verifyKey(ctx context.Context, machine *models.Machine, plaintextKey string) error {
	if err := s.conn.TestConnection(ctx, machine.Hostname, machine.Port, machine.SSHUsername, plaintextKey, machine.PodmanSocketPath); err != nil {
		if stderrors.Is(err, context.DeadlineExceeded) {
			return errors.ErrConnectionTestTimeout
		}
		return errors.ErrConnectionTestFailed
	}

	encryptedKey, fingerprint, err := s.encryption.EncryptSSHKey(plaintextKey)
	if err != nil {
		return fmt.Errorf("failed to encrypt SSH key: %w", err)
	}
	machine.SSHKeyEncrypted = encryptedKey
	machine.SSHKeyFingerprint = fingerprint
	return nil
}

// GrantAccessRequest represents a request to grant access to a machine
type GrantAccessRequest struct {
	UserID     string `json:"userId"`
	Permission string `json:"permission"`
}

// GrantAccess grants a user access to a machine. Only the owner may
// grant; the permission must be read, write, or admin; an existing
// access row for the target user is a client error. DB failures from
// the lookups and the write propagate to the envelope's 500 fallback.
func (s *MachineService) GrantAccess(grantorID, machineID string, req *GrantAccessRequest) (*models.MachineAccess, error) {
	if req.Permission != models.PermissionRead && req.Permission != models.PermissionWrite && req.Permission != models.PermissionAdmin {
		return nil, errors.NewAppError("INVALID_PERMISSION", "Permission must be read, write, or admin", http.StatusBadRequest)
	}

	machine, err := s.machineRepo.GetByID(machineID)
	if err != nil {
		return nil, err
	}
	if machine == nil {
		return nil, errors.ErrMachineNotFound
	}

	if machine.OwnerID != grantorID {
		return nil, errors.ErrAccessDenied
	}

	existingAccess, err := s.accessRepo.GetByMachineAndUser(machineID, req.UserID)
	if err != nil {
		return nil, err
	}
	if existingAccess != nil {
		return nil, errors.ErrAccessAlreadyGranted
	}

	access := &models.MachineAccess{
		MachineID:  machineID,
		UserID:     req.UserID,
		Permission: req.Permission,
		GrantedAt:  time.Now().UTC(),
	}

	if err := s.accessRepo.Create(access); err != nil {
		return nil, err
	}

	return access, nil
}

// RevokeAccess revokes a user's access to a machine. Only the owner may
// revoke, and the owner's own access cannot be revoked. DB failures
// propagate to the envelope's 500 fallback.
func (s *MachineService) RevokeAccess(revokerID, machineID, targetUserID string) error {
	machine, err := s.machineRepo.GetByID(machineID)
	if err != nil {
		return err
	}
	if machine == nil {
		return errors.ErrMachineNotFound
	}

	if machine.OwnerID != revokerID {
		return errors.ErrAccessDenied
	}
	if targetUserID == revokerID {
		return errors.ErrCannotRevokeOwner
	}

	return s.accessRepo.Delete(machineID, targetUserID)
}

// ListAccess lists a machine's access rows after re-checking the
// caller's own access (read level suffices). DB failures from the
// re-check and the listing propagate to the envelope's 500 fallback.
func (s *MachineService) ListAccess(callerID, machineID string) ([]*models.MachineAccess, error) {
	hasAccess, err := s.UserHasAccess(callerID, machineID)
	if err != nil {
		return nil, err
	}
	if !hasAccess {
		return nil, errors.ErrAccessDenied
	}

	return s.accessRepo.ListByMachine(machineID)
}

// Delete deletes a machine by ID after verifying ownership, then tears
// the machine out of the runtime caches in order: the connection cache's
// RemoveMachine first (graceful: in-flight calls finish, and an
// acquisition between the row delete and the eviction fails the machine
// lookup with 502 CONNECTION_FAILED — the reverse race is the cache's
// removed-during-acquisition error), then the SSH-exec pool's
// invalidation. Teardown runs only after a successful DB delete, so a
// failed delete never evicts a live machine's connection. Access rows are
// not cleaned explicitly: machine_access cascades on machine deletion
// (ON DELETE CASCADE, foreign keys on).
func (s *MachineService) Delete(ctx context.Context, userID, machineID string) error {
	// Get machine to verify it exists and user is owner
	machine, err := s.machineRepo.GetByID(machineID)
	if err != nil {
		return fmt.Errorf("failed to get machine: %w", err)
	}
	if machine == nil {
		return errors.ErrMachineNotFound
	}

	// Verify user is the owner
	if machine.OwnerID != userID {
		return errors.ErrAccessDenied
	}

	// Delete the machine from database
	if err := s.machineRepo.Delete(machineID); err != nil {
		return fmt.Errorf("failed to delete machine: %w", err)
	}

	// The row is gone: nothing may serve a fresh connection for this
	// machine, and nothing pooled may outlive it.
	s.conn.RemoveMachine(machineID)
	s.sshPool.RemoveMachine(machineID)

	return nil
}
