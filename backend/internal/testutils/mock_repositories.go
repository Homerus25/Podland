package testutils

import (
	"time"

	"github.com/Homerus25/podland/backend/internal/models"
)

// MockUserRepository implements a mock user repository for testing
type MockUserRepository struct {
	users        map[string]*models.User
	createErr    error
	getByIDErr   error
	getByNameErr error
	listErr      error
	updateErr    error
	deleteErr    error
	existsErr    error
}

func NewMockUserRepository() *MockUserRepository {
	return &MockUserRepository{
		users: make(map[string]*models.User),
	}
}

// NewMockUserRepositoryWithErrors creates a mock repository with configurable errors
func NewMockUserRepositoryWithErrors(createErr, getByIDErr, getByNameErr, updateErr, deleteErr, existsErr error) *MockUserRepository {
	return &MockUserRepository{
		users:        make(map[string]*models.User),
		createErr:    createErr,
		getByIDErr:   getByIDErr,
		getByNameErr: getByNameErr,
		updateErr:    updateErr,
		deleteErr:    deleteErr,
		existsErr:    existsErr,
	}
}

// WithListError returns the same repository with List configured to
// fail, for tests injecting a list-time storage failure (mirrors
// MockSessionRepository.WithRotateError).
func (m *MockUserRepository) WithListError(err error) *MockUserRepository {
	m.listErr = err
	return m
}

// WithUpdateError returns the same repository with Update configured to
// fail, for tests injecting an update-time storage failure (mirrors
// WithListError).
func (m *MockUserRepository) WithUpdateError(err error) *MockUserRepository {
	m.updateErr = err
	return m
}

// AddUser adds a user directly to the mock repository
func (m *MockUserRepository) AddUser(user *models.User) {
	m.users[user.ID] = user
}

func (m *MockUserRepository) Create(user *models.User) error {
	if m.createErr != nil {
		return m.createErr
	}
	m.users[user.ID] = user
	return nil
}

func (m *MockUserRepository) GetByID(id string) (*models.User, error) {
	if m.getByIDErr != nil {
		return nil, m.getByIDErr
	}
	user, exists := m.users[id]
	if !exists {
		return nil, nil
	}
	// Copy, like a real read: a caller mutating the returned struct must
	// not change the stored row behind the repository's back — only
	// Update persists.
	stored := *user
	return &stored, nil
}

func (m *MockUserRepository) GetByUsername(username string) (*models.User, error) {
	if m.getByNameErr != nil {
		return nil, m.getByNameErr
	}
	for _, user := range m.users {
		if user.Username == username {
			stored := *user
			return &stored, nil
		}
	}
	return nil, nil
}

// List returns every stored user. The mock holds users in a map, so the
// order is unspecified — tests pinning order must go through the real
// repository.
func (m *MockUserRepository) List() ([]*models.User, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	users := make([]*models.User, 0, len(m.users))
	for _, user := range m.users {
		// Copy, like a real read: a caller mutating a returned row must
		// not change the stored row behind the repository's back — only
		// Update persists.
		stored := *user
		users = append(users, &stored)
	}
	return users, nil
}

func (m *MockUserRepository) Update(user *models.User) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	m.users[user.ID] = user
	return nil
}

func (m *MockUserRepository) Delete(id string) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	delete(m.users, id)
	return nil
}

func (m *MockUserRepository) UsernameExists(username string) (bool, error) {
	if m.existsErr != nil {
		return false, m.existsErr
	}
	for _, user := range m.users {
		if user.Username == username {
			return true, nil
		}
	}
	return false, nil
}

// MockMachineRepository implements a mock machine repository for testing
type MockMachineRepository struct {
	machines    map[string]*models.Machine
	listErr     error
	getErr      error
	createErr   error
	updateErr   error
	deleteErr   error
	updateCalls []string
}

func NewMockMachineRepository() *MockMachineRepository {
	return &MockMachineRepository{
		machines: make(map[string]*models.Machine),
	}
}

