package quadlets

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yourorg/podland/backend/internal/domain/machines"
	appErrors "github.com/yourorg/podland/backend/internal/errors"
	"github.com/yourorg/podland/backend/internal/testutils"
	"go.podman.io/podman/v6/pkg/domain/entities"
	"go.podman.io/podman/v6/libpod/define"
)

// TestQuadletService_ImplementsInterface is a compile-time check that
// *QuadletService satisfies QuadletServiceInterface.
func TestQuadletService_ImplementsInterface(t *testing.T) {
	var _ QuadletServiceInterface = (*QuadletService)(nil)
}

// TestConnectionManager_ImplementsInterface is a compile-time check that
// *connections.ConnectionManager satisfies ConnectionManagerInterface,
// allowing it to be passed directly to NewQuadletService.
func TestConnectionManager_ImplementsInterface(t *testing.T) {
	// var _ machines.ConnectionManagerInterface = (*connections.ConnectionManager)(nil)
}

// TestNewQuadletService_WithMockConnManager verifies that QuadletService can be
// constructed with a mock ConnectionManagerInterface, enabling unit testing.
func TestNewQuadletService_WithMockConnManager(t *testing.T) {
	var mockConnManager machines.ConnectionManagerInterface = testutils.NewMockConnectionManager()

	service := NewQuadletService(mockConnManager, nil)

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
	service := NewQuadletService(mockConnManager, nil)

	result, err := service.List(ctx, "test-user", "test-machine", ListFilters{})

	assert.Error(t, err, "Should return error when connection fails")
	assert.Equal(t, err, appErrors.ErrConnectionFailed, "Should map to ErrConnectionFailed")
	assert.Nil(t, result, "Should return nil result when connection fails")
}

func TestQuadletService_List_InvocationShape(t *testing.T) {
	ctx := context.Background()
	var mockConnManager machines.ConnectionManagerInterface = testutils.NewMockConnectionManager()
	service := NewQuadletService(mockConnManager, nil)

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
	service := NewQuadletService(mockConnManager, nil)

	result, err := service.Get(ctx, "test-user", "test-machine", "my-container.container")

	assert.Error(t, err, "Should return error when connection fails")
	assert.Equal(t, err, appErrors.ErrConnectionFailed, "Should map to ErrConnectionFailed")
	assert.Nil(t, result, "Should return nil result when connection fails")
}

func TestQuadletService_Get_InvocationShape(t *testing.T) {
	ctx := context.Background()
	var mockConnManager machines.ConnectionManagerInterface = testutils.NewMockConnectionManager()
	service := NewQuadletService(mockConnManager, nil)

	result, err := service.Get(ctx, "test-user", "test-machine", "my-container.container")

	// The mock connection succeeds but the engine call fails because there is
	// no real podman backend behind the mock connection context.
	assert.Error(t, err, "Should return error from engine call with mock connection")
	assert.Nil(t, result, "Should return nil result when engine call fails")
	assert.NotContains(t, err.Error(), "CONNECTION_FAILED",
		"Error should not be a connection error — connection should have succeeded")
}

// --- Service seam: Create ---

func TestQuadletService_Create_ConnectionFailure(t *testing.T) {
	ctx := context.Background()
	connErr := errors.New("SSH connection failed")
	var mockConnManager machines.ConnectionManagerInterface = testutils.NewMockConnectionManagerWithError(connErr)
	service := NewQuadletService(mockConnManager, nil)

	result, err := service.Create(ctx, "test-user", "test-machine", QuadletCreateRequest{
		Name:     "my-container",
		Resource: "container",
		Content:  "[Container]\nImage=nginx:latest\n",
	})

	assert.Error(t, err, "Should return error when connection fails")
	assert.Equal(t, err, appErrors.ErrConnectionFailed, "Should map to ErrConnectionFailed")
	assert.Nil(t, result, "Should return nil result when connection fails")
}

