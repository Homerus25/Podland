package quadlets

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yourorg/podland/backend/internal/domain/machines"
	appErrors "github.com/yourorg/podland/backend/internal/errors"
	"github.com/yourorg/podland/backend/internal/testutils"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

// TestQuadletService_ImplementsInterface is a compile-time check that
// *QuadletService satisfies QuadletServiceInterface.
func TestQuadletService_ImplementsInterface(t *testing.T) {
	var _ QuadletServiceInterface = (*QuadletService)(nil)
}

// TestConcreteConnectionManager_ImplementsInterface is a compile-time check
// that concreteConnectionManager satisfies ConnectionManagerInterface.
func TestConcreteConnectionManager_ImplementsInterface(t *testing.T) {
	var _ machines.ConnectionManagerInterface = (*concreteConnectionManager)(nil)
}

// TestNewQuadletService_WithMockConnManager verifies that QuadletService can be
// constructed with a mock ConnectionManagerInterface, enabling unit testing.
func TestNewQuadletService_WithMockConnManager(t *testing.T) {
	var mockConnManager machines.ConnectionManagerInterface = testutils.NewMockConnectionManager()

	service := NewQuadletService(mockConnManager)

	assert.NotNil(t, service)
	assert.NotNil(t, service.connManager)
}

// --- Service seam: List ---
//
// The podman engine boundary is no longer mocked (it is built inline, matching
// the images/containers convention). Service-level tests therefore cover only
// connection-stage error wrapping and the invocation shape: the call reaches the
// engine step but fails because there is no real podman backend behind the mock
// connection context. The domain logic (filtering, mapping, state/resource
// derivation) is covered directly on the pure functions further below.

func TestQuadletService_List_ConnectionFailure(t *testing.T) {
	ctx := context.Background()
	connErr := errors.New("SSH connection failed")
	var mockConnManager machines.ConnectionManagerInterface = testutils.NewMockConnectionManagerWithError(connErr)
	service := NewQuadletService(mockConnManager)

	result, err := service.List(ctx, "test-user", "test-machine", ListFilters{})

	assert.Error(t, err, "Should return error when connection fails")
	assert.Equal(t, err, appErrors.ErrConnectionFailed, "Should map to ErrConnectionFailed")
	assert.Nil(t, result, "Should return nil result when connection fails")
}

func TestQuadletService_List_InvocationShape(t *testing.T) {
	ctx := context.Background()
	var mockConnManager machines.ConnectionManagerInterface = testutils.NewMockConnectionManager()
	service := NewQuadletService(mockConnManager)

	result, err := service.List(ctx, "test-user", "test-machine", ListFilters{})

	// The mock connection succeeds but the engine call fails because there is
	// no real podman backend behind the mock connection context.
	assert.Error(t, err, "Should return error from engine call with mock connection")
	assert.Nil(t, result, "Should return nil result when engine call fails")
	assert.NotContains(t, err.Error(), "CONNECTION_FAILED",
		"Error should not be a connection error — connection should have succeeded")
}

// --- Service seam: Get ---

func TestQuadletService_Get_ConnectionFailure(t *testing.T) {
	ctx := context.Background()
	connErr := errors.New("SSH connection failed")
	var mockConnManager machines.ConnectionManagerInterface = testutils.NewMockConnectionManagerWithError(connErr)
	service := NewQuadletService(mockConnManager)

	result, err := service.Get(ctx, "test-user", "test-machine", "my-container.container")

	assert.Error(t, err, "Should return error when connection fails")
	assert.Equal(t, err, appErrors.ErrConnectionFailed, "Should map to ErrConnectionFailed")
	assert.Nil(t, result, "Should return nil result when connection fails")
}

func TestQuadletService_Get_InvocationShape(t *testing.T) {
	ctx := context.Background()
	var mockConnManager machines.ConnectionManagerInterface = testutils.NewMockConnectionManager()
	service := NewQuadletService(mockConnManager)

	result, err := service.Get(ctx, "test-user", "test-machine", "my-container.container")

	// The mock connection succeeds but the engine call fails because there is
	// no real podman backend behind the mock connection context.
	assert.Error(t, err, "Should return error from engine call with mock connection")
	assert.Nil(t, result, "Should return nil result when engine call fails")
	assert.NotContains(t, err.Error(), "CONNECTION_FAILED",
		"Error should not be a connection error — connection should have succeeded")
}

// --- Internal seams: pure domain logic ---
//
// These functions are the part of the implementation that carries real domain
// rules (resource derivation from file suffix, systemd-status normalization,
// and filtering). They are tested directly through their own surface rather than
// through the service, so coverage of the meaningful logic is preserved.

