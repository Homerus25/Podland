package hosts

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
	"github.com/Homerus25/podland/backend/internal/permission"
	"github.com/google/uuid"
)

// HostService provides business logic for hosts
type HostService struct {
	hostRepo   HostRepositoryInterface
	accessRepo models.HostAccessRepositoryInterface
	conn       connections.Manager
	encryption sshKeyEncryptor
	sshPool    sshInvalidator

	// hostPolicy is the opt-in egress block on the probe target
	// (hostpolicy.go). The zero value is disabled — no resolution, no
	// new failure modes — so a service without WithHostPolicy behaves
	// exactly as before.
	hostPolicy HostPolicy

	// fetchVersion asks a live connection context for the remote Podman's
	// version report (version.go). Production wires fetchRemoteVersion
	// (engine.go); tests swap the field — the connections.Manager dial
	// seam pattern.
	fetchVersion versionFetcher
}

// NewHostService creates a new HostService. sshPool is the SSH-exec
// pool behind the sshInvalidator seam; production passes the real
// sshexec.Service so host deletion can invalidate its cached clients.
func NewHostService(
	hostRepo HostRepositoryInterface,
	accessRepo models.HostAccessRepositoryInterface,
	conn connections.Manager,
	encryption sshKeyEncryptor,
	sshPool sshInvalidator,
) *HostService {
	return &HostService{
		hostRepo:     hostRepo,
		accessRepo:   accessRepo,
		conn:         conn,
		encryption:   encryption,
		sshPool:      sshPool,
		fetchVersion: fetchRemoteVersion,
	}
}

// CreateHostRequest represents a request to create a new host
type CreateHostRequest struct {
	Name             string `json:"name"`
	Description      string `json:"description"`
	Hostname         string `json:"hostname"`
	Port             int    `json:"port"`
	SSHUsername      string `json:"sshUsername"`
	SSHPrivateKey    string `json:"sshPrivateKey"`
	PodmanSocketPath string `json:"podmanSocketPath"`
}