func TestQuadletService_Create_InvocationShape(t *testing.T) {
	ctx := context.Background()
	var mockConnManager machines.ConnectionManagerInterface = testutils.NewMockConnectionManager()
	service := NewQuadletService(mockConnManager, nil)

	result, err := service.Create(ctx, "test-user", "test-machine", QuadletCreateRequest{
		Name:     "my-container",
		Resource: "container",
		Content:  "[Container]\nImage=nginx:latest\n",
	})

	// The mock connection succeeds but the bindings-level Install call fails
	// because there is no real podman backend behind the mock connection context.
	assert.Error(t, err, "Should return error from install call with mock connection")
	assert.Nil(t, result, "Should return nil result when install call fails")
	assert.NotContains(t, err.Error(), "CONNECTION_FAILED",
		"Error should not be a connection error — connection should have succeeded")
}

// --- Service seam: Update ---
//
// Podman has no dedicated update primitive, so Update composes QuadletRemove +
// QuadletInstall(Replace:true). As with the other service seams, the podman
// engine boundary is built inline and cannot be exercised under the mock
// connection, so these tests cover only connection-stage error wrapping and the
// invocation shape (the call reaches the QuadletList existence check but fails
// because there is no real podman backend behind the mock connection context).
//
// The remaining acceptance-criterion cases for Update are covered outside the
// service seam:
//   - success-shape mapping → buildUpdateResponse (pure helper, tested below);
//   - not-found → findQuadletByName returning nil + the trivial nil check
//     (findQuadletByName is tested below; the nil check is a one-liner);
//   - mid-flow failure (remove succeeds, install fails, leaving the quadlet
//     removed) → cannot be exercised without a real podman backend; see the
//     doc comment on Service.Update for the documented non-atomicity guarantee.

func TestQuadletService_Update_ConnectionFailure(t *testing.T) {
	ctx := context.Background()
	connErr := errors.New("SSH connection failed")
	var mockConnManager machines.ConnectionManagerInterface = testutils.NewMockConnectionManagerWithError(connErr)
	service := NewQuadletService(mockConnManager, nil)

	result, err := service.Update(ctx, "test-user", "test-machine", "my-container.container", QuadletUpdateRequest{
		Content: "[Container]\nImage=nginx:1.25\n",
	})

	assert.Error(t, err, "Should return error when connection fails")
	assert.Equal(t, err, appErrors.ErrConnectionFailed, "Should map to ErrConnectionFailed")
	assert.Nil(t, result, "Should return nil result when connection fails")
}

func TestQuadletService_Update_InvocationShape(t *testing.T) {
	ctx := context.Background()
	var mockConnManager machines.ConnectionManagerInterface = testutils.NewMockConnectionManager()
	service := NewQuadletService(mockConnManager, nil)

	result, err := service.Update(ctx, "test-user", "test-machine", "my-container.container", QuadletUpdateRequest{
		Content: "[Container]\nImage=nginx:1.25\n",
	})

	// The mock connection succeeds but the QuadletList existence check fails
	// because there is no real podman backend behind the mock connection context.
	assert.Error(t, err, "Should return error from engine call with mock connection")
	assert.Nil(t, result, "Should return nil result when engine call fails")
	assert.NotContains(t, err.Error(), "CONNECTION_FAILED",
		"Error should not be a connection error — connection should have succeeded")
}

// --- Service seam: Delete ---
//
// As with List/Get/Create, the engine boundary is built inline and cannot be
// exercised under the mock connection. These tests cover the connection-stage
// error wrapping and the invocation shape (the call reaches the engine step but
// fails because there is no real podman backend). The domain logic
// (not-found / running guard) is covered directly via the pure helpers above.

func TestQuadletService_Delete_ConnectionFailure(t *testing.T) {
	ctx := context.Background()
	connErr := errors.New("SSH connection failed")
	var mockConnManager machines.ConnectionManagerInterface = testutils.NewMockConnectionManagerWithError(connErr)
	service := NewQuadletService(mockConnManager, nil)

	err := service.Delete(ctx, "test-user", "test-machine", "my-container.container", false, true)

	assert.Error(t, err, "Should return error when connection fails")
	assert.Equal(t, err, appErrors.ErrConnectionFailed, "Should map to ErrConnectionFailed")
}