func TestApplyFilters(t *testing.T) {
	quadlets := []QuadletListResponse{
		{ID: "a.container", Name: "alpha.container", Resource: "container", State: "running"},
		{ID: "b.pod", Name: "bravo.pod", Resource: "pod", State: "stopped"},
		{ID: "c.container", Name: "charlie.container", Resource: "container", State: "error"},
	}

	t.Run("no filters returns all", func(t *testing.T) {
		got := applyFilters(quadlets, ListFilters{})
		assert.Len(t, got, 3)
	})

	t.Run("filter by resource", func(t *testing.T) {
		got := applyFilters(quadlets, ListFilters{Resource: "container"})
		assert.Len(t, got, 2)
		for _, q := range got {
			assert.Equal(t, "container", q.Resource)
		}
	})

	t.Run("filter by name is case-insensitive substring", func(t *testing.T) {
		got := applyFilters(quadlets, ListFilters{Name: "POD"})
		assert.Len(t, got, 1)
		assert.Equal(t, "bravo.pod", got[0].Name)
	})

	t.Run("resource and name combined", func(t *testing.T) {
		got := applyFilters(quadlets, ListFilters{Resource: "container", Name: "char"})
		assert.Len(t, got, 1)
		assert.Equal(t, "charlie.container", got[0].Name)
	})

	t.Run("no match returns empty non-nil", func(t *testing.T) {
		got := applyFilters(quadlets, ListFilters{Name: "zzz"})
		assert.Len(t, got, 0)
	})

	t.Run("empty input returns empty", func(t *testing.T) {
		got := applyFilters([]QuadletListResponse{}, ListFilters{Resource: "container"})
		assert.Len(t, got, 0)
	})
}

func TestFromListQuadlet(t *testing.T) {
	t.Run("nil returns zero value", func(t *testing.T) {
		got := FromListQuadlet(nil)
		assert.Equal(t, QuadletListResponse{}, got)
	})

	t.Run("maps fields and derives resource/state", func(t *testing.T) {
		got := FromListQuadlet(&entities.ListQuadlet{
			Name:     "my-container.container",
			UnitName: "my-container.service",
			Path:     "/etc/containers/systemd/my-container.container",
			Status:   "running (12s ago)",
		})
		assert.Equal(t, QuadletListResponse{
			ID:       "my-container.container",
			Name:     "my-container.container",
			Path:     "/etc/containers/systemd/my-container.container",
			Unit:     "my-container.service",
			Resource: "container",
			State:    "running",
			Raw:      "",
		}, got)
	})
}

func TestToQuadletListResponse(t *testing.T) {
	t.Run("empty slice", func(t *testing.T) {
		got := ToQuadletListResponse([]*entities.ListQuadlet{})
		assert.Len(t, got, 0)
	})

	t.Run("maps each element preserving order", func(t *testing.T) {
		in := []*entities.ListQuadlet{
			{Name: "a.container", UnitName: "a.service", Path: "/x/a.container", Status: "running"},
			{Name: "b.pod", UnitName: "b.service", Path: "/x/b.pod", Status: "stopped"},
		}
		got := ToQuadletListResponse(in)
		assert.Len(t, got, 2)
		assert.Equal(t, "a.container", got[0].Name)
		assert.Equal(t, "container", got[0].Resource)
		assert.Equal(t, "running", got[0].State)
		assert.Equal(t, "b.pod", got[1].Name)
		assert.Equal(t, "pod", got[1].Resource)
		assert.Equal(t, "stopped", got[1].State)
	})
}

func TestDeriveResourceFromPath(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/etc/containers/systemd/web.container", "container"},
		{"/etc/containers/systemd/grp.pod", "pod"},
		{"/etc/containers/systemd/img.image", "image"},
		{"/etc/containers/systemd/dat.volume", "volume"},
		{"/etc/containers/systemd/net.network", "network"},
		{"/etc/containers/systemd/k8s.kube", "container"}, // kube -> container
		{"/etc/containers/systemd/bld.build", "image"},    // build -> image
		{"/etc/containers/systemd/UPPER.CONTAINER", "container"}, // case-insensitive
		{"/etc/containers/systemd/weird.txt", "unknown"},
		{"", "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			assert.Equal(t, tt.want, deriveResourceFromPath(tt.path))
		})
	}
}

func TestParseStateFromStatus(t *testing.T) {
	tests := []struct {
		status string
		want   string
	}{
		{"running (12s ago)", "running"},
		{"STOPPED", "stopped"},
		{"active (running) since...", "running"},
		{"failed", "error"},
		{"inactive (error)", "error"},
		{"activating", "unknown"},
		{"", "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			assert.Equal(t, tt.want, parseStateFromStatus(tt.status))
		})
	}
}
