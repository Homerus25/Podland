import type { RefusalVocabulary } from "@shared/components/MutationDialog";

// The network removal vocabulary (spec 461, ticket #465): the refusal code
// the server answers with when containers are still attached to the
// network, plus the hint copy and its testid. The network detail page's
// Remove action reads it from here; the lists' per-row delete modal that
// used to share it is gone (spec 461, ticket #470). The force-refusal
// ritual itself is the shared ForceConfirmDialog preset's; this module owns
// the domain words only.
export const NETWORK_IN_USE_REFUSAL = {
	code: "NETWORK_IN_USE",
	hint: "One or more containers are attached to this network, so it cannot be removed normally. Force remove removes those containers — stopping any that are running — and then the network.",
	hintTestId: "network-in-use-warning",
} as const satisfies RefusalVocabulary;
