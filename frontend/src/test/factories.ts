import type { ContainerSummary } from "@features/containers/services/containersApi";
import type { ImageSummary } from "@features/images/services/imagesApi";
import type { QuadletSummary } from "@features/quadlets/services/quadletsApi";
import type { SecretSummary } from "@features/secrets/services/secretsApi";
import type { VolumeSummary } from "@features/volumes/services/volumesApi";
import type { Machine } from "@shared/types/api";

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

export function containerFixture(
	overrides: Partial<ContainerSummary> = {},
): ContainerSummary {
	return {
		id: "c-1",
		names: ["/my-container"],
		image: "docker.io/library/nginx:1.27",
		createdAt: "2026-08-20T10:00:00Z",
		state: "running",
		status: "Up 2 hours",
		restarts: 0,
		exited: false,
		...overrides,
	};
}

export function quadletFixture(
	overrides: Partial<QuadletSummary> = {},
): QuadletSummary {
	return {
		id: "web",
		name: "web",
		path: "/etc/containers/systemd/web.container",
		unit: "web.service",
		resource: "container",
		state: "running",
		...overrides,
	};
}

export function secretFixture(
	overrides: Partial<SecretSummary> = {},
): SecretSummary {
	return {
		id: "3f2e1d0c",
		name: "db-password",
		driver: "file",
		createdAt: "2026-08-20T10:00:00Z",
		updatedAt: "2026-08-20T10:00:00Z",
		...overrides,
	};
}

export function machineFixture(overrides: Partial<Machine> = {}): Machine {
	return {
		id: "m-1",
		name: "prod-web-01",
		description: "Production web server",
		hostname: "web01.example.com",
		port: 22,
		sshUsername: "deploy",
		sshKeyFingerprint: "SHA256:aaa",
		podmanSocketPath: "/run/podman/podman.sock",
		ownerId: "u-owner",
		permission: "admin",
		createdAt: "2026-08-01T10:00:00Z",
		updatedAt: "2026-08-01T10:00:00Z",
		...overrides,
	};
}
