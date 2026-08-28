package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestHasAtLeast_Ordering pins the permission ladder the access model
// is built on: read < write < admin. A caller holding `actual` may act
// when a route demands `required` iff actual is at or above required.
// The expectations below are literals from the spec (CONTEXT.md
// "Application-level permissions enforced: read, write, admin"), not
// values recomputed from the implementation.
func TestHasAtLeast_Ordering(t *testing.T) {
	cases := []struct {
		name     string
		required string
		actual   string
		want     bool
	}{
		{"read satisfies read", PermissionRead, PermissionRead, true},
		{"write satisfies read", PermissionRead, PermissionWrite, true},
		{"admin satisfies read", PermissionRead, PermissionAdmin, true},
		{"write satisfies write", PermissionWrite, PermissionWrite, true},
		{"admin satisfies write", PermissionWrite, PermissionAdmin, true},
		{"read does not satisfy write", PermissionWrite, PermissionRead, false},
		{"admin satisfies admin", PermissionAdmin, PermissionAdmin, true},
		{"write does not satisfy admin", PermissionAdmin, PermissionWrite, false},
		{"read does not satisfy admin", PermissionAdmin, PermissionRead, false},

		// No access row (repository returns the empty string) never
		// satisfies any requirement — not even read.
		{"missing row does not satisfy read", PermissionRead, "", false},
		{"missing row does not satisfy write", PermissionWrite, "", false},

		// Unknown levels deny rather than escalate: a corrupted or
		// future permission string must not open doors.
		{"unknown actual does not satisfy read", PermissionRead, "owner", false},
		{"unknown required denies even admin", "superuser", PermissionAdmin, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, HasAtLeast(tc.required, tc.actual),
				"HasAtLeast(required=%q, actual=%q)", tc.required, tc.actual)
		})
	}
}
