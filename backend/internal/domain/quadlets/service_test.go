package quadlets

import (
	"context"
	"errors"
	"testing"

	"github.com/yourorg/podland/backend/internal/domain/machines"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

type mockConnectionManager struct {
	connCtx context.Context
	err     error
}

func (m *mockConnectionManager) GetConnection(ctx context.Context, machineID, userID string) (any, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.connCtx, nil
}

func (m *mockConnectionManager) Release(machineID string) {}

var _ machines.ConnectionManagerInterface = (*mockConnectionManager)(nil)

func TestQuadletService_List(t *testing.T) {
	tests := []struct {
		name           string
		userID         string
		machineID      string
		filters        ListFilters
		mockQuadlets   []*entities.ListQuadlet
		mockError      error
		expectedResult []QuadletListResponse
		expectedError  string
	}{
		{
			name:      "successful list",
			userID:    "user123",
			machineID: "machine123",
			filters:   ListFilters{},
			mockQuadlets: []*entities.ListQuadlet{
				{
					Name:     "my-container.container",
					UnitName: "my-container.service",
					Path:     "/etc/containers/systemd/my-container.container",
					Status:   "running (12s ago)",
				},
				{
					Name:     "my-pod.pod",
					UnitName: "my-pod.service",
					Path:     "/etc/containers/systemd/my-pod.pod",
					Status:   "stopped",
				},
			},
			expectedResult: []QuadletListResponse{
				{
					ID:       "my-container.container",
					Name:     "my-container.container",
					Path:     "/etc/containers/systemd/my-container.container",
					Unit:     "my-container.service",
					Resource: "container",
					State:    "running",
				},
				{
					ID:       "my-pod.pod",
					Name:     "my-pod.pod",
					Path:     "/etc/containers/systemd/my-pod.pod",
					Unit:     "my-pod.service",
					Resource: "pod",
					State:    "stopped",
				},
			},
		},
		{
			name:      "filter by resource",
			userID:    "user123",
			machineID: "machine123",
			filters:   ListFilters{Resource: "container"},
			mockQuadlets: []*entities.ListQuadlet{
				{
					Name:     "my-container.container",
					UnitName: "my-container.service",
					Path:     "/etc/containers/systemd/my-container.container",
					Status:   "running",
				},
				{
					Name:     "my-pod.pod",
					UnitName: "my-pod.service",
					Path:     "/etc/containers/systemd/my-pod.pod",
					Status:   "stopped",
				},
			},
			expectedResult: []QuadletListResponse{
				{
					ID:       "my-container.container",
					Name:     "my-container.container",
					Path:     "/etc/containers/systemd/my-container.container",
					Unit:     "my-container.service",
					Resource: "container",
					State:    "running",
				},
			},
		},
		{
			name:      "filter by name",
			userID:    "user123",
			machineID: "machine123",
			filters:   ListFilters{Name: "pod"},
			mockQuadlets: []*entities.ListQuadlet{
				{
					Name:     "my-container.container",
					UnitName: "my-container.service",
					Path:     "/etc/containers/systemd/my-container.container",
					Status:   "running",
				},
				{
					Name:     "my-pod.pod",
					UnitName: "my-pod.service",
					Path:     "/etc/containers/systemd/my-pod.pod",
					Status:   "stopped",
				},
			},
			expectedResult: []QuadletListResponse{
				{
					ID:       "my-pod.pod",
					Name:     "my-pod.pod",
					Path:     "/etc/containers/systemd/my-pod.pod",
					Unit:     "my-pod.service",
					Resource: "pod",
					State:    "stopped",
				},
			},
		},
		{
			name:           "connection failed",
			userID:         "user123",
			machineID:      "machine123",
			filters:        ListFilters{},
			mockError:      errors.New("connection failed"),
			expectedResult: nil,
			expectedError:  "CONNECTION_FAILED",
		},
		{
			name:      "empty list",
			userID:    "user123",
			machineID: "machine123",
			filters:   ListFilters{},
			mockQuadlets: []*entities.ListQuadlet{},
			expectedResult: []QuadletListResponse{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConnManager := &mockConnectionManager{
				connCtx: context.Background(),
				err:     tt.mockError,
			}

			service := NewQuadletService(mockConnManager)

			if tt.mockError == nil {
				originalQuadletList := QuadletListFunc
				QuadletListFunc = func(ctx context.Context, opts entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
					return tt.mockQuadlets, nil
				}
				defer func() {
					QuadletListFunc = originalQuadletList
				}()
			}

			result, err := service.List(context.Background(), tt.userID, tt.machineID, tt.filters)

			if tt.expectedError != "" {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.expectedError)
					return
				}
				if err.Error() == "" || !containsString(err.Error(), tt.expectedError) {
					t.Errorf("expected error containing %q, got %q", tt.expectedError, err.Error())
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if len(result) != len(tt.expectedResult) {
				t.Errorf("expected %d results, got %d", len(tt.expectedResult), len(result))
				return
			}

			for i, expected := range tt.expectedResult {
				if result[i].ID != expected.ID {
					t.Errorf("result[%d].ID = %q, want %q", i, result[i].ID, expected.ID)
				}
				if result[i].Name != expected.Name {
					t.Errorf("result[%d].Name = %q, want %q", i, result[i].Name, expected.Name)
				}
				if result[i].Resource != expected.Resource {
					t.Errorf("result[%d].Resource = %q, want %q", i, result[i].Resource, expected.Resource)
				}
				if result[i].State != expected.State {
					t.Errorf("result[%d].State = %q, want %q", i, result[i].State, expected.State)
				}
			}
		})
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}