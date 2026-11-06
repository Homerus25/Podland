import type { RefusalVocabulary } from "@shared/components/MutationDialog";

// The image removal vocabulary (spec 461, ticket #465): the refusal code
// the server answers with when containers still use the image, plus the
// hint copy and its testid. The image detail page's Remove action reads it
// from here; the lists' per-row delete modal that used to share it is gone
// (spec 461, ticket #470). The force-refusal ritual itself is the shared
// ForceConfirmDialog preset's; this module owns the domain words only.
export const IMAGE_IN_USE_REFUSAL = {
	code: "IMAGE_USED_BY_CONTAINERS",
	hint: "One or more containers use this image, so it cannot be removed normally. Force remove removes the image even though containers still use it; those containers may stop working.",
	hintTestId: "image-in-use-warning",
} as const satisfies RefusalVocabulary;
