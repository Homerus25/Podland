package machines

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/yourorg/podland/backend/internal/models"
)

// MachineRepository handles database operations for machines
type MachineRepository struct {
	db *sql.DB
}

// NewMachineRepository creates a new MachineRepository
type MachineRepositoryInterface interface {
	Create(machine *models.Machine) error
	GetByID(id string) (*models.Machine, error)
	Update(machine *models.Machine) error
	Delete(id string) error
	ListByOwner(ownerID string) ([]*models.Machine, error)
	ListAccessible(userID string) ([]*models.Machine, error)
	NameExistsForUser(userID, name string) (bool, error)
}

func NewMachineRepository(db *sql.DB) *MachineRepository {
	return &MachineRepository{db: db}
}

// Create inserts a new machine into the database
func (r *MachineRepository) Create(machine *models.Machine) error {
	query := `
		INSERT INTO machines (
			id, name, owner_id, description, hostname, port, 
			ssh_username, ssh_key_encrypted, ssh_key_fingerprint, 
			podman_socket_path, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := r.db.Exec(
		query,
		machine.ID, machine.Name, machine.OwnerID, machine.Description, 
		machine.Hostname, machine.Port, machine.SSHUsername, 
		machine.SSHKeyEncrypted, machine.SSHKeyFingerprint,
		machine.PodmanSocketPath, machine.CreatedAt, machine.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to create machine: %w", err)
	}

	return nil
}

// GetByID retrieves a machine by its ID
func (r *MachineRepository) GetByID(id string) (*models.Machine, error) {
	query := `
		SELECT 
			id, name, owner_id, description, hostname, port, 
			ssh_username, ssh_key_encrypted, ssh_key_fingerprint, 
			podman_socket_path, created_at, updated_at
		FROM machines
		WHERE id = ?
	`

	var machine models.Machine
	err := r.db.QueryRow(query, id).Scan(
		&machine.ID, &machine.Name, &machine.OwnerID, &machine.Description,
		&machine.Hostname, &machine.Port, &machine.SSHUsername,
		&machine.SSHKeyEncrypted, &machine.SSHKeyFingerprint,
		&machine.PodmanSocketPath, &machine.CreatedAt, &machine.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get machine: %w", err)
	}

	return &machine, nil
}

// Update updates an existing machine
func (r *MachineRepository) Update(machine *models.Machine) error {
	query := `
		UPDATE machines SET
			name = ?,
			description = ?,
			hostname = ?,
			port = ?,
			ssh_username = ?,
			ssh_key_encrypted = ?,
			ssh_key_fingerprint = ?,
			podman_socket_path = ?,
			updated_at = ?
		WHERE id = ?
	`

	_, err := r.db.Exec(
		query,
		machine.Name, machine.Description, machine.Hostname, machine.Port,
		machine.SSHUsername, machine.SSHKeyEncrypted, machine.SSHKeyFingerprint,
		machine.PodmanSocketPath, machine.UpdatedAt, machine.ID,
	)

	if err != nil {
		return fmt.Errorf("failed to update machine: %w", err)
	}

	return nil
}

// Delete removes a machine from the database
func (r *MachineRepository) Delete(id string) error {
	query := "DELETE FROM machines WHERE id = ?"

	_, err := r.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete machine: %w", err)
	}

	return nil
}

// ListByOwner retrieves all machines owned by a specific user
func (r *MachineRepository) ListByOwner(ownerID string) ([]*models.Machine, error) {
	query := `
		SELECT 
			id, name, owner_id, description, hostname, port, 
			ssh_username, ssh_key_encrypted, ssh_key_fingerprint, 
			podman_socket_path, created_at, updated_at
		FROM machines
		WHERE owner_id = ?
		ORDER BY name
	`

	rows, err := r.db.Query(query, ownerID)
	if err != nil {
		return nil, fmt.Errorf("failed to list machines by owner: %w", err)
	}
	defer rows.Close()

	var machines []*models.Machine
	for rows.Next() {
		var machine models.Machine
		if err := rows.Scan(
			&machine.ID, &machine.Name, &machine.OwnerID, &machine.Description,
			&machine.Hostname, &machine.Port, &machine.SSHUsername,
			&machine.SSHKeyEncrypted, &machine.SSHKeyFingerprint,
			&machine.PodmanSocketPath, &machine.CreatedAt, &machine.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan machine: %w", err)
		}
		machines = append(machines, &machine)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating machines: %w", err)
	}

	return machines, nil
}

// ListAccessible retrieves all machines accessible to a user (owned + shared)
func (r *MachineRepository) ListAccessible(userID string) ([]*models.Machine, error) {
	query := `
		SELECT DISTINCT m.id, m.name, m.owner_id, m.description, m.hostname, m.port,
			m.ssh_username, m.ssh_key_encrypted, m.ssh_key_fingerprint,
			m.podman_socket_path, m.created_at, m.updated_at
		FROM machines m
		LEFT JOIN machine_access ma ON m.id = ma.machine_id
		WHERE m.owner_id = ? OR ma.user_id = ?
		ORDER BY m.name
	`

	rows, err := r.db.Query(query, userID, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list accessible machines: %w", err)
	}
	defer rows.Close()

	var machines []*models.Machine
	for rows.Next() {
		var machine models.Machine
		if err := rows.Scan(
			&machine.ID, &machine.Name, &machine.OwnerID, &machine.Description,
			&machine.Hostname, &machine.Port, &machine.SSHUsername,
			&machine.SSHKeyEncrypted, &machine.SSHKeyFingerprint,
			&machine.PodmanSocketPath, &machine.CreatedAt, &machine.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan machine: %w", err)
		}
		machines = append(machines, &machine)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating machines: %w", err)
	}

	return machines, nil
}

// NameExistsForUser checks if a machine name already exists for a user
func (r *MachineRepository) NameExistsForUser(userID, name string) (bool, error) {
	query := "SELECT COUNT(*) FROM machines WHERE owner_id = ? AND name = ?"

	var count int
	err := r.db.QueryRow(query, userID, name).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check machine name existence: %w", err)
	}

	return count > 0, nil
}