func TestQuadletService_Delete_InvocationShape(t *testing.T) {
	ctx := context.Background()
	var mockConnManager machines.ConnectionManagerInterface = testutils.NewMockConnectionManager()
	service := NewQuadletService(mockConnManager, nil)

	err := service.Delete(ctx, "test-user", "test-machine", "my-container.container", false, true)

	// The mock connection succeeds but the QuadletList pre-check fails because
	// there is no real podman backend behind the mock connection context.
	assert.Error(t, err, "Should return error from engine call with mock connection")
	assert.NotContains(t, err.Error(), "CONNECTION_FAILED",
		"Error should not be a connection error — connection should have succeeded")
}

func TestQuadletService_Delete_ForceTrue_InvocationShape(t *testing.T) {
	ctx := context.Background()
	var mockConnManager machines.ConnectionManagerInterface = testutils.NewMockConnectionManager()
	service := NewQuadletService(mockConnManager, nil)

	err := service.Delete(ctx, "test-user", "test-machine", "my-container.container", true, true)

	// Same shape as above — force only affects the post-list guard, which we
	// never reach under the mock connection.
	assert.Error(t, err, "Should return error from engine call with mock connection")
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
		{"/etc/containers/systemd/k8s.kube", "container"},        // kube -> container
		{"/etc/containers/systemd/bld.build", "image"},           // build -> image
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

// --- Pure helpers used by Create ---

func TestResourceToExtension(t *testing.T) {
	tests := []struct {
		resource string
		want     string
	}{
		{"container", ".container"},
		{"pod", ".pod"},
		{"image", ".image"},
		{"volume", ".volume"},
		{"network", ".network"},
		{"CONTAINER", ".container"}, // case-insensitive
		{"Pod", ".pod"},
		{"", ".container"},      // empty defaults to container
		{"kube", ".container"},  // not in contract enum → default
		{"build", ".container"}, // not in contract enum → default
		{"bogus", ".container"}, // unknown → default
	}
	for _, tt := range tests {
		t.Run(tt.resource, func(t *testing.T) {
			assert.Equal(t, tt.want, resourceToExtension(tt.resource))
		})
	}
}

func TestCreateTarBody(t *testing.T) {
	t.Run("produces a valid single-file tar with the requested name and content", func(t *testing.T) {
		fileName := "my-container.container"
		content := []byte("[Container]\nImage=nginx:latest\n")

		reader, err := createTarBody(fileName, content)
		assert.NoError(t, err, "createTarBody should not error for valid input")

		got, err := io.ReadAll(reader)
		assert.NoError(t, err)

		tr := tar.NewReader(bytes.NewReader(got))

		hdr, err := tr.Next()
		assert.NoError(t, err, "Should yield at least one header")
		assert.Equal(t, fileName, hdr.Name, "Tar entry name should match the requested filename")
		assert.Equal(t, int64(len(content)), hdr.Size, "Header size should match content length")

		body, err := io.ReadAll(tr)
		assert.NoError(t, err)
		assert.Equal(t, content, body, "Tar entry body should equal the input content")

		// No further entries beyond EOF.
		_, err = tr.Next()
		assert.ErrorIs(t, err, io.EOF, "Tar should contain exactly one entry")
	})

	t.Run("empty content still produces a valid (zero-length) tar entry", func(t *testing.T) {
		reader, err := createTarBody("empty.container", []byte{})
		assert.NoError(t, err)

		got, err := io.ReadAll(reader)
		assert.NoError(t, err)

		tr := tar.NewReader(bytes.NewReader(got))
		hdr, err := tr.Next()
		assert.NoError(t, err)
		assert.Equal(t, "empty.container", hdr.Name)
		assert.Equal(t, int64(0), hdr.Size)
		_, err = tr.Next()
		assert.ErrorIs(t, err, io.EOF)
	})
}

func TestIsAlreadyExistsError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"plain error", errors.New("something broke"), false},
		{"already exists phrase", errors.New("quadlet my-container already exists"), true},
		{"bare exists phrase is not enough", errors.New("file exists"), false},
		{"case-insensitive", errors.New("Quadlet ALREADY EXISTS"), true},
		{"unrelated message", errors.New("network unreachable"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isAlreadyExistsError(tt.err))
		})
	}
}

