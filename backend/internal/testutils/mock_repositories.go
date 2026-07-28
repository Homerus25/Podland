package testutils

import (
	"context"
	"time"

	"github.com/yourorg/podland/backend/internal/models"
)

// MockUserRepository implements a mock user repository for testing
type MockUserRepository struct {
	users        map[string]*models.User
	createErr    error
	getByIDErr   error
	getByNameErr error
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
	return user, nil
}

func (m *MockUserRepository) GetByUsername(username string) (*models.User, error) {
	if m.getByNameErr != nil {
		return nil, m.getByNameErr
	}
	for _, user := range m.users {
		if user.Username == username {
			return user, nil
		}
	}
	return nil, nil
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
	machines  map[string]*models.Machine
	listErr   error
	getErr    error
	createErr error
	updateErr error
	deleteErr error
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
	m.machines[machine.ID] = machine
	return nil
}

func (m *MockMachineRepository) Delete(id string) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	delete(m.machines, id)
	return nil
}

func (m *MockMachineRepository) ListByOwner(ownerID string) ([]*models.Machine, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	var result []*models.Machine
	for _, machine := range m.machines {
		if machine.OwnerID == ownerID {
			result = append(result, machine)
		}
	}
	return result, nil
}

func (m *MockMachineRepository) ListAccessible(userID string) ([]*models.Machine, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	// For simplicity, return all machines - in real tests you can customize this behavior
	var result []*models.Machine
	for _, machine := range m.machines {
		if machine.OwnerID == userID {
			result = append(result, machine)
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

// MockConnectionCache implements a mock connection cache for testing
// It simulates the ConnectionCache from connections/cache.go but with simplified interface
type MockConnectionCache struct {
	connections     map[string]any
	getOrCreateErr  error
	getOrCreateFunc func(ctx context.Context, machineID, userID string) (any, error)
}

func NewMockConnectionCache() *MockConnectionCache {
	return &MockConnectionCache{
		connections: make(map[string]any),
	}
}

func NewMockConnectionCacheWithError(getOrCreateErr error) *MockConnectionCache {
	return &MockConnectionCache{
		connections:    make(map[string]any),
		getOrCreateErr: getOrCreateErr,
	}
}

// SetCustomGetOrCreate sets a custom function for GetOrCreate
func (m *MockConnectionCache) SetCustomGetOrCreate(getOrCreateFunc func(ctx context.Context, machineID, userID string) (any, error)) {
	m.getOrCreateFunc = getOrCreateFunc
}

// GetOrCreate simulates the ConnectionCache.GetOrCreate method
func (m *MockConnectionCache) GetOrCreate(ctx context.Context, machineID, userID string) (any, error) {
	if m.getOrCreateErr != nil {
		return nil, m.getOrCreateErr
	}
	if m.getOrCreateFunc != nil {
		return m.getOrCreateFunc(ctx, machineID, userID)
	}

	// Default mock behavior - return a simple mock connection
	mockConn := &MockPodmanConnection{MachineID: machineID}
	m.connections[machineID] = mockConn
	return mockConn, nil
}

// Release simulates releasing a connection
func (m *MockConnectionCache) Release(machineID string) {
	delete(m.connections, machineID)
}

// MarkActive marks a connection as active
func (m *MockConnectionCache) MarkActive(machineID string) {}

// MarkIdle marks a connection as idle
func (m *MockConnectionCache) MarkIdle(machineID string) {}

// Clear removes all connections
func (m *MockConnectionCache) Clear() {
	m.connections = make(map[string]any)
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
