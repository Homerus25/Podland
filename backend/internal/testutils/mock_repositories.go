package testutils

import (
	"github.com/yourorg/podland/backend/internal/models"
)

// MockUserRepository implements a mock user repository for testing
type MockUserRepository struct {
	users map[string]*models.User
}

func NewMockUserRepository() *MockUserRepository {
	return &MockUserRepository{
		users: make(map[string]*models.User),
	}
}

func (m *MockUserRepository) Create(user *models.User) error {
	m.users[user.ID] = user
	return nil
}

func (m *MockUserRepository) GetByID(id string) (*models.User, error) {
	user, exists := m.users[id]
	if !exists {
		return nil, nil // or return an error if preferred
	}
	return user, nil
}

func (m *MockUserRepository) GetByUsername(username string) (*models.User, error) {
	// Simple implementation - return first user with matching username
	for _, user := range m.users {
		if user.Username == username {
			return user, nil
		}
	}
	return nil, nil
}

func (m *MockUserRepository) Update(user *models.User) error {
	m.users[user.ID] = user
	return nil
}

func (m *MockUserRepository) Delete(id string) error {
	delete(m.users, id)
	return nil
}

func (m *MockUserRepository) UsernameExists(username string) (bool, error) {
	for _, user := range m.users {
		if user.Username == username {
			return true, nil
		}
	}
	return false, nil
}

// MockMachineRepository implements a mock machine repository for testing
type MockMachineRepository struct {
	machines map[string]*models.Machine
}

func NewMockMachineRepository() *MockMachineRepository {
	return &MockMachineRepository{
		machines: make(map[string]*models.Machine),
	}
}

func (m *MockMachineRepository) Create(machine *models.Machine) error {
	m.machines[machine.ID] = machine
	return nil
}

func (m *MockMachineRepository) GetByID(id string) (*models.Machine, error) {
	machine, exists := m.machines[id]
	if !exists {
		return nil, nil
	}
	return machine, nil
}

func (m *MockMachineRepository) GetByName(name string) (*models.Machine, error) {
	for _, machine := range m.machines {
		if machine.Name == name {
			return machine, nil
		}
	}
	return nil, nil
}

func (m *MockMachineRepository) List() ([]*models.Machine, error) {
	var result []*models.Machine
	for _, machine := range m.machines {
		result = append(result, machine)
	}
	return result, nil
}

func (m *MockMachineRepository) Update(machine *models.Machine) error {
	m.machines[machine.ID] = machine
	return nil
}

func (m *MockMachineRepository) Delete(id string) error {
	delete(m.machines, id)
	return nil
}

func (m *MockMachineRepository) NameExists(name string) (bool, error) {
	for _, machine := range m.machines {
		if machine.Name == name {
			return true, nil
		}
	}
	return false, nil
}

// MockConnectionCache implements a mock connection cache for testing
type MockConnectionCache struct {
	connections map[string]interface{}
}

func NewMockConnectionCache() *MockConnectionCache {
	return &MockConnectionCache{
		connections: make(map[string]interface{}),
	}
}

func (m *MockConnectionCache) Get(key string) (interface{}, bool) {
	conn, exists := m.connections[key]
	return conn, exists
}

func (m *MockConnectionCache) Set(key string, value interface{}) {
	m.connections[key] = value
}

func (m *MockConnectionCache) Delete(key string) {
	delete(m.connections, key)
}

func (m *MockConnectionCache) Clear() {
	m.connections = make(map[string]interface{})
}