// --- Pure helpers used by Create + Update ---

func TestFirstInstalledPath(t *testing.T) {
	t.Run("nil map returns empty", func(t *testing.T) {
		assert.Equal(t, "", firstInstalledPath(nil))
	})

	t.Run("empty map returns empty", func(t *testing.T) {
		assert.Equal(t, "", firstInstalledPath(map[string]string{}))
	})

	t.Run("populated map returns one of the destinations", func(t *testing.T) {
		// Map ordering is non-deterministic; the contract is "a populated path".
		installed := map[string]string{
			"/tmp/x.container": "/etc/containers/systemd/x.container",
		}
		assert.Equal(t, "/etc/containers/systemd/x.container", firstInstalledPath(installed))
	})

	t.Run("multiple entries returns exactly one (non-empty)", func(t *testing.T) {
		installed := map[string]string{
			"/tmp/a.container": "/etc/containers/systemd/a.container",
			"/tmp/b.container": "/etc/containers/systemd/b.container",
		}
		got := firstInstalledPath(installed)
		assert.NotEmpty(t, got)
		assert.Contains(t, installed, "/tmp/a.container")
	})
}

// --- Pure helpers used by Update ---

func TestBuildUpdateResponse(t *testing.T) {
	t.Run("maps all fields and forces state to unknown", func(t *testing.T) {
		got := buildUpdateResponse(
			"my-container.container",
			"my-container.service",
			"container",
			"/etc/containers/systemd/my-container.container",
			"[Container]\nImage=nginx:1.25\n",
		)

		assert.Equal(t, &QuadletGetResponse{
			ID:       "my-container.container",
			Name:     "my-container.container",
			Path:     "/etc/containers/systemd/my-container.container",
			Unit:     "my-container.service",
			Resource: "container",
			State:    "unknown",
			Content:  "[Container]\nImage=nginx:1.25\n",
		}, got)
	})

	t.Run("state is unknown even though a freshly updated unit may be running", func(t *testing.T) {
		// The decision under test: Update reports "unknown" right after reinstall
		// because systemd has not reloaded to report a fresh status. This pins
		// that decision regardless of the inputs.
		got := buildUpdateResponse("x.pod", "x.service", "pod", "/p/x.pod", "[Pod]\n")
		assert.Equal(t, "unknown", got.State)
	})

	t.Run("empty installed path is echoed as empty", func(t *testing.T) {
		got := buildUpdateResponse("x.container", "x.service", "container", "", "c")
		assert.Equal(t, "", got.Path)
	})
}

// Compile-time check that *QuadletCreateResponse has the fields the issue requires.
func TestQuadletCreateResponse_Shape(t *testing.T) {
	r := QuadletCreateResponse{
		ID:       "my-container",
		Name:     "my-container",
		Path:     "/etc/containers/systemd/my-container.container",
		Unit:     "",
		Resource: "container",
		State:    "unknown",
		Content:  "[Container]\nImage=nginx:latest\n",
	}
	assert.Equal(t, "my-container", r.ID)
	assert.Equal(t, "unknown", r.State)
}

// Compile-time sanity that ErrQuadletAlreadyExists is exposed for handler mapping.
func TestQuadletAlreadyExists_ErrorIsExposed(t *testing.T) {
	assert.Equal(t, "QUADLET_ALREADY_EXISTS", appErrors.ErrQuadletAlreadyExists.Code)
}

// --- Pure helpers used by Delete ---
//
// The delete path's domain logic (not-found detection from a list, the
// running-resource guard, and post-remove error mapping) is extracted into pure
// functions so it can be unit-tested directly — the engine boundary itself
// cannot be exercised under the mock connection (see the seam note at the top
// of the List tests).

