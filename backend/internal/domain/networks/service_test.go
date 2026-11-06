package networks

import (
	"context"
	"errors"
	"net/http"
	"testing"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/common/libnetwork/types"
	errorhandling "go.podman.io/podman/v6/pkg/errorhandling"
)

// Service behaviour tests run the real NetworkService against the fake
// engine, so the mapping rules below are pinned as behaviour, not
// invocation shape. Connection acquisition (and its 502 mapping) is the
// provider seam: the connections package owns that contract and pins it
// in its own tests.

func TestNetworkService_List_ReturnsEngineNetworks(t *testing.T) {
	networks := []types.Network{
		networkFixture("podman", "bridge", "2026-08-30T10:00:00Z", "10.88.0.0/16"),
		networkFixture("frontend", "bridge", "2026-09-01T12:30:00Z", "10.89.0.0/16", "fd00:10:89::/64"),
	}
	engine := &fakeNetworkEngine{
		listFn: func() ([]types.Network, error) {
			return networks, nil
		},
	}
	service := NewNetworkService(fakeEngineProvider{engine: engine})

	got, err := service.List(context.Background(), "user", "host")

	require.NoError(t, err)
	assert.Equal(t, networks, got)
}

func TestNetworkService_List_WrapsEngineFailure(t *testing.T) {
	engine := &fakeNetworkEngine{
		listFn: func() ([]types.Network, error) {
			return nil, errors.New("boom")
		},
	}
	service := NewNetworkService(fakeEngineProvider{engine: engine})

	got, err := service.List(context.Background(), "user", "host")

	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "failed to list networks", "engine failures must be wrapped consistently")
}

// TestNetworkService_AcquireFailureIsThe502Sentinel pins the service-side
// half of the connection-failure contract: the provider's sentinel crosses
// untouched — the service never wraps, maps, or swallows it (the 502
// mapping itself is pinned in the connections package).
func TestNetworkService_AcquireFailureIsThe502Sentinel(t *testing.T) {
	service := NewNetworkService(fakeEngineProvider{err: appErrors.ErrConnectionFailed})

	_, err := service.List(context.Background(), "user", "host")

	require.ErrorIs(t, err, appErrors.ErrConnectionFailed)
	var appErr *appErrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, http.StatusBadGateway, appErr.Status)
}

func TestNetworkService_Create_ReturnsCreatedNetwork(t *testing.T) {
	created := networkFixture("frontend", "bridge", "2026-10-08T10:00:00Z", "10.89.0.0/16")
	var createdName string
	engine := &fakeNetworkEngine{
		createFn: func(name string) (*types.Network, error) {
			createdName = name
			return &created, nil
		},
	}
	service := NewNetworkService(fakeEngineProvider{engine: engine})

	got, err := service.Create(context.Background(), "user", "host", NetworkCreateRequest{Name: "frontend"})

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "frontend", createdName, "the request's name must reach the host")
	assert.Equal(t, created, *got)
}

func TestNetworkService_Create_AttachesListedContainers(t *testing.T) {
	created := networkFixture("frontend", "bridge", "2026-10-08T10:00:00Z", "10.89.0.0/16")
	var attached []string
	engine := &fakeNetworkEngine{
		createFn: func(name string) (*types.Network, error) {
			return &created, nil
		},
		connectFn: func(networkName, containerID string) error {
			assert.Equal(t, "frontend", networkName, "attaches run against the created network")
			attached = append(attached, containerID)
			return nil
		},
	}
	service := NewNetworkService(fakeEngineProvider{engine: engine})

	_, err := service.Create(context.Background(), "user", "host", NetworkCreateRequest{
		Name:       "frontend",
		Containers: []string{"ctr-a", "ctr-b"},
	})

	require.NoError(t, err)
	assert.Equal(t, []string{"ctr-a", "ctr-b"}, attached, "every listed container must be attached after the create")
}

func TestNetworkService_Create_MapsDuplicateNameTo409(t *testing.T) {
	engine := &fakeNetworkEngine{
		createFn: func(name string) (*types.Network, error) {
			return nil, &errorhandling.ErrorModel{
				ResponseCode: http.StatusConflict,
				Message:      "network already exists",
			}
		},
	}
	service := NewNetworkService(fakeEngineProvider{engine: engine})

	got, err := service.Create(context.Background(), "user", "host", NetworkCreateRequest{Name: "frontend"})

	require.ErrorIs(t, err, appErrors.ErrNetworkAlreadyExists)
	assert.Nil(t, got)
	var appErr *appErrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, http.StatusConflict, appErr.Status)
	assert.Equal(t, "NETWORK_ALREADY_EXISTS", appErr.Code)
}

