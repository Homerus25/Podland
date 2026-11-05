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

// Count returns the number of stored users, mirroring the real
// repository's contract (first-run setup derives "needs setup" from it).
func (m *MockUserRepository) Count() (int, error) {
	return len(m.users), nil
}

// CreateFirstUser inserts user only when the mock is empty, mirroring
// the real repository's atomic first-user guard: it reports whether the
// insert happened, and false (nil error) means a user already existed.
func (m *MockUserRepository) CreateFirstUser(user *models.User) (bool, error) {
	if len(m.users) > 0 {
		return false, nil
	}
	m.users[user.ID] = user
	return true, nil
}

// MockHostRepository implements a mock host repository for testing
type MockHostRepository struct {
	hosts       map[string]*models.Host
	grants      []models.AccessibleHost
	listErr     error
	getErr      error
	createErr   error
	updateErr   error
	deleteErr   error
	updateCalls []string
}

func NewMockHostRepository() *MockHostRepository {
	return &MockHostRepository{
		hosts: make(map[string]*models.Host),
	}
}

// NewMockHostRepositoryWithErrors creates a mock repository with configurable errors
func NewMockHostRepositoryWithErrors(listErr, getErr, createErr, updateErr, deleteErr error) *MockHostRepository {
	return &MockHostRepository{
		hosts:     make(map[string]*models.Host),
		listErr:   listErr,
		getErr:    getErr,
		createErr: createErr,
		updateErr: updateErr,
		deleteErr: deleteErr,
	}
}

// AddHost adds a host directly to the mock repository
func (m *MockHostRepository) AddHost(host *models.Host) {
	m.hosts[host.ID] = host
}

func (m *MockHostRepository) Create(host *models.Host) error {
	if m.createErr != nil {
		return m.createErr
	}
	m.hosts[host.ID] = host
	return nil
}

func (m *MockHostRepository) GetByID(id string) (*models.Host, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	host, exists := m.hosts[id]
	if !exists {
		return nil, nil
	}
	return host, nil
}

func (m *MockHostRepository) Update(host *models.Host) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	m.updateCalls = append(m.updateCalls, host.ID)
	m.hosts[host.ID] = host
	return nil
}

// UpdateCalls returns a snapshot of every successful Update invocation's
// host IDs, in call order, so service tests can pin that a failed
// operation (e.g. a failed update probe) wrote nothing.
func (m *MockHostRepository) UpdateCalls() []string {
	out := make([]string, len(m.updateCalls))
	copy(out, m.updateCalls)
	return out
}

func (m *MockHostRepository) Delete(id string) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	delete(m.hosts, id)
	return nil
}

// AddAccessibleHost registers a host the caller reaches through a
// grant rather than ownership; ListAccessible returns it verbatim (the
// granted permission included) after the owned hosts.
func (m *MockHostRepository) AddAccessibleHost(accessible models.AccessibleHost) {
	m.grants = append(m.grants, accessible)
}

func (m *MockHostRepository) ListAccessible(userID string) ([]models.AccessibleHost, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	// Owned hosts carry the admin permission, mirroring the access
	// row Create grants; hosts registered through
	// AddAccessibleHost stand in for the granted rows the real JOIN
	// also returns.
	var result []models.AccessibleHost
	for _, host := range m.hosts {
		if host.OwnerID == userID {
			result = append(result, models.AccessibleHost{Host: host, Permission: models.PermissionAdmin})
		}
	}
	return append(result, m.grants...), nil
}