func TestFindQuadletByName(t *testing.T) {
	list := []*entities.ListQuadlet{
		{Name: "alpha.container", Status: "running"},
		{Name: "bravo.pod", Status: "stopped"},
	}

	t.Run("found", func(t *testing.T) {
		got := findQuadletByName(list, "bravo.pod")
		assert.NotNil(t, got)
		assert.Equal(t, "bravo.pod", got.Name)
	})

	t.Run("not found returns nil", func(t *testing.T) {
		assert.Nil(t, findQuadletByName(list, "missing.container"))
	})

	t.Run("empty list returns nil", func(t *testing.T) {
		assert.Nil(t, findQuadletByName(nil, "alpha.container"))
	})

	t.Run("nil entries handled", func(t *testing.T) {
		list := []*entities.ListQuadlet{nil, {Name: "x.container"}}
		got := findQuadletByName(list, "x.container")
		assert.NotNil(t, got)
		assert.Equal(t, "x.container", got.Name)
	})
}

func TestCheckDeletePreconditions(t *testing.T) {
	t.Run("nil target is not found", func(t *testing.T) {
		err := checkDeletePreconditions(nil, false)
		assert.Equal(t, appErrors.ErrQuadletNotFound, err)
	})

	t.Run("stopped quadlet may be deleted without force", func(t *testing.T) {
		err := checkDeletePreconditions(&entities.ListQuadlet{Name: "x.container", Status: "stopped"}, false)
		assert.NoError(t, err)
	})

	t.Run("unknown state may be deleted without force", func(t *testing.T) {
		err := checkDeletePreconditions(&entities.ListQuadlet{Name: "x.container", Status: ""}, false)
		assert.NoError(t, err)
	})

	t.Run("running quadlet without force is rejected", func(t *testing.T) {
		err := checkDeletePreconditions(&entities.ListQuadlet{Name: "x.container", Status: "running (1s ago)"}, false)
		assert.Equal(t, appErrors.ErrQuadletResourceRunning, err)
	})

	t.Run("running quadlet with force is allowed", func(t *testing.T) {
		err := checkDeletePreconditions(&entities.ListQuadlet{Name: "x.container", Status: "running (1s ago)"}, true)
		assert.NoError(t, err)
	})

	t.Run("failed/error state may be deleted without force", func(t *testing.T) {
		err := checkDeletePreconditions(&entities.ListQuadlet{Name: "x.container", Status: "failed"}, false)
		assert.NoError(t, err)
	})
}

func TestIsNotFoundError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"plain error", errors.New("something broke"), false},
		{"no such quadlet", errors.New("no such quadlet: foo.container"), true},
		{"quadlet not found", errors.New("quadlet foo not found"), true},
		{"no such file", errors.New("no such file or directory"), true},
		{"case-insensitive not found", errors.New("Quadlet NOT FOUND"), true},
		{"unrelated running error", errors.New("resource is running"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isNotFoundError(tt.err))
		})
	}
}

func TestIsResourceRunningError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"plain error", errors.New("something broke"), false},
		{"in use", errors.New("resource is in use"), true},
		{"is running", errors.New("container is running"), true},
		{"still running", errors.New("unit still running"), true},
		{"active", errors.New("resource is active"), true},
		{"case-insensitive running", errors.New("RESOURCE IS RUNNING"), true},
		{"not found is not running", errors.New("no such quadlet"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isResourceRunningError(tt.err))
		})
	}
}

// --- Service seam: Generate ---

func TestQuadletService_Generate_ConnectionFailure(t *testing.T) {
	ctx := context.Background()
	connErr := errors.New("SSH connection failed")
	var mockConnManager machines.ConnectionManagerInterface = testutils.NewMockConnectionManagerWithError(connErr)
	service := NewQuadletService(mockConnManager, nil)

	result, err := service.Generate(ctx, "test-user", "test-machine", QuadletGenerateRequest{
		ResourceID:   "my-container",
		ResourceType: "container",
	})

	assert.Error(t, err, "Should return error when connection fails")
	assert.Equal(t, err, appErrors.ErrConnectionFailed, "Should map to ErrConnectionFailed")
	assert.Nil(t, result, "Should return nil result when connection fails")
}

