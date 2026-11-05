package models

import (
	"database/sql"
	"fmt"
)

// HostAccessRepository handles database operations for host access permissions
type HostAccessRepository struct {
	db *sql.DB
}

// HostAccessRepositoryInterface defines the interface for host access repository operations
type HostAccessRepositoryInterface interface {
	Create(access *HostAccess) error
	GetByHostAndUser(hostID, userID string) (*HostAccess, error)
	ListByHost(hostID string) ([]*HostAccess, error)
	ListByUser(userID string) ([]*HostAccess, error)
	Update(access *HostAccess) error
	Delete(hostID, userID string) error
	HasAccess(hostID, userID string) (bool, error)
	GetPermission(hostID, userID string) (string, error)
}

// NewHostAccessRepository creates a new HostAccessRepository
func NewHostAccessRepository(db *sql.DB) *HostAccessRepository {
	return &HostAccessRepository{db: db}
}

// Create inserts a new host access record
func (r *HostAccessRepository) Create(access *HostAccess) error {
	query := `INSERT INTO host_access (host_id, user_id, permission, granted_at) VALUES (?, ?, ?, ?)`

	_, err := r.db.Exec(query, access.HostID, access.UserID, access.Permission, access.GrantedAt)
	if err != nil {
		return fmt.Errorf("failed to create host access: %w", err)
	}

	return nil
}

// GetByHostAndUser retrieves a host access record by host and user ID
func (r *HostAccessRepository) GetByHostAndUser(hostID, userID string) (*HostAccess, error) {
	query := `SELECT host_id, user_id, permission, granted_at FROM host_access WHERE host_id = ? AND user_id = ?`

	var access HostAccess
	err := r.db.QueryRow(query, hostID, userID).Scan(&access.HostID, &access.UserID, &access.Permission, &access.GrantedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get host access: %w", err)
	}

	return &access, nil
}

// ListByHost retrieves all access records for a specific host
func (r *HostAccessRepository) ListByHost(hostID string) ([]*HostAccess, error) {
	query := `SELECT host_id, user_id, permission, granted_at FROM host_access WHERE host_id = ? ORDER BY granted_at DESC`

	rows, err := r.db.Query(query, hostID)
	if err != nil {
		return nil, fmt.Errorf("failed to list host access: %w", err)
	}
	defer rows.Close()

	var accesses []*HostAccess
	for rows.Next() {
		var access HostAccess
		if err := rows.Scan(&access.HostID, &access.UserID, &access.Permission, &access.GrantedAt); err != nil {
			return nil, fmt.Errorf("failed to scan host access: %w", err)
		}
		accesses = append(accesses, &access)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating host access: %w", err)
	}

	return accesses, nil
}

// ListByUser retrieves all access records for a specific user
func (r *HostAccessRepository) ListByUser(userID string) ([]*HostAccess, error) {
	query := `SELECT host_id, user_id, permission, granted_at FROM host_access WHERE user_id = ? ORDER BY granted_at DESC`

	rows, err := r.db.Query(query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list user access: %w", err)
	}
	defer rows.Close()

	var accesses []*HostAccess
	for rows.Next() {
		var access HostAccess
		if err := rows.Scan(&access.HostID, &access.UserID, &access.Permission, &access.GrantedAt); err != nil {
			return nil, fmt.Errorf("failed to scan user access: %w", err)
		}
		accesses = append(accesses, &access)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating user access: %w", err)
	}

	return accesses, nil
}

// Update updates an existing host access record
func (r *HostAccessRepository) Update(access *HostAccess) error {
	query := `UPDATE host_access SET permission = ? WHERE host_id = ? AND user_id = ?`

	_, err := r.db.Exec(query, access.Permission, access.HostID, access.UserID)
	if err != nil {
		return fmt.Errorf("failed to update host access: %w", err)
	}

	return nil
}

// Delete removes a host access record
func (r *HostAccessRepository) Delete(hostID, userID string) error {
	query := `DELETE FROM host_access WHERE host_id = ? AND user_id = ?`

	_, err := r.db.Exec(query, hostID, userID)
	if err != nil {
		return fmt.Errorf("failed to delete host access: %w", err)
	}

	return nil
}

// HasAccess checks if a user has access to a host
func (r *HostAccessRepository) HasAccess(hostID, userID string) (bool, error) {
	query := `SELECT COUNT(*) FROM host_access WHERE host_id = ? AND user_id = ?`

	var count int
	err := r.db.QueryRow(query, hostID, userID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check access: %w", err)
	}

	return count > 0, nil
}

// GetPermission returns the permission level a user holds on a host,
// or the empty string when no host_access row exists. Level-aware
// gates compare this value through models.HasAtLeast; a missing row is
// the bottom of the ladder, not an error.
func (r *HostAccessRepository) GetPermission(hostID, userID string) (string, error) {
	query := `SELECT permission FROM host_access WHERE host_id = ? AND user_id = ?`

	var permission string
	err := r.db.QueryRow(query, hostID, userID).Scan(&permission)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", fmt.Errorf("failed to get permission: %w", err)
	}

	return permission, nil
}