// NewMockMachineRepositoryWithErrors creates a mock repository with configurable errors
func NewMockMachineRepositoryWithErrors(listErr, getErr, createErr, updateErr, deleteErr error) *MockMachineRepository {
	return &MockMachineRepository{
		machines:  make(map[string]*models.Machine),
		listErr:   listErr,
		getErr:    getErr,
		createErr: createErr,
		updateErr: updateErr,
		deleteErr: deleteErr,
	}
}

// AddMachine adds a machine directly to the mock repository
func (m *MockMachineRepository) AddMachine(machine *models.Machine) {
	m.machines[machine.ID] = machine
}

func (m *MockMachineRepository) Create(machine *models.Machine) error {
	if m.createErr != nil {
		return m.createErr
	}
	m.machines[machine.ID] = machine
	return nil
}

func (m *MockMachineRepository) GetByID(id string) (*models.Machine, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	machine, exists := m.machines[id]
	if !exists {
		return nil, nil
	}
	return machine, nil
}

func (m *MockMachineRepository) Update(machine *models.Machine) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	m.updateCalls = append(m.updateCalls, machine.ID)
	m.machines[machine.ID] = machine
	return nil
}

// UpdateCalls returns a snapshot of every successful Update invocation's
// machine IDs, in call order, so service tests can pin that a failed
// operation (e.g. a failed update probe) wrote nothing.
func (m *MockMachineRepository) UpdateCalls() []string {
	out := make([]string, len(m.updateCalls))
	copy(out, m.updateCalls)
	return out
}

func (m *MockMachineRepository) Delete(id string) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	delete(m.machines, id)
	return nil
}

func (m *MockMachineRepository) ListAccessible(userID string) ([]models.AccessibleMachine, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	// For simplicity, return all machines - in real tests you can customize this behavior.
	// Owners are granted admin, mirroring the access row Create grants.
	var result []models.AccessibleMachine
	for _, machine := range m.machines {
		if machine.OwnerID == userID {
			result = append(result, models.AccessibleMachine{Machine: machine, Permission: models.PermissionAdmin})
		}
	}
	return result, nil
}

func (m *MockMachineRepository) NameExistsForUser(userID, name string) (bool, error) {
	for _, machine := range m.machines {
		if machine.OwnerID == userID && machine.Name == name {
			return true, nil
		}
	}
	return false, nil
}

// MockSessionRepository implements a mock session repository for testing
type MockSessionRepository struct {
	sessions         map[string]*models.Session // keyed by token hash
	createErr        error
	getByHashErr     error
	deleteErr        error
	deleteExpiredErr error
	rotateErr        error
	deleteAllErr     error
}

func NewMockSessionRepository() *MockSessionRepository {
	return &MockSessionRepository{
		sessions: make(map[string]*models.Session),
	}
}

// NewMockSessionRepositoryWithErrors creates a mock session repository
// with configurable errors
func NewMockSessionRepositoryWithErrors(createErr, getByHashErr, deleteErr, deleteExpiredErr error) *MockSessionRepository {
	return &MockSessionRepository{
		sessions:         make(map[string]*models.Session),
		createErr:        createErr,
		getByHashErr:     getByHashErr,
		deleteErr:        deleteErr,
		deleteExpiredErr: deleteExpiredErr,
	}
}

// WithRotateError returns the same repository with Rotate configured to
// fail, for tests injecting a rotation-time storage failure.
func (m *MockSessionRepository) WithRotateError(err error) *MockSessionRepository {
	m.rotateErr = err
	return m
}

// WithDeleteAllError returns the same repository with DeleteAllForUser
// configured to fail, for tests injecting a revoke-all storage failure
// (mirrors WithRotateError).
func (m *MockSessionRepository) WithDeleteAllError(err error) *MockSessionRepository {
	m.deleteAllErr = err
	return m
}

