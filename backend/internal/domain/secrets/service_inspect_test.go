package secrets

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/pkg/domain/entities"
	errorhandling "go.podman.io/podman/v6/pkg/errorhandling"
)

// --- Service seam: Inspect (raw report, reveal never passed) ---

// TestSecretService_Inspect_ReturnsRawReport pins that Inspect returns the
// engine's report untouched — the raw *entities.SecretInfoReport, not the
// Get endpoint's DTO mapping (ToSecretInspectResponse stays Get's).
func TestSecretService_Inspect_ReturnsRawReport(t *testing.T) {
	report := secretFixture("3f2e1d0c", "db-password", mustParseSecretTime("2026-01-02T15:04:05Z"), "")
	engine := &fakeSecretEngine{
		inspectFn: func(name string, reveal bool) (*entities.SecretInfoReport, error) {
			return report, nil
		},
	}
	service := NewSecretService(fakeEngineProvider{engine: engine}, nil)

	result, err := service.Inspect(context.Background(), "reader-1", "host", "db-password")

	require.NoError(t, err)
	assert.Same(t, report, result, "inspect must return the engine's raw report, not a DTO mapping")
}

// TestSecretService_Inspect_NeverPassesReveal pins the redaction contract:
// the engine's inspect is called with reveal=false — for ANY caller,
// including an admin (the raw inspect endpoint has no reveal path at all,
// so the report podman returns cannot carry the value).
func TestSecretService_Inspect_NeverPassesReveal(t *testing.T) {
	var gotReveals []bool
	engine := &fakeSecretEngine{
		inspectFn: func(name string, reveal bool) (*entities.SecretInfoReport, error) {
			gotReveals = append(gotReveals, reveal)
			return secretFixture("3f2e1d0c", "db-password", time.Time{}, ""), nil
		},
	}
	// adminRepo answers admin: even an admin caller must not turn the raw
	// inspect into a reveal.
	service := NewSecretService(fakeEngineProvider{engine: engine}, adminRepo{})

	_, err := service.Inspect(context.Background(), "admin-1", "host", "db-password")

	require.NoError(t, err)
	assert.Equal(t, []bool{false}, gotReveals,
		"the engine inspect must be called with reveal disabled — never true")
}

// TestSecretService_Inspect_NotFound pins the 404 mapping: a podman
// not-found answer surfaces as the domain's SECRET_NOT_FOUND sentinel.
func TestSecretService_Inspect_NotFound(t *testing.T) {
	engine := &fakeSecretEngine{
		inspectFn: func(name string, reveal bool) (*entities.SecretInfoReport, error) {
			return nil, &errorhandling.ErrorModel{
				ResponseCode: http.StatusNotFound,
				Message:      "no such secret with name or id \"nope\"",
				Because:      "no such secret",
			}
		},
	}
	service := NewSecretService(fakeEngineProvider{engine: engine}, nil)

	result, err := service.Inspect(context.Background(), "reader-1", "host", "nope")

	assert.ErrorIs(t, err, appErrors.ErrSecretNotFound)
	assert.Nil(t, result)
}

// TestSecretService_Inspect_EngineFailureWraps pins that any other engine
// failure wraps "failed to inspect secret" and stays a non-AppError, so the
// handler answers 500 — never a masquerading 404.
func TestSecretService_Inspect_EngineFailureWraps(t *testing.T) {
	engine := &fakeSecretEngine{
		inspectFn: func(name string, reveal bool) (*entities.SecretInfoReport, error) {
			return nil, errors.New("connection reset")
		},
	}
	service := NewSecretService(fakeEngineProvider{engine: engine}, nil)

	result, err := service.Inspect(context.Background(), "reader-1", "host", "db-password")

	require.Error(t, err)
	assert.ErrorContains(t, err, "failed to inspect secret")
	var appErr *appErrors.AppError
	assert.NotErrorAs(t, err, &appErr, "the wrapped error stays non-AppError so the handler 500s it")
	assert.Nil(t, result)
}

// TestSecretService_Inspect_AcquireFailureIsThe502Sentinel pins the
// connection-failure contract for inspect: the provider's sentinel crosses
// untouched — the service never maps, wraps, or logs it (the 502
// CONNECTION_FAILED mapping lives at the connection seam).
func TestSecretService_Inspect_AcquireFailureIsThe502Sentinel(t *testing.T) {
	service := NewSecretService(fakeEngineProvider{err: appErrors.ErrConnectionFailed}, nil)

	result, err := service.Inspect(context.Background(), "user", "host", "db-password")

	require.ErrorIs(t, err, appErrors.ErrConnectionFailed)
	assert.Nil(t, result)
	var appErr *appErrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, http.StatusBadGateway, appErr.Status)
}
