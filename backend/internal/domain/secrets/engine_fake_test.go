package secrets

import (
	"context"
	"net/http"
	"testing"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/transfer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/pkg/domain/entities"
	errorhandling "go.podman.io/podman/v6/pkg/errorhandling"
)

// fakeSecretEngine is the function-field fake for the SecretEngine seam.
// Each test arms only the operations its scenario needs; an unarmed
// operation panics, so an unexpected engine call fails the test loudly.
type fakeSecretEngine struct {
	listFn    func() ([]*entities.SecretInfoReport, error)
	inspectFn func(name string, reveal bool) (*entities.SecretInfoReport, error)
	createFn  func(name, value string) (*entities.SecretCreateReport, error)
	removeFn  func(name string) error
}

func (f *fakeSecretEngine) List() ([]*entities.SecretInfoReport, error) {
	if f.listFn == nil {
		panic("unexpected SecretEngine.List call")
	}
	return f.listFn()
}

func (f *fakeSecretEngine) Inspect(name string, reveal bool) (*entities.SecretInfoReport, error) {
	if f.inspectFn == nil {
		panic("unexpected SecretEngine.Inspect call")
	}
	return f.inspectFn(name, reveal)
}

func (f *fakeSecretEngine) Create(name, value string) (*entities.SecretCreateReport, error) {
	if f.createFn == nil {
		panic("unexpected SecretEngine.Create call")
	}
	return f.createFn(name, value)
}

func (f *fakeSecretEngine) Remove(name string) error {
	if f.removeFn == nil {
		panic("unexpected SecretEngine.Remove call")
	}
	return f.removeFn(name)
}

// fakeEngineProvider arms the provider seam: engine is handed to the
// service closure; err simulates an acquisition failure (the 502 sentinel
// crossing) and short-circuits the closure, exactly like the production
// manager's warn-and-sentinel behaviour.
type fakeEngineProvider struct {
	engine SecretEngine
	err    error
	// engines serves per-machine engines keyed by machine ID — the shape
	// the cross-machine copy/move exercises. When non-nil it takes
	// precedence over engine; a lookup miss panics, so a closure crossing
	// an unexpected machine fails the test loudly.
	engines map[string]SecretEngine
}

func (p fakeEngineProvider) WithEngine(ctx context.Context, userID, machineID, op string, fn func(e SecretEngine) error) error {
	if p.err != nil {
		return p.err
	}
	if p.engines != nil {
		e, ok := p.engines[machineID]
		if !ok {
			panic("unexpected WithEngine machine: " + machineID)
		}
		return fn(e)
	}
	return fn(p.engine)
}

// fakeUsageEngine is the function-field fake for the kit's
// transfer.QuadletReader seam (issue 260, §19) — the width the usage
// provider hands out. Each test arms only the operations its scenario
// needs; an unarmed operation panics, so an unexpected engine call
// fails the test loudly.
type fakeUsageEngine struct {
	listQuadletNamesFn func() ([]string, error)
	printQuadletFn     func(name string) (string, error)
}

func (f *fakeUsageEngine) ListQuadletNames() ([]string, error) {
	if f.listQuadletNamesFn == nil {
		panic("unexpected QuadletReader.ListQuadletNames call")
	}
	return f.listQuadletNamesFn()
}

func (f *fakeUsageEngine) PrintQuadlet(name string) (string, error) {
	if f.printQuadletFn == nil {
		panic("unexpected QuadletReader.PrintQuadlet call")
	}
	return f.printQuadletFn(name)
}

// fakeUsageEngineProvider serves per-machine quadlet readers keyed by
// machine ID; a lookup miss panics, so a closure crossing an unexpected
// machine fails the test loudly.
type fakeUsageEngineProvider struct {
	engines map[string]transfer.QuadletReader
	err     error
}

func (p fakeUsageEngineProvider) WithEngine(ctx context.Context, userID, machineID, op string, fn func(e transfer.QuadletReader) error) error {
	if p.err != nil {
		return p.err
	}
	e, ok := p.engines[machineID]
	if !ok {
		panic("unexpected usage WithEngine machine: " + machineID)
	}
	return fn(e)
}

// TestSecretService_AcquireFailureIsThe502Sentinel pins the service-side
// half of the connection-failure contract: the provider's sentinel crosses
// untouched — the service never wraps, maps, or swallows it (the 502
// mapping itself is pinned in the connections package). The other service
// behaviours run against a fake podman service API through the production
// adapter (see service_test.go).
func TestSecretService_AcquireFailureIsThe502Sentinel(t *testing.T) {
	service := NewSecretService(fakeEngineProvider{err: appErrors.ErrConnectionFailed}, nil)

	result, err := service.List(context.Background(), "user", "machine")

	require.ErrorIs(t, err, appErrors.ErrConnectionFailed)
	assert.Nil(t, result)
	var appErr *appErrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, http.StatusBadGateway, appErr.Status)
}

// TestSecretService_Create_MapsAlreadyExists pins the duplicate-name
// mapping at the service level: the engine's name-in-use answer (podman's
// 500 ErrorModel whose cause carries the "name in use" sentinel — the wire
// shape is pinned in service_test.go) surfaces as ErrSecretAlreadyExists.
func TestSecretService_Create_MapsAlreadyExists(t *testing.T) {
	engine := &fakeSecretEngine{
		createFn: func(string, string) (*entities.SecretCreateReport, error) {
			return nil, &errorhandling.ErrorModel{
				ResponseCode: http.StatusInternalServerError,
				Message:      "db-password: conflict",
				Because:      "secret name in use",
			}
		},
	}
	service := NewSecretService(fakeEngineProvider{engine: engine}, nil)

	result, err := service.Create(context.Background(), "user", "machine",
		SecretCreateRequest{Name: "db-password", Value: "hunter2"})

	assert.ErrorIs(t, err, appErrors.ErrSecretAlreadyExists)
	assert.Nil(t, result)
}

// TestSecretService_Get_RevealDeniedNeverTouchesEngine pins that the
// admin gate fires before the connection lease: a below-admin reveal is
// refused without arming (or reaching) the engine at all.
func TestSecretService_Get_RevealDeniedNeverTouchesEngine(t *testing.T) {
	service := NewSecretService(fakeEngineProvider{engine: &fakeSecretEngine{}}, &denyRepo{})

	result, err := service.Get(context.Background(), "reader-1", "machine", "db-password", true)

	assert.ErrorIs(t, err, appErrors.ErrAccessDenied)
	assert.Nil(t, result)
}

// denyRepo answers every permission lookup with a non-admin level.
type denyRepo struct{}

func (denyRepo) GetPermission(machineID, userID string) (string, error) {
	return "read", nil
}