// AddSession adds a session directly to the mock repository
func (m *MockSessionRepository) AddSession(session *models.Session) {
	m.sessions[session.TokenHash] = session
}

// SessionsForUser returns every stored session belonging to the user,
// for tests asserting how many rows login created (multi-session).
func (m *MockSessionRepository) SessionsForUser(userID string) []*models.Session {
	var result []*models.Session
	for _, session := range m.sessions {
		if session.UserID == userID {
			result = append(result, session)
		}
	}
	return result
}

func (m *MockSessionRepository) Create(session *models.Session) error {
	if m.createErr != nil {
		return m.createErr
	}
	m.sessions[session.TokenHash] = session
	return nil
}

func (m *MockSessionRepository) GetByTokenHash(tokenHash string) (*models.Session, error) {
	if m.getByHashErr != nil {
		return nil, m.getByHashErr
	}
	session, exists := m.sessions[tokenHash]
	if !exists {
		return nil, nil
	}
	return session, nil
}

func (m *MockSessionRepository) DeleteByTokenHash(tokenHash string) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	delete(m.sessions, tokenHash)
	return nil
}

func (m *MockSessionRepository) Rotate(oldTokenHash string, newSession *models.Session) (bool, error) {
	if m.rotateErr != nil {
		return false, m.rotateErr
	}
	if _, exists := m.sessions[oldTokenHash]; !exists {
		return false, nil
	}
	delete(m.sessions, oldTokenHash)
	m.sessions[newSession.TokenHash] = newSession
	return true, nil
}

func (m *MockSessionRepository) DeleteExpiredForUser(userID string) error {
	if m.deleteExpiredErr != nil {
		return m.deleteExpiredErr
	}
	now := time.Now().UTC()
	for hash, session := range m.sessions {
		if session.UserID == userID && !session.ExpiresAt.After(now) {
			delete(m.sessions, hash)
		}
	}
	return nil
}

func (m *MockSessionRepository) DeleteAllForUser(userID string) error {
	if m.deleteAllErr != nil {
		return m.deleteAllErr
	}
	for hash, session := range m.sessions {
		if session.UserID == userID {
			delete(m.sessions, hash)
		}
	}
	return nil
}

// MockMachineAccessRepository implements a mock machine access repository for testing
type MockMachineAccessRepository struct {
	accesses               map[string]*MockMachineAccessEntry
	createErr              error
	getByMachineAndUserErr error
	listByMachineErr       error
	listByUserErr          error
	updateErr              error
	deleteErr              error
	hasAccessErr           error
	getPermissionErr       error
}

// MockMachineAccessEntry represents a mock machine access entry
// Using machineID+userID as composite key
type MockMachineAccessEntry struct {
	machineID  string
	userID     string
	permission string
	grantedAt  time.Time
}

func NewMockMachineAccessRepository() *MockMachineAccessRepository {
	return &MockMachineAccessRepository{
		accesses: make(map[string]*MockMachineAccessEntry),
	}
}

// NewMockMachineAccessRepositoryWithErrors creates a mock access repository with configurable errors
func NewMockMachineAccessRepositoryWithErrors(createErr, getByMachineAndUserErr, listByMachineErr, listByUserErr, updateErr, deleteErr, hasAccessErr error) *MockMachineAccessRepository {
	return &MockMachineAccessRepository{
		accesses:               make(map[string]*MockMachineAccessEntry),
		createErr:              createErr,
		getByMachineAndUserErr: getByMachineAndUserErr,
		listByMachineErr:       listByMachineErr,
		listByUserErr:          listByUserErr,
		updateErr:              updateErr,
		deleteErr:              deleteErr,
		hasAccessErr:           hasAccessErr,
	}
}

func (m *MockMachineAccessRepository) createKey(machineID, userID string) string {
	return machineID + "|" + userID
}

