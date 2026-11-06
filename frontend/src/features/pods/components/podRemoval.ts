import type { RefusalVocabulary } from "@shared/components/MutationDialog";

// The pod removal vocabulary (spec 461, ticket #465): the refusal code the
// server answers with when the pod still has running containers, plus the
// hint copy and its testid. The pod detail page's Remove action reads it
// from here; the pods list's per-row remove modal that used to share it is
// gone (spec 461, ticket #470). The force-refusal ritual itself is the
// shared ForceConfirmDialog preset's; this module owns the domain words
// only.
export const POD_RUNNING_REFUSAL = {
	code: "POD_STATE_CONFLICT",
	hint: "This pod has running containers, so it cannot be removed normally. Force remove also removes its running containers; they may lose work.",
	hintTestId: "pod-running-warning",
} as const satisfies RefusalVocabulary;
