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

type mockQuadletLister struct {
	quadlets []*entities.ListQuadlet
	err      error
}

func (m *mockQuadletLister) QuadletList(ctx context.Context, opts entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.quadlets, nil
}

type mockQuadletPrinter struct {
	content string
	err     error
}

func (m *mockQuadletPrinter) QuadletPrint(ctx context.Context, name string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.content, nil
}

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

			mockQuadletLister := &mockQuadletLister{
				quadlets: tt.mockQuadlets,
			}

			mockQuadletPrinter := &mockQuadletPrinter{
				content: "",
			}

			service := NewQuadletService(mockConnManager, mockQuadletLister, mockQuadletPrinter)

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

func TestQuadletService_Get(t *testing.T) {
	tests := []struct {
		name           string
		userID         string
		machineID      string
		quadletName    string
		mockQuadlets   []*entities.ListQuadlet
		mockContent    string
		mockError      error
		expectedResult *QuadletGetResponse
		expectedError  string
	}{
		{
			name:        "successful get",
			userID:      "user123",
			machineID:   "machine123",
			quadletName: "my-container.container",
			mockQuadlets: []*entities.ListQuadlet{
				{
					Name:     "my-container.container",
					UnitName: "my-container.service",
					Path:     "/etc/containers/systemd/my-container.container",
					Status:   "running (12s ago)",
				},
			},
			mockContent: "[Unit]\nDescription=My Container\n\n[Container]\nImage=nginx:latest\n",
			expectedResult: &QuadletGetResponse{
				ID:       "my-container.container",
				Name:     "my-container.container",
				Path:     "/etc/containers/systemd/my-container.container",
				Unit:     "my-container.service",
				Resource: "container",
				State:    "running",
				Content:  "[Unit]\nDescription=My Container\n\n[Container]\nImage=nginx:latest\n",
			},
		},
		{
			name:           "quadlet not found",
			userID:         "user123",
			machineID:      "machine123",
			quadletName:    "nonexistent.container",
			mockQuadlets:   []*entities.ListQuadlet{},
			mockContent:    "",
			expectedResult: nil,
			expectedError:  "QUADLET_NOT_FOUND",
		},
		{
			name:        "quadlet not found - name mismatch",
			userID:      "user123",
			machineID:   "machine123",
			quadletName: "different.container",
			mockQuadlets: []*entities.ListQuadlet{
				{
					Name:     "my-container.container",
					UnitName: "my-container.service",
					Path:     "/etc/containers/systemd/my-container.container",
					Status:   "running",
				},
			},
			mockContent:    "",
			expectedResult: nil,
			expectedError:  "QUADLET_NOT_FOUND",
		},
		{
			name:           "connection failed",
			userID:         "user123",
			machineID:      "machine123",
			quadletName:    "my-container.container",
			mockError:      errors.New("connection failed"),
			expectedResult: nil,
			expectedError:  "CONNECTION_FAILED",
		},
		{
			name:        "pod resource type",
			userID:      "user123",
			machineID:   "machine123",
			quadletName: "my-pod.pod",
			mockQuadlets: []*entities.ListQuadlet{
				{
					Name:     "my-pod.pod",
					UnitName: "my-pod.service",
					Path:     "/etc/containers/systemd/my-pod.pod",
					Status:   "stopped",
				},
			},
			mockContent: "[Unit]\nDescription=My Pod\n\n[Pod]\nPodmanArgs=--name=my-pod\n",
			expectedResult: &QuadletGetResponse{
				ID:       "my-pod.pod",
				Name:     "my-pod.pod",
				Path:     "/etc/containers/systemd/my-pod.pod",
				Unit:     "my-pod.service",
				Resource: "pod",
				State:    "stopped",
				Content:  "[Unit]\nDescription=My Pod\n\n[Pod]\nPodmanArgs=--name=my-pod\n",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConnManager := &mockConnectionManager{
				connCtx: context.Background(),
				err:     tt.mockError,
			}

			mockQuadletLister := &mockQuadletLister{
				quadlets: tt.mockQuadlets,
			}

			mockQuadletPrinter := &mockQuadletPrinter{
				content: tt.mockContent,
			}

			service := NewQuadletService(mockConnManager, mockQuadletLister, mockQuadletPrinter)

			result, err := service.Get(context.Background(), tt.userID, tt.machineID, tt.quadletName)

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

			if result == nil {
				t.Errorf("expected result, got nil")
				return
			}

			if result.ID != tt.expectedResult.ID {
				t.Errorf("result.ID = %q, want %q", result.ID, tt.expectedResult.ID)
			}
			if result.Name != tt.expectedResult.Name {
				t.Errorf("result.Name = %q, want %q", result.Name, tt.expectedResult.Name)
			}
			if result.Path != tt.expectedResult.Path {
				t.Errorf("result.Path = %q, want %q", result.Path, tt.expectedResult.Path)
			}
			if result.Unit != tt.expectedResult.Unit {
				t.Errorf("result.Unit = %q, want %q", result.Unit, tt.expectedResult.Unit)
			}
			if result.Resource != tt.expectedResult.Resource {
				t.Errorf("result.Resource = %q, want %q", result.Resource, tt.expectedResult.Resource)
			}
			if result.State != tt.expectedResult.State {
				t.Errorf("result.State = %q, want %q", result.State, tt.expectedResult.State)
			}
			if result.Content != tt.expectedResult.Content {
				t.Errorf("result.Content = %q, want %q", result.Content, tt.expectedResult.Content)
			}
		})
	}
}