func TestNetworkService_Create_MapsClientErrorsToInvalidRequest(t *testing.T) {
	engine := &fakeNetworkEngine{
		createFn: func(name string) (*types.Network, error) {
			return nil, &errorhandling.ErrorModel{
				ResponseCode: http.StatusBadRequest,
				Message:      "network name invalid",
			}
		},
	}
	service := NewNetworkService(fakeEngineProvider{engine: engine})

	got, err := service.Create(context.Background(), "user", "host", NetworkCreateRequest{Name: "bad"})

	require.Error(t, err)
	assert.Nil(t, got)
	var appErr *appErrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, http.StatusBadRequest, appErr.Status)
	assert.Equal(t, "INVALID_REQUEST", appErr.Code)
	assert.Contains(t, appErr.Message, "network name invalid", "podman's rejection text must reach the frontend")
}

func TestNetworkService_Create_WrapsEngineFailure(t *testing.T) {
	engine := &fakeNetworkEngine{
		createFn: func(name string) (*types.Network, error) {
			return nil, errors.New("boom")
		},
	}
	service := NewNetworkService(fakeEngineProvider{engine: engine})

	got, err := service.Create(context.Background(), "user", "host", NetworkCreateRequest{Name: "frontend"})

	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "failed to create network", "engine failures must be wrapped consistently")
}

// TestNetworkService_Create_AttachFailureNamesContainerAndLeavesNetwork
// pins the create-with-attach failure contract: the error names the
// failing container, and the network stays created — the create call
// happened and nothing rolls it back (the engine seam has no remove
// operation, so rollback is impossible by construction).
func TestNetworkService_Create_AttachFailureNamesContainerAndLeavesNetwork(t *testing.T) {
	created := networkFixture("frontend", "bridge", "2026-10-08T10:00:00Z", "10.89.0.0/16")
	createCalled := false
	var attached []string
	engine := &fakeNetworkEngine{
		createFn: func(name string) (*types.Network, error) {
			createCalled = true
			return &created, nil
		},
		connectFn: func(networkName, containerID string) error {
			if containerID == "bad-ctr" {
				return &errorhandling.ErrorModel{
					ResponseCode: http.StatusInternalServerError,
					Message:      "container is not running",
				}
			}
			attached = append(attached, containerID)
			return nil
		},
	}
	service := NewNetworkService(fakeEngineProvider{engine: engine})

	got, err := service.Create(context.Background(), "user", "host", NetworkCreateRequest{
		Name:       "frontend",
		Containers: []string{"good-ctr", "bad-ctr"},
	})

	require.Error(t, err)
	assert.Nil(t, got)
	assert.True(t, createCalled, "the network stays created: the create is not rolled back")
	assert.Equal(t, []string{"good-ctr"}, attached, "attaches run in order until the failing one")
	var appErr *appErrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, http.StatusConflict, appErr.Status)
	assert.Equal(t, "NETWORK_ATTACH_FAILED", appErr.Code)
	assert.Contains(t, appErr.Message, "bad-ctr", "the error must name the failing container")
	assert.Contains(t, appErr.Message, "container is not running", "podman's rejection text must reach the frontend")
}

func TestNetworkService_Create_AttachUnknownContainerMapsTo404(t *testing.T) {
	created := networkFixture("frontend", "bridge", "2026-10-08T10:00:00Z", "10.89.0.0/16")
	engine := &fakeNetworkEngine{
		createFn: func(name string) (*types.Network, error) {
			return &created, nil
		},
		connectFn: func(networkName, containerID string) error {
			return &errorhandling.ErrorModel{
				ResponseCode: http.StatusNotFound,
				Message:      "no such container",
			}
		},
	}
	service := NewNetworkService(fakeEngineProvider{engine: engine})

	_, err := service.Create(context.Background(), "user", "host", NetworkCreateRequest{
		Name:       "frontend",
		Containers: []string{"ghost-ctr"},
	})

	var appErr *appErrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, http.StatusNotFound, appErr.Status)
	assert.Equal(t, "CONTAINER_NOT_FOUND", appErr.Code)
	assert.Contains(t, appErr.Message, "ghost-ctr", "the error must name the failing container")
}

func TestNetworkService_Create_AcquireFailureIsThe502Sentinel(t *testing.T) {
	service := NewNetworkService(fakeEngineProvider{err: appErrors.ErrConnectionFailed})

	_, err := service.Create(context.Background(), "user", "host", NetworkCreateRequest{Name: "frontend"})

	require.ErrorIs(t, err, appErrors.ErrConnectionFailed)
	var appErr *appErrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, http.StatusBadGateway, appErr.Status)
}

