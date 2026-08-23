// Package users owns user management: creating users (hash + exists
// check + insert) behind one service path shared by the admin-only
// POST /api/users endpoint, the cmd/initdb bootstrap tool, and the
// integration-test seeding seam.
package users

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/mattn/go-sqlite3"
	apperrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/models"
	"golang.org/x/crypto/bcrypt"
)

// UserService handles user management operations.
type UserService struct {
	userRepo models.UserRepositoryInterface
}

// NewUserService creates a new UserService.
func NewUserService(userRepo models.UserRepositoryInterface) *UserService {
	return &UserService{userRepo: userRepo}
}

// CreateUser creates a regular user with role "user". See
// CreateUserWithRole for the full contract.
func (s *UserService) CreateUser(username, password string) (*models.User, error) {
	return s.CreateUserWithRole(username, password, models.RoleUser)
}

// CreateUserWithRole creates a user with the given role, bcrypt-hashing
// the password before it is persisted. The plaintext password never
// reaches the repository.
func (s *UserService) CreateUserWithRole(username, password, role string) (*models.User, error) {
	if username == "" || password == "" {
		return nil, apperrors.ErrMissingRequiredFields
	}

	exists, err := s.userRepo.UsernameExists(username)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, apperrors.ErrUsernameTaken
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user := &models.User{
		ID:           uuid.New().String(),
		Username:     username,
		PasswordHash: string(hash),
		Role:         role,
		CreatedAt:    time.Now().UTC(),
	}

	if err := s.userRepo.Create(user); err != nil {
		// Backstop for the concurrent-duplicate race: two creations can
		// pass the exists-check before either INSERT lands, and the
		// loser then trips the users.username UNIQUE constraint.
		var sqliteErr sqlite3.Error
		if errors.As(err, &sqliteErr) &&
			sqliteErr.ExtendedCode == sqlite3.ErrConstraintUnique {
			return nil, apperrors.ErrUsernameTaken
		}
		return nil, err
	}

	return user, nil
}
