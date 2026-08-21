import type { Machine, MachinePermission } from '@shared/types/api'

// Single source of truth for permission-derived UI gating. The backend
// remains the enforcement point; these predicates only decide which
// controls render, so users are never baited into 403s.
//
// The backend grants the machine owner an admin access row at creation
// time (backend/internal/domain/machines/service.go), so a machine's
// permission already reads 'admin' for its owner. Effective permission is
// therefore just the machine permission; owner → admin is handled upstream.

export type EffectivePermission = MachinePermission

export function effectivePermission(machine: Machine): EffectivePermission {
  return machine.permission
}

// Metadata edits (name, description, hostname, port, SSH username, key).
export function canEditMachine(permission: EffectivePermission): boolean {
  return permission === 'admin' || permission === 'write'
}

// Sharing (grant/revoke access) and deletion are admin/owner-only.
export function canShareMachine(permission: EffectivePermission): boolean {
  return permission === 'admin'
}

export function canDeleteMachine(permission: EffectivePermission): boolean {
  return permission === 'admin'
}