func TestNetworkService_Connect_AttachesContainer(t *testing.T) {
	networks := []types.Network{
		networkFixture("podman", "bridge", "2026-08-30T10:00:00Z", "10.88.0.0/16"),
		networkFixture("frontend", "bridge", "2026-09-01T12:30:00Z", "10.89.0.0/16"),
	}
	var connectedNetwork, connectedContainer string
	engine := &fakeNetworkEngine{
		listFn: func() ([]types.Network, error) {
			return networks, nil
		},
		connectFn: func(networkName, containerID string) error {
			connectedNetwork = networkName
			connectedContainer = containerID
			return nil
		},
	}
	service := NewNetworkService(fakeEngineProvider{engine: engine})

	err := service.Connect(context.Background(), "user", "host", "frontend", "ctr-a")

	require.NoError(t, err)
	assert.Equal(t, "frontend", connectedNetwork)
	assert.Equal(t, "ctr-a", connectedContainer)
}

func TestNetworkService_Connect_MapsUnknownNetworkTo404(t *testing.T) {
	networks := []types.Network{
		networkFixture("podman", "bridge", "2026-08-30T10:00:00Z", "10.88.0.0/16"),
	}
	engine := &fakeNetworkEngine{
		listFn: func() ([]types.Network, error) {
			return networks, nil
		},
		// connectFn stays unarmed: an unknown network must never reach
		// the connect call.
	}
	service := NewNetworkService(fakeEngineProvider{engine: engine})

	err := service.Connect(context.Background(), "user", "host", "ghost-net", "ctr-a")

	require.ErrorIs(t, err, appErrors.ErrNetworkNotFound)
	var appErr *appErrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, http.StatusNotFound, appErr.Status)
	assert.Equal(t, "NETWORK_NOT_FOUND", appErr.Code)
}

func TestNetworkService_Connect_MapsUnknownContainerTo404(t *testing.T) {
	networks := []types.Network{
		networkFixture("frontend", "bridge", "2026-09-01T12:30:00Z", "10.89.0.0/16"),
	}
	engine := &fakeNetworkEngine{
		listFn: func() ([]types.Network, error) {
			return networks, nil
		},
		connectFn: func(networkName, containerID string) error {
			return &errorhandling.ErrorModel{
				ResponseCode: http.StatusNotFound,
				Message:      "no such container",
			}
		},
	}
	service := NewNetworkService(fakeEngineProvider{engine: engine})

	err := service.Connect(context.Background(), "user", "host", "frontend", "ghost-ctr")

	require.ErrorIs(t, err, appErrors.ErrContainerNotFound)
	var appErr *appErrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, http.StatusNotFound, appErr.Status)
	assert.Equal(t, "CONTAINER_NOT_FOUND", appErr.Code)
}

func TestNetworkService_Connect_MapsClientErrorsToInvalidRequest(t *testing.T) {
	networks := []types.Network{
		networkFixture("frontend", "bridge", "2026-09-01T12:30:00Z", "10.89.0.0/16"),
	}
	engine := &fakeNetworkEngine{
		listFn: func() ([]types.Network, error) {
			return networks, nil
		},
		connectFn: func(networkName, containerID string) error {
			return &errorhandling.ErrorModel{
				ResponseCode: http.StatusBadRequest,
				Message:      "invalid container",
			}
		},
	}
	service := NewNetworkService(fakeEngineProvider{engine: engine})

	err := service.Connect(context.Background(), "user", "host", "frontend", "ctr-a")

	require.Error(t, err)
	var appErr *appErrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, http.StatusBadRequest, appErr.Status)
	assert.Equal(t, "INVALID_REQUEST", appErr.Code)
	assert.Contains(t, appErr.Message, "invalid container", "podman's rejection text must reach the frontend")
}

func TestNetworkService_Connect_WrapsEngineFailure(t *testing.T) {
	networks := []types.Network{
		networkFixture("frontend", "bridge", "2026-09-01T12:30:00Z", "10.89.0.0/16"),
	}
	engine := &fakeNetworkEngine{
		listFn: func() ([]types.Network, error) {
			return networks, nil
		},
		connectFn: func(networkName, containerID string) error {
			return errors.New("boom")
		},
	}
	service := NewNetworkService(fakeEngineProvider{engine: engine})

	err := service.Connect(context.Background(), "user", "host", "frontend", "ctr-a")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to connect container to network", "engine failures must be wrapped consistently")
}

