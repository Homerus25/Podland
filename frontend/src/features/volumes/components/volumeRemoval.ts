import type { RefusalVocabulary } from "@shared/components/MutationDialog";

// The volume removal vocabulary (spec 461, ticket #465): the refusal code
// podman answers with when containers still use the volume, plus the hint
// copy and its testid. The volume detail page's Remove action reads it from
// here; the lists' per-row delete modal that used to share it is gone
// (spec 461, ticket #470). The force-refusal ritual itself is the shared
// ForceConfirmDialog preset's; this module owns the domain words only.
export const VOLUME_IN_USE_REFUSAL = {
	code: "VOLUME_IN_USE",
	hint: "One or more containers use this volume, so it cannot be removed normally. Force remove stops those containers and removes the volume.",
	hintTestId: "volume-in-use-warning",
} as const satisfies RefusalVocabulary;
