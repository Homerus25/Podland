import type { RefusalVocabulary } from "@shared/components/MutationDialog";

// The container removal vocabulary (spec 461, tickets #462 and #467): the
// backend's refusal sentinel for removing a running container without
// force, plus the hint copy and its testid. The container detail page's
// Remove action reads it from here; the lists' per-row delete modal that
// used to exist is gone (spec 461, ticket #470). The force-refusal ritual
// itself is the shared ForceConfirmDialog preset's; this module owns the
// domain words only.
export const CONTAINER_RUNNING_REFUSAL = {
	code: "CONTAINER_RUNNING",
	hint: "This container is running, so it cannot be removed normally. Force remove stops it and then removes it; anything it was doing is lost.",
	hintTestId: "container-running-warning",
} as const satisfies RefusalVocabulary;