func (m *MockHostRepository) NameExistsForUser(userID, name string) (bool, error) {
	for _, host := range m.hosts {
		if host.OwnerID == userID && host.Name == name {
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

// MockHostAccessRepository implements a mock host access repository for testing
type MockHostAccessRepository struct {
	accesses            map[string]*MockHostAccessEntry
	createErr           error
	getByHostAndUserErr error
	listByHostErr       error
	listByUserErr       error
	updateErr           error
	deleteErr           error
	hasAccessErr        error
	getPermissionErr    error
}

// MockHostAccessEntry represents a mock host access entry
// Using hostID+userID as composite key
type MockHostAccessEntry struct {
	hostID     string
	userID     string
	permission string
	grantedAt  time.Time
}

func NewMockHostAccessRepository() *MockHostAccessRepository {
	return &MockHostAccessRepository{
		accesses: make(map[string]*MockHostAccessEntry),
	}
}

// NewMockHostAccessRepositoryWithErrors creates a mock access repository with configurable errors
func NewMockHostAccessRepositoryWithErrors(createErr, getByHostAndUserErr, listByHostErr, listByUserErr, updateErr, deleteErr, hasAccessErr error) *MockHostAccessRepository {
	return &MockHostAccessRepository{
		accesses:            make(map[string]*MockHostAccessEntry),
		createErr:           createErr,
		getByHostAndUserErr: getByHostAndUserErr,
		listByHostErr:       listByHostErr,
		listByUserErr:       listByUserErr,
		updateErr:           updateErr,
		deleteErr:           deleteErr,
		hasAccessErr:        hasAccessErr,
	}
}

func (m *MockHostAccessRepository) createKey(hostID, userID string) string {
	return hostID + "|" + userID
}

func (m *MockHostAccessRepository) Create(access *models.HostAccess) error {
	if m.createErr != nil {
		return m.createErr
	}
	key := m.createKey(access.HostID, access.UserID)
	m.accesses[key] = &MockHostAccessEntry{
		hostID:     access.HostID,
		userID:     access.UserID,
		permission: access.Permission,
		grantedAt:  access.GrantedAt,
	}
	return nil
}

func (m *MockHostAccessRepository) GetByHostAndUser(hostID, userID string) (*models.HostAccess, error) {
	if m.getByHostAndUserErr != nil {
		return nil, m.getByHostAndUserErr
	}
	key := m.createKey(hostID, userID)
	entry, exists := m.accesses[key]
	if !exists {
		return nil, nil
	}
	return &models.HostAccess{
		HostID:     entry.hostID,
		UserID:     entry.userID,
		Permission: entry.permission,
		GrantedAt:  entry.grantedAt,
	}, nil
}

func (m *MockHostAccessRepository) ListByHost(hostID string) ([]*models.HostAccess, error) {
	if m.listByHostErr != nil {
		return nil, m.listByHostErr
	}
	var result []*models.HostAccess
	for _, entry := range m.accesses {
		if entry.hostID == hostID {
			result = append(result, &models.HostAccess{
				HostID:     entry.hostID,
				UserID:     entry.userID,
				Permission: entry.permission,
				GrantedAt:  entry.grantedAt,
			})
		}
	}
	return result, nil
}

func (m *MockHostAccessRepository) ListByUser(userID string) ([]*models.HostAccess, error) {
	if m.listByUserErr != nil {
		return nil, m.listByUserErr
	}
	var result []*models.HostAccess
	for _, entry := range m.accesses {
		if entry.userID == userID {
			result = append(result, &models.HostAccess{
				HostID:     entry.hostID,
				UserID:     entry.userID,
				Permission: entry.permission,
				GrantedAt:  entry.grantedAt,
			})
		}
	}
	return result, nil
}

func (m *MockHostAccessRepository) Update(access *models.HostAccess) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	key := m.createKey(access.HostID, access.UserID)
	m.accesses[key] = &MockHostAccessEntry{
		hostID:     access.HostID,
		userID:     access.UserID,
		permission: access.Permission,
		grantedAt:  access.GrantedAt,
	}
	return nil
}

func (m *MockHostAccessRepository) Delete(hostID, userID string) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	key := m.createKey(hostID, userID)
	delete(m.accesses, key)
	return nil
}

func (m *MockHostAccessRepository) HasAccess(hostID, userID string) (bool, error) {
	if m.hasAccessErr != nil {
		return false, m.hasAccessErr
	}
	key := m.createKey(hostID, userID)
	_, exists := m.accesses[key]
	return exists, nil
}

// GetPermission returns the stored permission level for the pair, or
// "" when no entry exists — mirroring the repository's no-row contract
// that models.HasAtLeast consumes.
func (m *MockHostAccessRepository) GetPermission(hostID, userID string) (string, error) {
	if m.getPermissionErr != nil {
		return "", m.getPermissionErr
	}
	key := m.createKey(hostID, userID)
	entry, exists := m.accesses[key]
	if !exists {
		return "", nil
	}
	return entry.permission, nil
}

// WithGetPermissionError returns the same repository with GetPermission
// configured to fail, for tests injecting a permission-lookup storage
// failure (mirrors MockSessionRepository.WithRotateError).
func (m *MockHostAccessRepository) WithGetPermissionError(err error) *MockHostAccessRepository {
	m.getPermissionErr = err
	return m
}

// AddAccess adds access directly without going through Create
func (m *MockHostAccessRepository) AddAccess(hostID, userID, permission string, grantedAt time.Time) {
	key := m.createKey(hostID, userID)
	m.accesses[key] = &MockHostAccessEntry{
		hostID:     hostID,
		userID:     userID,
		permission: permission,
		grantedAt:  grantedAt,
	}
}

// MockRevokedTokenRepository implements a mock access-token denylist for
// testing (issue 441). It records every Create so logout tests can pin
// which jti was denylisted with which expiry, and counts DeleteExpired
// calls for the login-purge pin.
type MockRevokedTokenRepository struct {
	revoked            map[string]time.Time
	createErr          error
	isRevokedErr       error
	deleteExpiredErr   error
	deleteExpiredCalls int
}

func NewMockRevokedTokenRepository() *MockRevokedTokenRepository {
	return &MockRevokedTokenRepository{
		revoked: make(map[string]time.Time),
	}
}

// WithCreateError returns the same repository with Create configured to
// fail, for tests injecting a denylist-write storage failure.
func (m *MockRevokedTokenRepository) WithCreateError(err error) *MockRevokedTokenRepository {
	m.createErr = err
	return m
}

// WithIsRevokedError returns the same repository with IsRevoked
// configured to fail, for tests injecting a denylist-lookup storage
// failure.
func (m *MockRevokedTokenRepository) WithIsRevokedError(err error) *MockRevokedTokenRepository {
	m.isRevokedErr = err
	return m
}

// WithDeleteExpiredError returns the same repository with DeleteExpired
// configured to fail, for tests injecting a purge-time storage failure.
func (m *MockRevokedTokenRepository) WithDeleteExpiredError(err error) *MockRevokedTokenRepository {
	m.deleteExpiredErr = err
	return m
}

// Revoked returns a snapshot of the denylist (jti → expiry), so tests can
// pin exactly which tokens logout revoked and until when.
func (m *MockRevokedTokenRepository) Revoked() map[string]time.Time {
	out := make(map[string]time.Time, len(m.revoked))
	for jti, expiresAt := range m.revoked {
		out[jti] = expiresAt
	}
	return out
}

// DeleteExpiredCalls reports how many times the lazy purge ran.
func (m *MockRevokedTokenRepository) DeleteExpiredCalls() int {
	return m.deleteExpiredCalls
}

func (m *MockRevokedTokenRepository) Create(jti string, expiresAt time.Time) error {
	if m.createErr != nil {
		return m.createErr
	}
	m.revoked[jti] = expiresAt
	return nil
}

func (m *MockRevokedTokenRepository) IsRevoked(jti string) (bool, error) {
	if m.isRevokedErr != nil {
		return false, m.isRevokedErr
	}
	_, exists := m.revoked[jti]
	return exists, nil
}

func (m *MockRevokedTokenRepository) DeleteExpired() error {
	if m.deleteExpiredErr != nil {
		return m.deleteExpiredErr
	}
	m.deleteExpiredCalls++
	now := time.Now().UTC()
	for jti, expiresAt := range m.revoked {
		if !expiresAt.After(now) {
			delete(m.revoked, jti)
		}
	}
	return nil
}
