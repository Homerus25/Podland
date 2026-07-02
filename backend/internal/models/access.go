package models

import (
	"database/sql"
	"time"
)

// MachineAccess represents access permissions for a user on a machine
type MachineAccess struct {
	MachineID  string    `json:"machineId"`
	UserID     string    `json:"userId"`
	Permission string    `json:"permission"`
	GrantedAt  time.Time `json:"grantedAt"`
}

// MachineAccessRepository handles database operations for machine access
type MachineAccessRepository struct {
	db *sql.DB
}

// NewMachineAccessRepository creates a new MachineAccessRepository
func NewMachineAccessRepository(db *sql.DB) *MachineAccessRepository {
	return &MachineAccessRepository{db: db}
}

// Create creates a new machine access entry
func (r *MachineAccessRepository) Create(access *MachineAccess) error {
	query := `INSERT INTO machine_access (machine_id, user_id, permission, granted_at) VALUES (?, ?, ?, ?)`
	_, err := r.db.Exec(query, access.MachineID, access.UserID, access.Permission, access.GrantedAt)
	return err
}

// GetByMachineAndUser retrieves access permissions for a user on a machine
func (r *MachineAccessRepository) GetByMachineAndUser(machineID, userID string) (*MachineAccess, error) {
	query := `SELECT machine_id, user_id, permission, granted_at FROM machine_access WHERE machine_id = ? AND user_id = ?`
	access := &MachineAccess{}
	err := r.db.QueryRow(query, machineID, userID).Scan(&access.MachineID, &access.UserID, &access.Permission, &access.GrantedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return access, nil
}

// ListByMachine retrieves all access entries for a machine
func (r *MachineAccessRepository) ListByMachine(machineID string) ([]*MachineAccess, error) {
	query := `SELECT machine_id, user_id, permission, granted_at FROM machine_access WHERE machine_id = ?`
	rows, err := r.db.Query(query, machineID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var accesses []*MachineAccess
	for rows.Next() {
		access := &MachineAccess{}
		if err := rows.Scan(&access.MachineID, &access.UserID, &access.Permission, &access.GrantedAt); err != nil {
			return nil, err
		}
		accesses = append(accesses, access)
	}
	return accesses, nil
}

// ListByUser retrieves all machines a user has access to
func (r *MachineAccessRepository) ListByUser(userID string) ([]*MachineAccess, error) {
	query := `SELECT machine_id, user_id, permission, granted_at FROM machine_access WHERE user_id = ?`
	rows, err := r.db.Query(query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var accesses []*MachineAccess
	for rows.Next() {
		access := &MachineAccess{}
		if err := rows.Scan(&access.MachineID, &access.UserID, &access.Permission, &access.GrantedAt); err != nil {
			return nil, err
		}
		accesses = append(accesses, access)
	}
	return accesses, nil
}

// Update updates access permissions
func (r *MachineAccessRepository) Update(access *MachineAccess) error {
	query := `UPDATE machine_access SET permission = ? WHERE machine_id = ? AND user_id = ?`
	_, err := r.db.Exec(query, access.Permission, access.MachineID, access.UserID)
	return err
}

// Delete deletes an access entry
func (r *MachineAccessRepository) Delete(machineID, userID string) error {
	query := `DELETE FROM machine_access WHERE machine_id = ? AND user_id = ?`
	_, err := r.db.Exec(query, machineID, userID)
	return err
}

// DeleteByMachine deletes all access entries for a machine
func (r *MachineAccessRepository) DeleteByMachine(machineID string) error {
	query := `DELETE FROM machine_access WHERE machine_id = ?`
	_, err := r.db.Exec(query, machineID)
	return err
}

// DeleteByUser deletes all access entries for a user
func (r *MachineAccessRepository) DeleteByUser(userID string) error {
	query := `DELETE FROM machine_access WHERE user_id = ?`
	_, err := r.db.Exec(query, userID)
	return err
}

// HasAccess checks if a user has any access to a machine
func (r *MachineAccessRepository) HasAccess(machineID, userID string) (bool, error) {
	query := `SELECT COUNT(*) FROM machine_access WHERE machine_id = ? AND user_id = ?`
	var count int
	err := r.db.QueryRow(query, machineID, userID).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// HasPermission checks if a user has a specific permission on a machine
func (r *MachineAccessRepository) HasPermission(machineID, userID, permission string) (bool, error) {
	query := `SELECT COUNT(*) FROM machine_access WHERE machine_id = ? AND user_id = ? AND permission = ?`
	var count int
	err := r.db.QueryRow(query, machineID, userID, permission).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}