func (m *MockMachineAccessRepository) Create(access *models.MachineAccess) error {
	if m.createErr != nil {
		return m.createErr
	}
	key := m.createKey(access.MachineID, access.UserID)
	m.accesses[key] = &MockMachineAccessEntry{
		machineID:  access.MachineID,
		userID:     access.UserID,
		permission: access.Permission,
		grantedAt:  access.GrantedAt,
	}
	return nil
}

func (m *MockMachineAccessRepository) GetByMachineAndUser(machineID, userID string) (*models.MachineAccess, error) {
	if m.getByMachineAndUserErr != nil {
		return nil, m.getByMachineAndUserErr
	}
	key := m.createKey(machineID, userID)
	entry, exists := m.accesses[key]
	if !exists {
		return nil, nil
	}
	return &models.MachineAccess{
		MachineID:  entry.machineID,
		UserID:     entry.userID,
		Permission: entry.permission,
		GrantedAt:  entry.grantedAt,
	}, nil
}

func (m *MockMachineAccessRepository) ListByMachine(machineID string) ([]*models.MachineAccess, error) {
	if m.listByMachineErr != nil {
		return nil, m.listByMachineErr
	}
	var result []*models.MachineAccess
	for _, entry := range m.accesses {
		if entry.machineID == machineID {
			result = append(result, &models.MachineAccess{
				MachineID:  entry.machineID,
				UserID:     entry.userID,
				Permission: entry.permission,
				GrantedAt:  entry.grantedAt,
			})
		}
	}
	return result, nil
}

func (m *MockMachineAccessRepository) ListByUser(userID string) ([]*models.MachineAccess, error) {
	if m.listByUserErr != nil {
		return nil, m.listByUserErr
	}
	var result []*models.MachineAccess
	for _, entry := range m.accesses {
		if entry.userID == userID {
			result = append(result, &models.MachineAccess{
				MachineID:  entry.machineID,
				UserID:     entry.userID,
				Permission: entry.permission,
				GrantedAt:  entry.grantedAt,
			})
		}
	}
	return result, nil
}

func (m *MockMachineAccessRepository) Update(access *models.MachineAccess) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	key := m.createKey(access.MachineID, access.UserID)
	m.accesses[key] = &MockMachineAccessEntry{
		machineID:  access.MachineID,
		userID:     access.UserID,
		permission: access.Permission,
		grantedAt:  access.GrantedAt,
	}
	return nil
}

func (m *MockMachineAccessRepository) Delete(machineID, userID string) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	key := m.createKey(machineID, userID)
	delete(m.accesses, key)
	return nil
}

func (m *MockMachineAccessRepository) HasAccess(machineID, userID string) (bool, error) {
	if m.hasAccessErr != nil {
		return false, m.hasAccessErr
	}
	key := m.createKey(machineID, userID)
	_, exists := m.accesses[key]
	return exists, nil
}

// GetPermission returns the stored permission level for the pair, or
// "" when no entry exists — mirroring the repository's no-row contract
// that models.HasAtLeast consumes.
func (m *MockMachineAccessRepository) GetPermission(machineID, userID string) (string, error) {
	if m.getPermissionErr != nil {
		return "", m.getPermissionErr
	}
	key := m.createKey(machineID, userID)
	entry, exists := m.accesses[key]
	if !exists {
		return "", nil
	}
	return entry.permission, nil
}

// WithGetPermissionError returns the same repository with GetPermission
// configured to fail, for tests injecting a permission-lookup storage
// failure (mirrors MockSessionRepository.WithRotateError).
func (m *MockMachineAccessRepository) WithGetPermissionError(err error) *MockMachineAccessRepository {
	m.getPermissionErr = err
	return m
}

// AddAccess adds access directly without going through Create
func (m *MockMachineAccessRepository) AddAccess(machineID, userID, permission string, grantedAt time.Time) {
	key := m.createKey(machineID, userID)
	m.accesses[key] = &MockMachineAccessEntry{
		machineID:  machineID,
		userID:     userID,
		permission: permission,
		grantedAt:  grantedAt,
	}
}
