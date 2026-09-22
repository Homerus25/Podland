package models

import (
	"database/sql"
	"fmt"
)

// UserRepository handles database operations for users
type UserRepository struct {
	db *sql.DB
}

// UserRepositoryInterface defines the interface for user repository operations
type UserRepositoryInterface interface {
	Create(user *User) error
	GetByID(id string) (*User, error)
	GetByUsername(username string) (*User, error)
	List() ([]*User, error)
	Update(user *User) error
	Delete(id string) error
	UsernameExists(username string) (bool, error)
}

// NewUserRepository creates a new UserRepository
func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

// requireRole rejects any role outside {user, admin}. Unlike
// normalizeRole it does NOT default an empty role: Update must receive
// an explicit role so a hand-built User can never silently demote an
// admin back to "user".
func requireRole(role string) error {
	if !ValidRole(role) {
		return fmt.Errorf("invalid role %q: must be %q or %q", role, RoleUser, RoleAdmin)
	}
	return nil
}

// normalizeRole validates the role on Create. An unset role defaults to
// RoleUser, matching the DB column default.
func normalizeRole(role string) (string, error) {
	if role == "" {
		return RoleUser, nil
	}
	if err := requireRole(role); err != nil {
		return "", err
	}
	return role, nil
}

// Create inserts a new user into the database
func (r *UserRepository) Create(user *User) error {
	role, err := normalizeRole(user.Role)
	if err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}
	user.Role = role

	query := `INSERT INTO users (id, username, password_hash, role, created_at) VALUES (?, ?, ?, ?, ?)`

	_, err = r.db.Exec(query, user.ID, user.Username, user.PasswordHash, user.Role, user.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}

	return nil
}

// GetByID retrieves a user by their ID
func (r *UserRepository) GetByID(id string) (*User, error) {
	query := `SELECT id, username, password_hash, role, created_at FROM users WHERE id = ?`

	var user User
	err := r.db.QueryRow(query, id).Scan(&user.ID, &user.Username, &user.PasswordHash, &user.Role, &user.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return &user, nil
}

// GetByUsername retrieves a user by their username
func (r *UserRepository) GetByUsername(username string) (*User, error) {
	query := `SELECT id, username, password_hash, role, created_at FROM users WHERE username = ?`

	var user User
	err := r.db.QueryRow(query, username).Scan(&user.ID, &user.Username, &user.PasswordHash, &user.Role, &user.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return &user, nil
}

// List retrieves all users ordered by username, so the users-list
// endpoint answers with a stable order across calls.
func (r *UserRepository) List() ([]*User, error) {
	query := `SELECT id, username, password_hash, role, created_at FROM users ORDER BY username`

	rows, err := r.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		var user User
		if err := rows.Scan(&user.ID, &user.Username, &user.PasswordHash, &user.Role, &user.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to list users: %w", err)
		}
		users = append(users, &user)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}

	return users, nil
}

// Update updates an existing user. The role must be set explicitly —
// unlike Create there is no default — so a stale or hand-built User
// cannot silently overwrite an admin's role.
func (r *UserRepository) Update(user *User) error {
	if err := requireRole(user.Role); err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}

	query := `UPDATE users SET username = ?, password_hash = ?, role = ? WHERE id = ?`

	_, err := r.db.Exec(query, user.Username, user.PasswordHash, user.Role, user.ID)
	if err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}

	return nil
}

// Delete removes a user from the database
func (r *UserRepository) Delete(id string) error {
	query := `DELETE FROM users WHERE id = ?`

	_, err := r.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}

	return nil
}

// UsernameExists checks if a username already exists
func (r *UserRepository) UsernameExists(username string) (bool, error) {
	query := `SELECT COUNT(*) FROM users WHERE username = ?`

	var count int
	err := r.db.QueryRow(query, username).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check username existence: %w", err)
	}

	return count > 0, nil
}
