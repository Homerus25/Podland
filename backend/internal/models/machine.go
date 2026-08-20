package models

import (
	"database/sql"
	"time"
)

// Permission constants
const (
	PermissionRead  = "read"
	PermissionWrite = "write"
	PermissionAdmin = "admin"
)

// Machine represents a machine/host that can be managed
type Machine struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	OwnerID           string    `json:"ownerId"`
	Description       string    `json:"description"`
	Hostname          string    `json:"hostname"`
	Port              int       `json:"port"`
	SSHUsername       string    `json:"sshUsername"`
	SSHKeyEncrypted   []byte    `json:"-"`
	SSHKeyFingerprint string    `json:"sshKeyFingerprint"`
	PodmanSocketPath  string    `json:"podmanSocketPath"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

// MachineRepository handles database operations for machines
type MachineRepository struct {
	db *sql.DB
}

// NewMachineRepository creates a new MachineRepository
func NewMachineRepository(db *sql.DB) *MachineRepository {
	return &MachineRepository{db: db}
}

// Create creates a new machine
func (r *MachineRepository) Create(machine *Machine) error {
	query := `INSERT INTO machines (id, name, owner_id, description, hostname, port, ssh_username, ssh_key_encrypted, ssh_key_fingerprint, podman_socket_path, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := r.db.Exec(query,
		machine.ID, machine.Name, machine.OwnerID, machine.Description,
		machine.Hostname, machine.Port, machine.SSHUsername, machine.SSHKeyEncrypted,
		machine.SSHKeyFingerprint, machine.PodmanSocketPath, machine.CreatedAt, machine.UpdatedAt)
	return err
}

// GetByID retrieves a machine by ID
func (r *MachineRepository) GetByID(id string) (*Machine, error) {
	query := `SELECT id, name, owner_id, description, hostname, port, ssh_username, ssh_key_encrypted, ssh_key_fingerprint, podman_socket_path, created_at, updated_at FROM machines WHERE id = ?`
	machine := &Machine{}
	err := r.db.QueryRow(query, id).Scan(
		&machine.ID, &machine.Name, &machine.OwnerID, &machine.Description,
		&machine.Hostname, &machine.Port, &machine.SSHUsername, &machine.SSHKeyEncrypted,
		&machine.SSHKeyFingerprint, &machine.PodmanSocketPath, &machine.CreatedAt, &machine.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return machine, nil
}

// GetByNameAndOwner retrieves a machine by name and owner ID
func (r *MachineRepository) GetByNameAndOwner(name, ownerID string) (*Machine, error) {
	query := `SELECT id, name, owner_id, description, hostname, port, ssh_username, ssh_key_encrypted, ssh_key_fingerprint, podman_socket_path, created_at, updated_at FROM machines WHERE name = ? AND owner_id = ?`
	machine := &Machine{}
	err := r.db.QueryRow(query, name, ownerID).Scan(
		&machine.ID, &machine.Name, &machine.OwnerID, &machine.Description,
		&machine.Hostname, &machine.Port, &machine.SSHUsername, &machine.SSHKeyEncrypted,
		&machine.SSHKeyFingerprint, &machine.PodmanSocketPath, &machine.CreatedAt, &machine.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return machine, nil
}

// ListByOwner retrieves all machines owned by a user
func (r *MachineRepository) ListByOwner(ownerID string) ([]*Machine, error) {
	query := `SELECT id, name, owner_id, description, hostname, port, ssh_username, ssh_key_encrypted, ssh_key_fingerprint, podman_socket_path, created_at, updated_at FROM machines WHERE owner_id = ?`
	rows, err := r.db.Query(query, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var machines []*Machine
	for rows.Next() {
		machine := &Machine{}
		if err := rows.Scan(
			&machine.ID, &machine.Name, &machine.OwnerID, &machine.Description,
			&machine.Hostname, &machine.Port, &machine.SSHUsername, &machine.SSHKeyEncrypted,
			&machine.SSHKeyFingerprint, &machine.PodmanSocketPath, &machine.CreatedAt, &machine.UpdatedAt); err != nil {
			return nil, err
		}
		machines = append(machines, machine)
	}
	return machines, nil
}

// AccessibleMachine pairs a machine with the caller's permission on it.
// Owners implicitly hold admin via the machine_access row created at
// registration; shared users hold their granted permission.
type AccessibleMachine struct {
	*Machine
	Permission string
}

// ListAccessible retrieves all machines a user has access to, each with
// the caller's permission from machine_access
func (r *MachineRepository) ListAccessible(userID string) ([]AccessibleMachine, error) {
	query := `SELECT m.id, m.name, m.owner_id, m.description, m.hostname, m.port, m.ssh_username, m.ssh_key_encrypted, m.ssh_key_fingerprint, m.podman_socket_path, m.created_at, m.updated_at, ma.permission
		FROM machines m
		JOIN machine_access ma ON m.id = ma.machine_id
		WHERE ma.user_id = ?`
	rows, err := r.db.Query(query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var machines []AccessibleMachine
	for rows.Next() {
		machine := &Machine{}
		var permission string
		if err := rows.Scan(
			&machine.ID, &machine.Name, &machine.OwnerID, &machine.Description,
			&machine.Hostname, &machine.Port, &machine.SSHUsername, &machine.SSHKeyEncrypted,
			&machine.SSHKeyFingerprint, &machine.PodmanSocketPath, &machine.CreatedAt, &machine.UpdatedAt,
			&permission); err != nil {
			return nil, err
		}
		machines = append(machines, AccessibleMachine{Machine: machine, Permission: permission})
	}
	return machines, nil
}

// Update updates a machine
func (r *MachineRepository) Update(machine *Machine) error {
	query := `UPDATE machines SET name = ?, description = ?, hostname = ?, port = ?, ssh_username = ?, ssh_key_encrypted = ?, ssh_key_fingerprint = ?, podman_socket_path = ?, updated_at = ? WHERE id = ?`
	_, err := r.db.Exec(query,
		machine.Name, machine.Description, machine.Hostname, machine.Port,
		machine.SSHUsername, machine.SSHKeyEncrypted, machine.SSHKeyFingerprint,
		machine.PodmanSocketPath, machine.UpdatedAt, machine.ID)
	return err
}

// Delete deletes a machine by ID
func (r *MachineRepository) Delete(id string) error {
	query := `DELETE FROM machines WHERE id = ?`
	_, err := r.db.Exec(query, id)
	return err
}

// NameExistsForUser checks if a machine name already exists for a user
func (r *MachineRepository) NameExistsForUser(userID, name string) (bool, error) {
	query := `SELECT COUNT(*) FROM machines WHERE name = ? AND owner_id = ?`
	var count int
	err := r.db.QueryRow(query, name, userID).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}
