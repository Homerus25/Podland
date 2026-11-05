import { useHost } from "@features/hosts/hooks/useHosts";
import { type Capability, can } from "@features/hosts/permissions";

// The gate's answer: pending while the host query is in flight (the
// caller's level is unknown), then the permission answer.
export type CapabilityGate = "pending" | "allowed" | "denied";

// The detail-page capability gate, the twin of the list-side canManage
// pattern: lists receive their host eagerly from the parent page,
// detail pages must fetch it — the pending state exists only because of
// that fetch. The hook composes the host detail query with the
// capability predicate (which stays pure — no React, no queries) and owns
// the whole classification, so no page can re-derive it or drop the
// error branch: a failed host query classifies as denied
// (error-is-denied), never a silent gap. Pages render from the enum:
// pending renders neither control nor hint (never flash the read-only
// hint at a write-capable caller), allowed renders the control, denied
// renders the shared permission hint.
export function useCapabilityGate(
	hostId: string,
	capability: Capability,
): CapabilityGate {
	const { data: host, error } = useHost(hostId);
	if (host) {
		return can(host, capability) ? "allowed" : "denied";
	}
	if (error) {
		return "denied";
	}
	return "pending";
}