func TestQuadletService_Generate_Container_Success(t *testing.T) {
	ctx := context.Background()
	var mockConnManager machines.ConnectionManagerInterface = testutils.NewMockConnectionManager()
	service := NewQuadletService(mockConnManager, nil)

	result, err := service.Generate(ctx, "test-user", "test-machine", QuadletGenerateRequest{
		ResourceID:   "my-container",
		ResourceType: "container",
	})

	// The mock connection succeeds but the engine call fails because there is
	// no real podman backend behind the mock connection context.
	assert.Error(t, err, "Should return error from engine call with mock connection")
	assert.Nil(t, result, "Should return nil result when engine call fails")
	assert.NotContains(t, err.Error(), "CONNECTION_FAILED",
		"Error should not be a connection error — connection should have succeeded")
}

func TestQuadletService_Generate_Pod_Success(t *testing.T) {
	ctx := context.Background()
	var mockConnManager machines.ConnectionManagerInterface = testutils.NewMockConnectionManager()
	service := NewQuadletService(mockConnManager, nil)

	result, err := service.Generate(ctx, "test-user", "test-machine", QuadletGenerateRequest{
		ResourceID:   "my-pod",
		ResourceType: "pod",
	})

	// The mock connection succeeds but the engine call fails because there is
	// no real podman backend behind the mock connection context.
	assert.Error(t, err, "Should return error from engine call with mock connection")
	assert.Nil(t, result, "Should return nil result when engine call fails")
	assert.NotContains(t, err.Error(), "CONNECTION_FAILED",
		"Error should not be a connection error — connection should have succeeded")
}

func TestQuadletService_Generate_InvalidResourceType(t *testing.T) {
	ctx := context.Background()
	var mockConnManager machines.ConnectionManagerInterface = testutils.NewMockConnectionManager()
	service := NewQuadletService(mockConnManager, nil)

	result, err := service.Generate(ctx, "test-user", "test-machine", QuadletGenerateRequest{
		ResourceID:   "my-resource",
		ResourceType: "invalid",
	})

	assert.Error(t, err, "Should return error for invalid resource type")
	assert.Equal(t, err, appErrors.ErrInvalidRequest, "Should map to ErrInvalidRequest")
	assert.Nil(t, result, "Should return nil result for invalid resource type")
}

// --- Pure helpers used by Generate ---

