import type { ImageSummary } from "@features/images/services/imagesApi";
import type { VolumeSummary } from "@features/volumes/services/volumesApi";

// Shared test fixture factories: one authoritative shape per DTO, imported
// by any test that needs a valid object with `Partial` overrides. Keep
// values distinctive enough to assert against without being tautological.
export function volumeFixture(
	overrides: Partial<VolumeSummary> = {},
): VolumeSummary {
	return {
		name: "app-data",
		driver: "local",
		createdAt: "2026-08-20T10:00:00Z",
		anonymous: false,
		labels: {},
		options: {},
		mountpoint: "/var/lib/containers/storage/volumes/app-data/_data",
		...overrides,
	};
}

export function imageFixture(
	overrides: Partial<ImageSummary> = {},
): ImageSummary {
	return {
		id: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		repoTags: ["docker.io/library/nginx:1.27"],
		repoDigests: [],
		createdAt: "2026-08-18T10:00:00Z",
		size: 157286400,
		...overrides,
	};
}
