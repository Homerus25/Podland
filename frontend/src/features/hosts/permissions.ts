import type { Host, HostPermission } from "@shared/types/api";

// Single source of truth for permission-derived UI gating. The backend
// remains the enforcement point; this predicate only decides which
// controls render, so users are never baited into 403s.
//
// The table mirrors the backend's enforcement exactly, from its two
// sources: the route tree (backend/internal/app/app.go) gates every
// resource family's mutating routes at write level via requireWrite and
// keeps the secret value reveal at admin level (the secrets service
// answers 403 below admin), and the host service
// (backend/internal/domain/hosts/service.go) answers host edit,
// share, and delete to the owner only. The backend grants the host
// owner an admin access row at creation time, so an owner's
// `permission` already reads 'admin'; owner capabilities are answered
// from the `isOwner` enrichment fact instead of the rank.

// The access-level ladder, private to this module: a capability backed
// by a level is granted when the caller's stored level ranks at or
// above the requirement.
const LEVEL_RANK: Record<HostPermission, number> = {
	read: 0,
	write: 1,
	admin: 2,
};

// The capability table, private to this module: every UI-gated act maps
// to the backend gate that enforces it — a required level, or the
// owner-only fact.
const CAPABILITY_TABLE = {
	// Resource management (create/delete/lifecycle/prune/populate/pull)
	// mutates the host's remote state; write by rank.
	"manage-containers": "write",
	"manage-pods": "write",
	"manage-networks": "write",
	"manage-volumes": "write",
	"manage-images": "write",
	"manage-quadlets": "write",
	"manage-secrets": "write",
	// Revealing a secret's value is the admin-only capability.
	"reveal-secrets": "admin",
	// Metadata edits, sharing, and deletion are host-owner acts
	// (the backend service rejects every non-owner caller).
	"edit-host": "owner",
	"share-host": "owner",
	"delete-host": "owner",
} as const satisfies Record<string, keyof typeof LEVEL_RANK | "owner">;

export type Capability = keyof typeof CAPABILITY_TABLE;

// True when the host's per-caller facts grant the capability. Both
// facts are always present on a host the UI holds — the list and the
// detail responses enrich alike (backend AccessibleHostResponse) —
// so the predicate reads them directly.
export function can(host: Host, capability: Capability): boolean {
	const requirement = CAPABILITY_TABLE[capability];
	if (requirement === "owner") {
		return host.isOwner;
	}
	return LEVEL_RANK[host.permission] >= LEVEL_RANK[requirement];
}