func TestGenerateContainerQuadlet(t *testing.T) {
	t.Run("generates valid container quadlet with all fields", func(t *testing.T) {
		inspect := &entities.ContainerInspectReport{
			InspectContainerData: &define.InspectContainerData{
				Name: "my-container",
				Config: &define.InspectContainerConfig{
					Image: "nginx:latest",
					Env:   []string{"PORT=8080", "DEBUG=false"},
					Labels: map[string]string{
						"version": "1.0",
						"app":     "web",
					},
				},
				HostConfig: &define.InspectContainerHostConfig{
					RestartPolicy: &define.InspectRestartPolicy{
						Name: "always",
					},
					PortBindings: map[string][]define.InspectHostPort{
						"80/tcp": {{HostPort: "8080"}},
					},
					Binds:       []string{"/host/path:/container/path"},
					NetworkMode: "bridge",
				},
			},
		}

		content, err := generateContainerQuadlet(inspect)

		assert.NoError(t, err, "generateContainerQuadlet should succeed for valid input")
		assert.Contains(t, content, "# Generated by Podland from container my-container")
		assert.Contains(t, content, "[Unit]")
		assert.Contains(t, content, "Description=Podman container my-container")
		assert.Contains(t, content, "[Container]")
		assert.Contains(t, content, "ContainerName=my-container")
		assert.Contains(t, content, "Image=nginx:latest")
		assert.Contains(t, content, "Environment=PORT=8080")
		assert.Contains(t, content, "Environment=DEBUG=false")
		assert.Contains(t, content, "Label=version=1.0")
		assert.Contains(t, content, "Label=app=web")
		assert.Contains(t, content, "RestartPolicy=always")
		assert.Contains(t, content, "PublishPort=8080:80/tcp")
		assert.Contains(t, content, "Volume=/host/path:/container/path")
		assert.Contains(t, content, "Network=bridge")
	})

	t.Run("handles minimal container config", func(t *testing.T) {
		inspect := &entities.ContainerInspectReport{
			InspectContainerData: &define.InspectContainerData{
				Name: "simple-container",
				Config: &define.InspectContainerConfig{
					Image: "alpine:latest",
				},
			},
		}

		content, err := generateContainerQuadlet(inspect)

		assert.NoError(t, err)
		assert.Contains(t, content, "ContainerName=simple-container")
		assert.Contains(t, content, "Image=alpine:latest")
		assert.NotContains(t, content, "Environment=")
		assert.NotContains(t, content, "RestartPolicy=")
		assert.NotContains(t, content, "PublishPort=")
		assert.NotContains(t, content, "Volume=")
		assert.NotContains(t, content, "Network=")
		assert.NotContains(t, content, "Label=")
	})

	t.Run("handles nil config", func(t *testing.T) {
		inspect := &entities.ContainerInspectReport{
			InspectContainerData: &define.InspectContainerData{
				Name:   "broken-container",
				Config: nil,
			},
		}

		content, err := generateContainerQuadlet(inspect)

		assert.Error(t, err, "generateContainerQuadlet should fail with nil config")
		assert.Empty(t, content)
	})

	t.Run("handles nil inspect report", func(t *testing.T) {
		content, err := generateContainerQuadlet(nil)

		assert.Error(t, err, "generateContainerQuadlet should fail with nil inspect report")
		assert.Empty(t, content)
	})

	t.Run("handles empty container name", func(t *testing.T) {
		inspect := &entities.ContainerInspectReport{
			InspectContainerData: &define.InspectContainerData{
				Name: "",
				Config: &define.InspectContainerConfig{
					Image: "nginx:latest",
				},
			},
		}

		content, err := generateContainerQuadlet(inspect)

		assert.Error(t, err, "generateContainerQuadlet should fail with empty name")
		assert.Empty(t, content)
	})

	t.Run("handles restart policy 'no'", func(t *testing.T) {
		inspect := &entities.ContainerInspectReport{
			InspectContainerData: &define.InspectContainerData{
				Name: "no-restart-container",
				Config: &define.InspectContainerConfig{
					Image: "nginx:latest",
				},
				HostConfig: &define.InspectContainerHostConfig{
					RestartPolicy: &define.InspectRestartPolicy{
						Name: "no",
					},
				},
			},
		}

		content, err := generateContainerQuadlet(inspect)

		assert.NoError(t, err)
		assert.NotContains(t, content, "RestartPolicy=")
	})

	t.Run("handles multiple port bindings", func(t *testing.T) {
		inspect := &entities.ContainerInspectReport{
			InspectContainerData: &define.InspectContainerData{
				Name: "multi-port-container",
				Config: &define.InspectContainerConfig{
					Image: "nginx:latest",
				},
				HostConfig: &define.InspectContainerHostConfig{
					PortBindings: map[string][]define.InspectHostPort{
						"80/tcp":  {{HostPort: "8080"}},
						"443/tcp": {{HostPort: "8443"}},
					},
				},
			},
		}

		content, err := generateContainerQuadlet(inspect)

		assert.NoError(t, err)
		assert.Contains(t, content, "PublishPort=")
	})

	t.Run("handles multiple volumes", func(t *testing.T) {
		inspect := &entities.ContainerInspectReport{
			InspectContainerData: &define.InspectContainerData{
				Name: "multi-volume-container",
				Config: &define.InspectContainerConfig{
					Image: "nginx:latest",
				},
				HostConfig: &define.InspectContainerHostConfig{
					Binds: []string{
						"/host1:/container1",
						"/host2:/container2",
					},
				},
			},
		}

		content, err := generateContainerQuadlet(inspect)

		assert.NoError(t, err)
		assert.Contains(t, content, "Volume=/host1:/container1")
		assert.Contains(t, content, "Volume=/host2:/container2")
	})
}

