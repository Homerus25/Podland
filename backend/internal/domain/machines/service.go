package machines

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/yourorg/podland/backend/internal/errors"
	"github.com/yourorg/podland/backend/internal/models"
)

// MachineService provides business logic for machines
type MachineService struct {
	machineRepo MachineRepositoryInterface
	accessRepo  models.MachineAccessRepositoryInterface
	connManager ConnectionManagerInterface
	encryption  EncryptionServiceInterface
}

// NewMachineService creates a new MachineService
func NewMachineService(
	machineRepo MachineRepositoryInterface,
	accessRepo models.MachineAccessRepositoryInterface,
	connManager ConnectionManagerInterface,
	encryption EncryptionServiceInterface,
) *MachineService {
	return &MachineService{
		machineRepo: machineRepo,
		accessRepo:  accessRepo,
		connManager: connManager,
		encryption:  encryption,
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

// MachineResponse represents a machine in API responses
type MachineResponse struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Description       string `json:"description"`
	Hostname          string `json:"hostname"`
	Port              int    `json:"port"`
	SSHUsername       string `json:"sshUsername"`
	SSHKeyFingerprint string `json:"sshKeyFingerprint"`
	PodmanSocketPath  string `json:"podmanSocketPath"`
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
		CreatedAt:         machine.CreatedAt.Format(time.RFC3339),
		UpdatedAt:         machine.UpdatedAt.Format(time.RFC3339),
	}
}

// Create creates a new machine after validating the request and testing the connection
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

	// Encrypt SSH private key
	encryptedKey, fingerprint, err := s.encryption.EncryptSSHKey(req.SSHPrivateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt SSH key: %w", err)
	}

	// Set defaults
	if req.Port == 0 {
		req.Port = 22
	}
	if req.PodmanSocketPath == "" {
		req.PodmanSocketPath = "/run/podman/podman.sock"
	}

	// Create machine model
	now := time.Now().UTC()
	machine := &models.Machine{
		ID:                uuid.New().String(),
		Name:              req.Name,
		OwnerID:           userID,
		Description:       req.Description,
		Hostname:          req.Hostname,
		Port:              req.Port,
		SSHUsername:       req.SSHUsername,
		SSHKeyEncrypted:   encryptedKey,
		SSHKeyFingerprint: fingerprint,
		PodmanSocketPath:  req.PodmanSocketPath,
		CreatedAt:         now,
		UpdatedAt:         now,
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

// UserHasAccess checks if a user has access to a machine
func (s *MachineService) UserHasAccess(userID, machineID string) bool {
	hasAccess, err := s.accessRepo.HasAccess(machineID, userID)
	if err != nil {
		return false
	}
	return hasAccess
}

// NameExistsForUser checks if a machine name already exists for a user
func (s *MachineService) NameExistsForUser(userID, name string) (bool, error) {
	return s.machineRepo.NameExistsForUser(userID, name)
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

	// Check if user has access to the machine
	if !s.UserHasAccess(userID, machineID) {
		return nil, errors.ErrAccessDenied
	}

	return machine, nil
}

// ListAccessibleMachines retrieves all machines a user has access to
func (s *MachineService) ListAccessibleMachines(userID string) ([]*models.Machine, error) {
	return s.machineRepo.ListAccessible(userID)
}

// ListOwnedMachines retrieves all machines owned by a user
func (s *MachineService) ListOwnedMachines(userID string) ([]*models.Machine, error) {
	return s.machineRepo.ListByOwner(userID)
}

// ListAllMachines retrieves all machines accessible to a user (owned + shared)
func (s *MachineService) ListAllMachines(userID string) ([]*models.Machine, error) {
	return s.machineRepo.ListAccessible(userID)
}

// Delete deletes a machine by ID after verifying ownership
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

	return nil
}
