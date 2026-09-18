// Package permission owns the permission-level gate: the authorization
// contract — permission lookup fails ⇒ 500, stored level below the
// required one ⇒ 403, never a silent allow/deny guess — declared once
// and shared by every caller that compares a caller's stored
// machine_access level against a requirement. The route middleware
// (RequireMachinePermission) and in-service gates (the secrets reveal)
// are its callers; the HTTP-shaped machine-resolution ritual stays
// with the middleware.
package permission

import (
	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/models"
)

// PermissionGetter is the lookup the level gate needs: the caller's
// stored permission level for a machine (empty string when no row
// exists). *models.MachineAccessRepository and the test mocks satisfy
// it structurally.
type PermissionGetter interface {
	GetPermission(machineID, userID string) (string, error)
}

// RequireLevel decides the level gate for one (machine, user) pair:
// the caller's stored level must be at or above `required`
// (read < write < admin, via models.HasAtLeast). It returns the bare
// AppError sentinels — a failing lookup is appErrors.ErrInternalError,
// a level below `required` (an empty row included) is
// appErrors.ErrAccessDenied — so callers speak their native currency:
// the middleware maps them to the error envelope, a service returns
// them as-is. A nil answer means the gate passes.
func RequireLevel(repo PermissionGetter, machineID, userID, required string) error {
	level, err := repo.GetPermission(machineID, userID)
	if err != nil {
		return appErrors.ErrInternalError
	}
	if !models.HasAtLeast(required, level) {
		return appErrors.ErrAccessDenied
	}
	return nil
}