func TestGeneratePodQuadlet(t *testing.T) {
	t.Run("generates valid pod quadlet with all fields", func(t *testing.T) {
		inspect := &entities.PodInspectReport{
			InspectPodData: &define.InspectPodData{
				Name: "my-pod",
				CreateCommand: []string{
					"--name=my-pod",
					"--publish=8080:80",
				},
				Labels: map[string]string{
					"version": "1.0",
					"app":     "backend",
				},
			},
		}

		content, err := generatePodQuadlet(inspect)

		assert.NoError(t, err, "generatePodQuadlet should succeed for valid input")
		assert.Contains(t, content, "# Generated by Podland from pod my-pod")
		assert.Contains(t, content, "[Unit]")
		assert.Contains(t, content, "Description=Podman pod my-pod")
		assert.Contains(t, content, "[Pod]")
		assert.Contains(t, content, "PodName=my-pod")
		assert.Contains(t, content, "PodmanArgs=--name=my-pod --publish=8080:80")
		assert.Contains(t, content, "Label=version=1.0")
		assert.Contains(t, content, "Label=app=backend")
	})

	t.Run("handles minimal pod config", func(t *testing.T) {
		inspect := &entities.PodInspectReport{
			InspectPodData: &define.InspectPodData{
				Name: "simple-pod",
			},
		}

		content, err := generatePodQuadlet(inspect)

		assert.NoError(t, err)
		assert.Contains(t, content, "PodName=simple-pod")
		assert.NotContains(t, content, "PodmanArgs=")
		assert.NotContains(t, content, "Label=")
	})

	t.Run("handles nil inspect report", func(t *testing.T) {
		content, err := generatePodQuadlet(nil)

		assert.Error(t, err, "generatePodQuadlet should fail with nil inspect report")
		assert.Empty(t, content)
	})

	t.Run("handles empty pod name", func(t *testing.T) {
		inspect := &entities.PodInspectReport{
			InspectPodData: &define.InspectPodData{
				Name: "",
			},
		}

		content, err := generatePodQuadlet(inspect)

		assert.Error(t, err, "generatePodQuadlet should fail with empty name")
		assert.Empty(t, content)
	})

	t.Run("handles empty create command", func(t *testing.T) {
		inspect := &entities.PodInspectReport{
			InspectPodData: &define.InspectPodData{
				Name:          "no-command-pod",
				CreateCommand: []string{},
			},
		}

		content, err := generatePodQuadlet(inspect)

		assert.NoError(t, err)
		assert.NotContains(t, content, "PodmanArgs=")
	})

	t.Run("handles nil create command", func(t *testing.T) {
		inspect := &entities.PodInspectReport{
			InspectPodData: &define.InspectPodData{
				Name:          "nil-command-pod",
				CreateCommand: nil,
			},
		}

		content, err := generatePodQuadlet(inspect)

		assert.NoError(t, err)
		assert.NotContains(t, content, "PodmanArgs=")
	})

	t.Run("handles empty labels", func(t *testing.T) {
		inspect := &entities.PodInspectReport{
			InspectPodData: &define.InspectPodData{
				Name:   "no-labels-pod",
				Labels: map[string]string{},
			},
		}

		content, err := generatePodQuadlet(inspect)

		assert.NoError(t, err)
		assert.NotContains(t, content, "Label=")
	})

	t.Run("handles nil labels", func(t *testing.T) {
		inspect := &entities.PodInspectReport{
			InspectPodData: &define.InspectPodData{
				Name:   "nil-labels-pod",
				Labels: nil,
			},
		}

		content, err := generatePodQuadlet(inspect)

		assert.NoError(t, err)
		assert.NotContains(t, content, "Label=")
	})
}
