package models

import (
	"database/sql"
	"fmt"
)

// MachineAccessRepository handles database operations for machine access permissions
type MachineAccessRepository struct {
	db *sql.DB
}

// MachineAccessRepositoryInterface defines the interface for machine access repository operations
type MachineAccessRepositoryInterface interface {
	Create(access *MachineAccess) error
	GetByMachineAndUser(machineID, userID string) (*MachineAccess, error)
	ListByMachine(machineID string) ([]*MachineAccess, error)
	ListByUser(userID string) ([]*MachineAccess, error)
	Update(access *MachineAccess) error
	Delete(machineID, userID string) error
	HasAccess(machineID, userID string) (bool, error)
}

// NewMachineAccessRepository creates a new MachineAccessRepository
func NewMachineAccessRepository(db *sql.DB) *MachineAccessRepository {
	return &MachineAccessRepository{db: db}
}

// Create inserts a new machine access record
func (r *MachineAccessRepository) Create(access *MachineAccess) error {
	query := `INSERT INTO machine_access (machine_id, user_id, permission, granted_at) VALUES (?, ?, ?, ?)`

	_, err := r.db.Exec(query, access.MachineID, access.UserID, access.Permission, access.GrantedAt)
	if err != nil {
		return fmt.Errorf("failed to create machine access: %w", err)
	}

	return nil
}

// GetByMachineAndUser retrieves a machine access record by machine and user ID
func (r *MachineAccessRepository) GetByMachineAndUser(machineID, userID string) (*MachineAccess, error) {
	query := `SELECT machine_id, user_id, permission, granted_at FROM machine_access WHERE machine_id = ? AND user_id = ?`

	var access MachineAccess
	err := r.db.QueryRow(query, machineID, userID).Scan(&access.MachineID, &access.UserID, &access.Permission, &access.GrantedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get machine access: %w", err)
	}

	return &access, nil
}

// ListByMachine retrieves all access records for a specific machine
func (r *MachineAccessRepository) ListByMachine(machineID string) ([]*MachineAccess, error) {
	query := `SELECT machine_id, user_id, permission, granted_at FROM machine_access WHERE machine_id = ? ORDER BY granted_at DESC`

	rows, err := r.db.Query(query, machineID)
	if err != nil {
		return nil, fmt.Errorf("failed to list machine access: %w", err)
	}
	defer rows.Close()

	var accesses []*MachineAccess
	for rows.Next() {
		var access MachineAccess
		if err := rows.Scan(&access.MachineID, &access.UserID, &access.Permission, &access.GrantedAt); err != nil {
			return nil, fmt.Errorf("failed to scan machine access: %w", err)
		}
		accesses = append(accesses, &access)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating machine access: %w", err)
	}

	return accesses, nil
}

// ListByUser retrieves all access records for a specific user
func (r *MachineAccessRepository) ListByUser(userID string) ([]*MachineAccess, error) {
	query := `SELECT machine_id, user_id, permission, granted_at FROM machine_access WHERE user_id = ? ORDER BY granted_at DESC`

	rows, err := r.db.Query(query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list user access: %w", err)
	}
	defer rows.Close()

	var accesses []*MachineAccess
	for rows.Next() {
		var access MachineAccess
		if err := rows.Scan(&access.MachineID, &access.UserID, &access.Permission, &access.GrantedAt); err != nil {
			return nil, fmt.Errorf("failed to scan user access: %w", err)
		}
		accesses = append(accesses, &access)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating user access: %w", err)
	}

	return accesses, nil
}

// Update updates an existing machine access record
func (r *MachineAccessRepository) Update(access *MachineAccess) error {
	query := `UPDATE machine_access SET permission = ? WHERE machine_id = ? AND user_id = ?`

	_, err := r.db.Exec(query, access.Permission, access.MachineID, access.UserID)
	if err != nil {
		return fmt.Errorf("failed to update machine access: %w", err)
	}

	return nil
}

// Delete removes a machine access record
func (r *MachineAccessRepository) Delete(machineID, userID string) error {
	query := `DELETE FROM machine_access WHERE machine_id = ? AND user_id = ?`

	_, err := r.db.Exec(query, machineID, userID)
	if err != nil {
		return fmt.Errorf("failed to delete machine access: %w", err)
	}

	return nil
}

// HasAccess checks if a user has access to a machine
func (r *MachineAccessRepository) HasAccess(machineID, userID string) (bool, error) {
	query := `SELECT COUNT(*) FROM machine_access WHERE machine_id = ? AND user_id = ?`

	var count int
	err := r.db.QueryRow(query, machineID, userID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check access: %w", err)
	}

	return count > 0, nil
}
