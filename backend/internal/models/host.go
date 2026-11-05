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

// permissionRank fixes the permission ladder read < write < admin. It is
// the single source of truth for ordering: middleware and handlers
// compare levels through HasAtLeast instead of re-deriving the order.
var permissionRank = map[string]int{
	PermissionRead:  1,
	PermissionWrite: 2,
	PermissionAdmin: 3,
}

// HasAtLeast reports whether the actual permission a caller holds is at
// or above the required one (read < write < admin). An unknown level on
// either side — including the empty string the access repository
// returns when no row exists — denies, so a missing or malformed grant
// never escalates.
func HasAtLeast(required, actual string) bool {
	reqRank, reqOK := permissionRank[required]
	actRank, actOK := permissionRank[actual]
	return reqOK && actOK && actRank >= reqRank
}

// Host represents a remote host that can be managed
type Host struct {
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

// HostRepository handles database operations for hosts
type HostRepository struct {
	db *sql.DB
}

// NewHostRepository creates a new HostRepository
func NewHostRepository(db *sql.DB) *HostRepository {
	return &HostRepository{db: db}
}

// Create creates a new host
func (r *HostRepository) Create(host *Host) error {
	query := `INSERT INTO hosts (id, name, owner_id, description, hostname, port, ssh_username, ssh_key_encrypted, ssh_key_fingerprint, podman_socket_path, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := r.db.Exec(query,
		host.ID, host.Name, host.OwnerID, host.Description,
		host.Hostname, host.Port, host.SSHUsername, host.SSHKeyEncrypted,
		host.SSHKeyFingerprint, host.PodmanSocketPath, host.CreatedAt, host.UpdatedAt)
	return err
}

// GetByID retrieves a host by ID
func (r *HostRepository) GetByID(id string) (*Host, error) {
	query := `SELECT id, name, owner_id, description, hostname, port, ssh_username, ssh_key_encrypted, ssh_key_fingerprint, podman_socket_path, created_at, updated_at FROM hosts WHERE id = ?`
	host := &Host{}
	err := r.db.QueryRow(query, id).Scan(
		&host.ID, &host.Name, &host.OwnerID, &host.Description,
		&host.Hostname, &host.Port, &host.SSHUsername, &host.SSHKeyEncrypted,
		&host.SSHKeyFingerprint, &host.PodmanSocketPath, &host.CreatedAt, &host.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return host, nil
}

// ListByOwner retrieves all hosts owned by a user
func (r *HostRepository) ListByOwner(ownerID string) ([]*Host, error) {
	query := `SELECT id, name, owner_id, description, hostname, port, ssh_username, ssh_key_encrypted, ssh_key_fingerprint, podman_socket_path, created_at, updated_at FROM hosts WHERE owner_id = ?`
	rows, err := r.db.Query(query, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hosts []*Host
	for rows.Next() {
		host := &Host{}
		if err := rows.Scan(
			&host.ID, &host.Name, &host.OwnerID, &host.Description,
			&host.Hostname, &host.Port, &host.SSHUsername, &host.SSHKeyEncrypted,
			&host.SSHKeyFingerprint, &host.PodmanSocketPath, &host.CreatedAt, &host.UpdatedAt); err != nil {
			return nil, err
		}
		hosts = append(hosts, host)
	}
	return hosts, nil
}

// AccessibleHost pairs a host with the caller's permission on it.
// Owners implicitly hold admin via the host_access row created at
// registration; shared users hold their granted permission.
type AccessibleHost struct {
	*Host
	Permission string
}

// ListAccessible retrieves all hosts a user has access to, each with
// the caller's permission from host_access
func (r *HostRepository) ListAccessible(userID string) ([]AccessibleHost, error) {
	query := `SELECT m.id, m.name, m.owner_id, m.description, m.hostname, m.port, m.ssh_username, m.ssh_key_encrypted, m.ssh_key_fingerprint, m.podman_socket_path, m.created_at, m.updated_at, ma.permission
		FROM hosts m
		JOIN host_access ma ON m.id = ma.host_id
		WHERE ma.user_id = ?`
	rows, err := r.db.Query(query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hosts []AccessibleHost
	for rows.Next() {
		host := &Host{}
		var permission string
		if err := rows.Scan(
			&host.ID, &host.Name, &host.OwnerID, &host.Description,
			&host.Hostname, &host.Port, &host.SSHUsername, &host.SSHKeyEncrypted,
			&host.SSHKeyFingerprint, &host.PodmanSocketPath, &host.CreatedAt, &host.UpdatedAt,
			&permission); err != nil {
			return nil, err
		}
		hosts = append(hosts, AccessibleHost{Host: host, Permission: permission})
	}
	return hosts, nil
}

// Update updates a host
func (r *HostRepository) Update(host *Host) error {
	query := `UPDATE hosts SET name = ?, description = ?, hostname = ?, port = ?, ssh_username = ?, ssh_key_encrypted = ?, ssh_key_fingerprint = ?, podman_socket_path = ?, updated_at = ? WHERE id = ?`
	_, err := r.db.Exec(query,
		host.Name, host.Description, host.Hostname, host.Port,
		host.SSHUsername, host.SSHKeyEncrypted, host.SSHKeyFingerprint,
		host.PodmanSocketPath, host.UpdatedAt, host.ID)
	return err
}

// Delete deletes a host by ID
func (r *HostRepository) Delete(id string) error {
	query := `DELETE FROM hosts WHERE id = ?`
	_, err := r.db.Exec(query, id)
	return err
}

// NameExistsForUser checks if a host name already exists for a user
func (r *HostRepository) NameExistsForUser(userID, name string) (bool, error) {
	query := `SELECT COUNT(*) FROM hosts WHERE name = ? AND owner_id = ?`
	var count int
	err := r.db.QueryRow(query, name, userID).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}