func TestNetworkService_Connect_WrapsListFailure(t *testing.T) {
	engine := &fakeNetworkEngine{
		listFn: func() ([]types.Network, error) {
			return nil, errors.New("boom")
		},
		// connectFn stays unarmed: the target resolution fails first.
	}
	service := NewNetworkService(fakeEngineProvider{engine: engine})

	err := service.Connect(context.Background(), "user", "host", "frontend", "ctr-a")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list networks", "the resolution failure must be wrapped consistently")
}

func TestNetworkService_Connect_AcquireFailureIsThe502Sentinel(t *testing.T) {
	service := NewNetworkService(fakeEngineProvider{err: appErrors.ErrConnectionFailed})

	err := service.Connect(context.Background(), "user", "host", "frontend", "ctr-a")

	require.ErrorIs(t, err, appErrors.ErrConnectionFailed)
	var appErr *appErrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, http.StatusBadGateway, appErr.Status)
}

// TestNetworkService_Delete_RemovesByName pins the delete contract: the
// exact name and the force flag reach the host, and a clean removal
// answers nil.
func TestNetworkService_Delete_RemovesByName(t *testing.T) {
	var gotName string
	var gotForce bool
	engine := &fakeNetworkEngine{
		removeFn: func(name string, force bool) error {
			gotName = name
			gotForce = force
			return nil
		},
	}
	service := NewNetworkService(fakeEngineProvider{engine: engine})

	err := service.Delete(context.Background(), "user", "host", "frontend", true)

	require.NoError(t, err)
	assert.Equal(t, "frontend", gotName)
	assert.True(t, gotForce, "the force flag must reach the host")
}

// TestNetworkService_Delete_MapsNotFoundTo404 pins the vanished-network
// terminal: podman's not-found answer becomes the clean 404 sentinel.
func TestNetworkService_Delete_MapsNotFoundTo404(t *testing.T) {
	engine := &fakeNetworkEngine{
		removeFn: func(string, bool) error {
			return &errorhandling.ErrorModel{
				ResponseCode: http.StatusNotFound,
				Message:      "no such network",
			}
		},
	}
	service := NewNetworkService(fakeEngineProvider{engine: engine})

	err := service.Delete(context.Background(), "user", "host", "ghost-net", false)

	require.ErrorIs(t, err, appErrors.ErrNetworkNotFound)
	var appErr *appErrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, http.StatusNotFound, appErr.Status)
	assert.Equal(t, "NETWORK_NOT_FOUND", appErr.Code)
}

// TestNetworkService_Delete_MapsInUseTo409 pins the in-use refusal: 409
// NETWORK_IN_USE carrying podman's refusal text, so the UI can explain
// what is attached. The error shape is podman's real one — the compat
// handler answers a 500 whose message ends in the ErrNetworkInUse text,
// not a 409 — so this test fails if the conflict vocabulary stops
// recognising "is being used".
func TestNetworkService_Delete_MapsInUseTo409(t *testing.T) {
	const refusal = `"frontend" has associated containers with it. Use -f to forcibly delete containers and pods: network is being used`
	engine := &fakeNetworkEngine{
		removeFn: func(string, bool) error {
			return &errorhandling.ErrorModel{
				ResponseCode: http.StatusInternalServerError,
				Message:      refusal,
			}
		},
	}
	service := NewNetworkService(fakeEngineProvider{engine: engine})

	err := service.Delete(context.Background(), "user", "host", "frontend", false)

	var appErr *appErrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, http.StatusConflict, appErr.Status)
	assert.Equal(t, "NETWORK_IN_USE", appErr.Code)
	assert.Contains(t, appErr.Message, refusal,
		"podman's refusal text must reach the frontend")
}

// TestNetworkService_Delete_PropagatesEngineFailure pins the unclassified
// branch: anything else is wrapped with the domain's wrap text and rides
// the envelope's 500 fallback.
func TestNetworkService_Delete_PropagatesEngineFailure(t *testing.T) {
	engine := &fakeNetworkEngine{
		removeFn: func(string, bool) error {
			return errors.New("boom")
		},
	}
	service := NewNetworkService(fakeEngineProvider{engine: engine})

	err := service.Delete(context.Background(), "user", "host", "frontend", false)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to remove network")
}

func TestNetworkService_Delete_AcquireFailureIsThe502Sentinel(t *testing.T) {
	service := NewNetworkService(fakeEngineProvider{err: appErrors.ErrConnectionFailed})

	err := service.Delete(context.Background(), "user", "host", "frontend", false)

	require.ErrorIs(t, err, appErrors.ErrConnectionFailed)
	var appErr *appErrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, http.StatusBadGateway, appErr.Status)
}
