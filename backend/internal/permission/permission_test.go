package permission

import (
	"errors"
	"testing"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeGetter stands in for the machine-access repository: it answers
// every lookup with the armed level (or error).
type fakeGetter struct {
	level string
	err   error
}

func (f fakeGetter) GetPermission(machineID, userID string) (string, error) {
	return f.level, f.err
}

// TestRequireLevel_LevelLadder pins the gate's decision table once, as
// literals: a held level at or above the required one passes (nil);
// anything below it — an empty row (no machine_access row at all) and
// an unknown level included — is denied with 403 ACCESS_DENIED, never
// a silent allow/deny guess.
func TestRequireLevel_LevelLadder(t *testing.T) {
	cases := []struct {
		name     string
		required string
		held     string
		want     error
	}{
		// required read: every known level passes
		{"read passes read", models.PermissionRead, models.PermissionRead, nil},
		{"read passes write", models.PermissionRead, models.PermissionWrite, nil},
		{"read passes admin", models.PermissionRead, models.PermissionAdmin, nil},
		{"read denies empty row", models.PermissionRead, "", appErrors.ErrAccessDenied},
		{"read denies unknown level", models.PermissionRead, "superuser", appErrors.ErrAccessDenied},

		// required write: write and admin pass, read does not
		{"write denies read", models.PermissionWrite, models.PermissionRead, appErrors.ErrAccessDenied},
		{"write passes write", models.PermissionWrite, models.PermissionWrite, nil},
		{"write passes admin", models.PermissionWrite, models.PermissionAdmin, nil},
		{"write denies empty row", models.PermissionWrite, "", appErrors.ErrAccessDenied},
		{"write denies unknown level", models.PermissionWrite, "superuser", appErrors.ErrAccessDenied},

		// required admin: only admin passes
		{"admin denies read", models.PermissionAdmin, models.PermissionRead, appErrors.ErrAccessDenied},
		{"admin denies write", models.PermissionAdmin, models.PermissionWrite, appErrors.ErrAccessDenied},
		{"admin passes admin", models.PermissionAdmin, models.PermissionAdmin, nil},
		{"admin denies empty row", models.PermissionAdmin, "", appErrors.ErrAccessDenied},
		{"admin denies unknown level", models.PermissionAdmin, "superuser", appErrors.ErrAccessDenied},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := RequireLevel(fakeGetter{level: tc.held}, "machine-1", "user-1", tc.required)

			if tc.want == nil {
				assert.NoError(t, err, "a satisfied level must pass the gate")
			} else {
				require.Error(t, err, "a below-required level must be denied")
				assert.ErrorIs(t, err, tc.want, "the denial must be the bare ACCESS_DENIED sentinel")
			}
		})
	}
}

// TestRequireLevel_LookupErrorIsInternalError pins the failure side of
// the contract: a failing permission lookup is 500 INTERNAL_ERROR,
// never a silent allow/deny guess.
func TestRequireLevel_LookupErrorIsInternalError(t *testing.T) {
	err := RequireLevel(fakeGetter{err: errors.New("db down")}, "machine-1", "user-1", models.PermissionRead)

	require.Error(t, err, "a failing lookup must not pass the gate")
	assert.ErrorIs(t, err, appErrors.ErrInternalError, "the lookup failure must surface as the bare INTERNAL_ERROR sentinel")
}
