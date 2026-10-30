import { useMachine } from "@features/machines/hooks/useMachines";
import { type Capability, can } from "@features/machines/permissions";

// The gate's answer: pending while the machine query is in flight (the
// caller's level is unknown), then the permission answer.
export type CapabilityGate = "pending" | "allowed" | "denied";

// The detail-page capability gate, the twin of the list-side canManage
// pattern: lists receive their machine eagerly from the parent page,
// detail pages must fetch it — the pending state exists only because of
// that fetch. The hook composes the machine detail query with the
// capability predicate (which stays pure — no React, no queries) and owns
// the whole classification, so no page can re-derive it or drop the
// error branch: a failed machine query classifies as denied
// (error-is-denied), never a silent gap. Pages render from the enum:
// pending renders neither control nor hint (never flash the read-only
// hint at a write-capable caller), allowed renders the control, denied
// renders the shared permission hint.
export function useCapabilityGate(
	machineId: string,
	capability: Capability,
): CapabilityGate {
	const { data: machine, error } = useMachine(machineId);
	if (machine) {
		return can(machine, capability) ? "allowed" : "denied";
	}
	if (error) {
		return "denied";
	}
	return "pending";
}