// HostResponse represents a host in API responses. Permission is
// the caller's permission; it stays empty on the shapes this type is
// built for directly (Create, Update) and is populated by the enriched
// responses the list and detail endpoints answer (AccessibleHostResponse).
type HostResponse struct {
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

// FromModel converts a Host model to HostResponse
func FromModel(host *models.Host) HostResponse {
	return HostResponse{
		ID:                host.ID,
		Name:              host.Name,
		Description:       host.Description,
		Hostname:          host.Hostname,
		Port:              host.Port,
		SSHUsername:       host.SSHUsername,
		SSHKeyFingerprint: host.SSHKeyFingerprint,
		PodmanSocketPath:  host.PodmanSocketPath,
		OwnerID:           host.OwnerID,
		CreatedAt:         host.CreatedAt.Format(time.RFC3339),
		UpdatedAt:         host.UpdatedAt.Format(time.RFC3339),
	}
}

// AccessibleHostResponse is a host DTO carrying the per-caller
// enrichment: the host fields plus the caller's permission and
// ownership. IsOwner is a plain bool, never omitted — a grantee must
// read an explicit false. Both enrichment endpoints answer it: the list
// endpoint one per accessible host, the detail endpoint for the
// requested host — so gating facts are always present on a host
// object the client holds.
type AccessibleHostResponse struct {
	HostResponse
	IsOwner bool `json:"isOwner"`
}

// FromAccessibleHost builds the enriched DTO for one accessible
// host: the host fields, the caller's permission, and the
// caller-owns-it fact computed for the requesting user.
func FromAccessibleHost(host models.AccessibleHost, callerID string) AccessibleHostResponse {
	response := AccessibleHostResponse{HostResponse: FromModel(host.Host)}
	response.Permission = host.Permission
	response.IsOwner = host.OwnerID == callerID
	return response
}

// ToHostListResponse builds the list endpoint's response from the
// hosts the service returns, computing every row's per-caller facts
// for the requesting user.
func ToHostListResponse(hosts []models.AccessibleHost, callerID string) []AccessibleHostResponse {
	responses := make([]AccessibleHostResponse, len(hosts))
	for i, host := range hosts {
		responses[i] = FromAccessibleHost(host, callerID)
	}
	return responses
}

// Create creates a new host after validating the request and testing
// that the target is reachable over SSH + podman (description_backend.md
// §Host Registration). The synchronous connection probe runs BEFORE
// the host is persisted, so a bad host / bad credentials / wrong
// socket path is rejected at registration (400 CONNECTION_FAILED) instead
// of silently saving an unreachable host that only fails at first use.
// Every 4xx answer names the failed step and the reason (issue 399):
// the message is enriched via WithMessage while the error codes stay
// stable — the frontend renders 4xx envelope messages verbatim.
func (s *HostService) Create(ctx context.Context, userID string, req *CreateHostRequest) (*models.Host, error) {
	// Validate required fields. The message lists the missing field
	// names so the user can tell which inputs are empty.
	var missing []string
	if req.Name == "" {
		missing = append(missing, "name")
	}
	if req.Hostname == "" {
		missing = append(missing, "hostname")
	}
	if req.SSHUsername == "" {
		missing = append(missing, "sshUsername")
	}
	if req.SSHPrivateKey == "" {
		missing = append(missing, "sshPrivateKey")
	}
	if len(missing) > 0 {
		return nil, errors.ErrMissingRequiredFields.WithMessage(
			fmt.Sprintf("Missing required fields: %s", strings.Join(missing, ", ")))
	}

	// Validate SSH private key format (basic PEM check). The message
	// says what valid input looks like: a PEM block's marker pair.
	if !IsValidPEM(req.SSHPrivateKey) {
		return nil, errors.ErrInvalidSSHKey.WithMessage(invalidSSHKeyMessage)
	}

	// Check name uniqueness for user
	nameExists, err := s.hostRepo.NameExistsForUser(userID, req.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to check host name uniqueness: %w", err)
	}
	if nameExists {
		return nil, errors.ErrHostAlreadyExists.WithMessage(
			fmt.Sprintf("Host with name %q already exists", req.Name))
	}

	// Apply defaults BEFORE the probe so the connection test dials the
	// real (defaulted) port and socket the host will ultimately use.
	if req.Port == 0 {
		req.Port = 22
	}
	if req.PodmanSocketPath == "" {
		req.PodmanSocketPath = "/run/podman/podman.sock"
	}

	// Build the host model before verification: verifyKey probes the
	// host's own (already defaulted) connection fields, so the probe
	// dials the real port and socket the host will use, then fills in
	// the encrypted key and fingerprint. Nothing is persisted until the
	// probe has passed.
	now := time.Now().UTC()
	host := &models.Host{
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

	// The opt-in host policy gates the probe target right before the
	// probe (pentest 2026-11-01 finding 1): with the block enabled, a
	// blocked host is rejected here — never dialed, never persisted.
	// Every earlier 400 (missing fields, PEM, duplicate name) keeps its
	// precedence.
	if err := s.hostPolicy.Check(ctx, host.Hostname); err != nil {
		return nil, err
	}

	// A registration always carries a key (validated above): the new key
	// must earn its place on the host row before anything is persisted.
	if err := s.verifyKey(ctx, host, req.SSHPrivateKey); err != nil {
		return nil, err
	}

	// Create host in database
	if err := s.hostRepo.Create(host); err != nil {
		return nil, fmt.Errorf("failed to create host: %w", err)
	}

	// Grant owner admin access to the host
	access := &models.HostAccess{
		HostID:     host.ID,
		UserID:     userID,
		Permission: models.PermissionAdmin,
		GrantedAt:  now,
	}
	if err := s.accessRepo.Create(access); err != nil {
		// Rollback host creation if access creation fails
		if delErr := s.hostRepo.Delete(host.ID); delErr != nil {
			return nil, fmt.Errorf("failed to create access and rollback host: %w (rollback: %v)", err, delErr)
		}
		return nil, fmt.Errorf("failed to grant owner access: %w", err)
	}

	return host, nil
}

// invalidSSHKeyMessage describes the PEM marker pair a private key must
// carry (issue 399): the 400 INVALID_SSH_KEY message says what valid input
// looks like instead of only rejecting.
const invalidSSHKeyMessage = `Invalid SSH private key format: expected a PEM block — a "-----BEGIN OPENSSH PRIVATE KEY-----" (or RSA/EC/DSA/ED25519) header line paired with its matching "-----END ... PRIVATE KEY-----" footer line`

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
// host. A failing access lookup surfaces as an error — callers map it
// to 500 INTERNAL_ERROR — never as "no access": a broken database must
// not silently answer a 403 authorization decision.
func (s *HostService) UserHasAccess(userID, hostID string) (bool, error) {
	return s.accessRepo.HasAccess(hostID, userID)
}

// GetHostByID retrieves a host by ID together with the caller's
// per-caller facts: the stored permission level rides the returned
// AccessibleHost so the detail endpoint enriches its response the
// same way the list does. Access is the level contract at read floor
// through its single owner (permission.RequireLevel): a failing lookup
// answers ErrInternalError (500 via the envelope), a level below read —
// an empty row included — is ErrAccessDenied, and the re-read keeps
// grant changes effective immediately. The level is then looked up once
// more for the response, so the value the host carries is the one
// the gate just passed.
func (s *HostService) GetHostByID(userID, hostID string) (*models.AccessibleHost, error) {
	host, err := s.hostRepo.GetByID(hostID)
	if err != nil {
		return nil, err
	}
	if host == nil {
		return nil, errors.ErrHostNotFound
	}

	if err := permission.RequireLevel(s.accessRepo, hostID, userID, models.PermissionRead); err != nil {
		return nil, err
	}
	level, err := s.accessRepo.GetPermission(hostID, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get host permission: %w", err)
	}

	return &models.AccessibleHost{Host: host, Permission: level}, nil
}

// Host status values returned by GetHostStatus. There is no third
// server-side state: any probe failure is simply "offline".
const (
	StatusOnline  = "online"
	StatusOffline = "offline"
)

// GetHostStatus performs a live probe of a host the user can access
// and reports whether it is currently reachable. Access rules match
// GetHostByID (404 for unknown host, 403 without access; read level
// suffices). Offline is a valid answer, not an error: any probe failure —
// dial failure, auth failure, missing socket, ping failure, or timeout —
// yields StatusOffline. The detailed failure reason is logged server-side
// (with the requestId when present) and never returned to the caller,
// because SSH-level errors describe the owner's infrastructure and
// hosts can be shared with read-only users.
func (s *HostService) GetHostStatus(ctx context.Context, userID, hostID string) (string, time.Time, error) {
	if _, err := s.GetHostByID(userID, hostID); err != nil {
		return "", time.Time{}, err
	}

	checkedAt := time.Now().UTC()
	if err := s.conn.ProbeHost(hostID, userID); err != nil {
		slog.ErrorContext(ctx, "host status probe failed",
			"error", err,
			"hostID", hostID,
			"userID", userID,
			"requestID", contextutils.GetRequestID(ctx))
		return StatusOffline, checkedAt, nil
	}
	return StatusOnline, checkedAt, nil
}

// ListAllHosts retrieves all hosts accessible to a user (owned + shared),
// each paired with the caller's permission. It is the list endpoint's one
// list method.
func (s *HostService) ListAllHosts(userID string) ([]models.AccessibleHost, error) {
	return s.hostRepo.ListAccessible(userID)
}

// Update applies a partial host update (empty request fields keep the
// stored value). Only the owner may update; any other caller — including
// read- or write-level grantees — is denied.
func (s *HostService) Update(ctx context.Context, userID, hostID string, req *CreateHostRequest) (*models.Host, error) {
	host, err := s.hostRepo.GetByID(hostID)
	if err != nil {
		return nil, fmt.Errorf("failed to get host: %w", err)
	}
	if host == nil {
		return nil, errors.ErrHostNotFound
	}

	// Update is owner-only: an access row alone (read or write) must not
	// allow rewriting connection details or credentials.
	if host.OwnerID != userID {
		return nil, errors.ErrAccessDenied
	}

	if req.SSHPrivateKey != "" && !IsValidPEM(req.SSHPrivateKey) {
		return nil, errors.ErrInvalidSSHKey.WithMessage(invalidSSHKeyMessage)
	}

	if req.Name != "" {
		host.Name = req.Name
	}
	if req.Description != "" {
		host.Description = req.Description
	}
	if req.Hostname != "" {
		host.Hostname = req.Hostname
	}
	if req.Port != 0 {
		host.Port = req.Port
	}
	if req.SSHUsername != "" {
		host.SSHUsername = req.SSHUsername
	}
	if req.PodmanSocketPath != "" {
		host.PodmanSocketPath = req.PodmanSocketPath
	}

	// The opt-in host policy gates a hostname change whenever one is set
	// — with or without a key change — before the probe and before the
	// persist (pentest 2026-11-01 finding 1): a blocked host can never
	// be persisted. Updates that do not touch the hostname skip the
	// check entirely; the owner-only gate and the PEM validation above
	// keep their precedence.
	if req.Hostname != "" {
		if err := s.hostPolicy.Check(ctx, host.Hostname); err != nil {
			return nil, err
		}
	}

	// Probe parity with registration: a new SSH private key must earn its
	// place on the host before anything is persisted. verifyKey probes
	// the merged host — the request fields above already overrode the
	// stored ones — with the plaintext key, then encrypts and assigns in
	// place, all before the write, so a failed probe leaves the stored row
	// untouched. Updates without a key change skip the probe entirely.
	if req.SSHPrivateKey != "" {
		if err := s.verifyKey(ctx, host, req.SSHPrivateKey); err != nil {
			return nil, err
		}
	}
	host.UpdatedAt = time.Now().UTC()

	if err := s.hostRepo.Update(host); err != nil {
		return nil, fmt.Errorf("failed to update host: %w", err)
	}

	return host, nil
}

// verifyKey earns a plaintext SSH key its place on the host row. It
// probes the target over SSH + podman with the host's final (defaulted
// for a new registration, merged for an update) connection fields and the
// plaintext key; on a passed probe it encrypts the key and assigns
// ciphertext and fingerprint on the host. The probe runs against the
// plaintext key, before encryption, and callers persist only after this
// returns nil — so a failed probe leaves nothing behind. The probe is
// bounded by HOST_CONNECTION_TEST_TIMEOUT_SECONDS inside the connection
// manager: the manager's typed TestTimeoutError (carrying the configured
// bound) maps to CONNECTION_TEST_TIMEOUT naming that bound; the caller's
// own context deadline maps to the plain timeout sentinel (no configured
// bound to name); any other failure (bad host / credentials / socket) maps
// to CONNECTION_FAILED with a STATIC message. Both are 400 — the target
// the client described is not usable.
//
// The probe detail never reaches the client (pentest 2026-11-01 finding
// 1): the raw dial error is an internal port-scan / SSRF oracle from the
// server's network vantage point, and any authenticated user can
// register a host against a target that is not their own host. The
// failure is logged server-side with the requestID: the 400 mapping
// discards the underlying error, so the log line is where the full
// cause lives. The residual pure timing oracle (fast-fail vs timeout)
// is accepted and documented in the doku.
func (s *HostService) verifyKey(ctx context.Context, host *models.Host, plaintextKey string) error {
	if err := s.conn.TestConnection(ctx, host.Hostname, host.Port, host.SSHUsername, plaintextKey, host.PodmanSocketPath); err != nil {
		slog.ErrorContext(ctx, "host connection probe failed",
			"error", err,
			"hostname", host.Hostname,
			"port", host.Port,
			"hostName", host.Name,
			"requestID", contextutils.GetRequestID(ctx))

		var timeoutErr *connections.TestTimeoutError
		if stderrors.As(err, &timeoutErr) {
			return errors.ErrConnectionTestTimeout.WithMessage(
				fmt.Sprintf("Connection test timed out after %s", timeoutErr.Bound))
		}
		if stderrors.Is(err, context.DeadlineExceeded) {
			return errors.ErrConnectionTestTimeout
		}
		return errors.ErrConnectionTestFailed
	}

	encryptedKey, fingerprint, err := s.encryption.EncryptSSHKey(plaintextKey)
	if err != nil {
		return fmt.Errorf("failed to encrypt SSH key: %w", err)
	}
	host.SSHKeyEncrypted = encryptedKey
	host.SSHKeyFingerprint = fingerprint
	return nil
}

// GrantAccessRequest represents a request to grant access to a host
type GrantAccessRequest struct {
	UserID     string `json:"userId"`
	Permission string `json:"permission"`
}

// GrantAccess grants a user access to a host. Only the owner may
// grant; the permission must be read, write, or admin; an existing
// access row for the target user is a client error. DB failures from
// the lookups and the write propagate to the envelope's 500 fallback.
func (s *HostService) GrantAccess(grantorID, hostID string, req *GrantAccessRequest) (*models.HostAccess, error) {
	if req.Permission != models.PermissionRead && req.Permission != models.PermissionWrite && req.Permission != models.PermissionAdmin {
		return nil, errors.NewAppError("INVALID_PERMISSION", "Permission must be read, write, or admin", http.StatusBadRequest)
	}

	host, err := s.hostRepo.GetByID(hostID)
	if err != nil {
		return nil, err
	}
	if host == nil {
		return nil, errors.ErrHostNotFound
	}

	if host.OwnerID != grantorID {
		return nil, errors.ErrAccessDenied
	}

	existingAccess, err := s.accessRepo.GetByHostAndUser(hostID, req.UserID)
	if err != nil {
		return nil, err
	}
	if existingAccess != nil {
		return nil, errors.ErrAccessAlreadyGranted
	}

	access := &models.HostAccess{
		HostID:     hostID,
		UserID:     req.UserID,
		Permission: req.Permission,
		GrantedAt:  time.Now().UTC(),
	}

	if err := s.accessRepo.Create(access); err != nil {
		return nil, err
	}

	return access, nil
}

// RevokeAccess revokes a user's access to a host. Only the owner may
// revoke, and the owner's own access cannot be revoked. DB failures
// propagate to the envelope's 500 fallback.
func (s *HostService) RevokeAccess(revokerID, hostID, targetUserID string) error {
	host, err := s.hostRepo.GetByID(hostID)
	if err != nil {
		return err
	}
	if host == nil {
		return errors.ErrHostNotFound
	}

	if host.OwnerID != revokerID {
		return errors.ErrAccessDenied
	}
	if targetUserID == revokerID {
		return errors.ErrCannotRevokeOwner
	}

	return s.accessRepo.Delete(hostID, targetUserID)
}

// ListAccess lists a host's access rows after re-checking the
// caller's own access (read level suffices). DB failures from the
// re-check and the listing propagate to the envelope's 500 fallback.
func (s *HostService) ListAccess(callerID, hostID string) ([]*models.HostAccess, error) {
	hasAccess, err := s.UserHasAccess(callerID, hostID)
	if err != nil {
		return nil, err
	}
	if !hasAccess {
		return nil, errors.ErrAccessDenied
	}

	return s.accessRepo.ListByHost(hostID)
}

// Delete deletes a host by ID after verifying ownership, then tears
// the host out of the runtime caches in order: the connection cache's
// RemoveHost first (graceful: in-flight calls finish, and an
// acquisition between the row delete and the eviction fails the host
// lookup with 502 CONNECTION_FAILED — the reverse race is the cache's
// removed-during-acquisition error), then the SSH-exec pool's
// invalidation. Teardown runs only after a successful DB delete, so a
// failed delete never evicts a live host's connection. Access rows are
// not cleaned explicitly: host_access cascades on host deletion
// (ON DELETE CASCADE, foreign keys on).
func (s *HostService) Delete(ctx context.Context, userID, hostID string) error {
	// Get host to verify it exists and user is owner
	host, err := s.hostRepo.GetByID(hostID)
	if err != nil {
		return fmt.Errorf("failed to get host: %w", err)
	}
	if host == nil {
		return errors.ErrHostNotFound
	}

	// Verify user is the owner
	if host.OwnerID != userID {
		return errors.ErrAccessDenied
	}

	// Delete the host from database
	if err := s.hostRepo.Delete(hostID); err != nil {
		return fmt.Errorf("failed to delete host: %w", err)
	}

	// The row is gone: nothing may serve a fresh connection for this
	// host, and nothing pooled may outlive it.
	s.conn.RemoveHost(hostID)
	s.sshPool.RemoveHost(hostID)

	return nil
}
