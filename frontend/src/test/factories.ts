import type { AuthStatusResponse } from "@features/auth/services/authApi";
import type {
	ContainerDetail,
	ContainerStats,
	ContainerStatsListResult,
	ContainerSummary,
} from "@features/containers/services/containersApi";
import type { HostVersion } from "@features/hosts/services/hostsApi";
import type {
	ImageCopyPreview,
	ImageCopyResult,
	ImageDetail,
	ImageSummary,
} from "@features/images/services/imagesApi";
import type {
	NetworkDetail,
	NetworkSummary,
} from "@features/networks/services/networksApi";
import type { PodDetail, PodSummary } from "@features/pods/services/podsApi";
import type {
	QuadletDetail,
	QuadletImportEntry,
	QuadletImportResponse,
	QuadletMoveCompanionState,
	QuadletMovePreview,
	QuadletMovePreviewSecret,
	QuadletMovePreviewVolume,
	QuadletMoveResult,
	QuadletMoveSecretOutcome,
	QuadletMoveVolumeOutcome,
	QuadletSummary,
} from "@features/quadlets/services/quadletsApi";
import type {
	SecretMovePreview,
	SecretMoveResult,
	SecretSummary,
} from "@features/secrets/services/secretsApi";
import type { UserSummary } from "@features/users/services/usersApi";
import type {
	VolumeMovePreview,
	VolumeMoveResult,
	VolumeSummary,
} from "@features/volumes/services/volumesApi";
import type { Host } from "@shared/types/api";

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

// --- spec 301 #308: image detail page fixture -----------------------------
// Image detail (GET .../images/{imageId}): the list fields combined with
// the inspect report's config fields. Distinctive values so tests can
// assert against them without being tautological.
export function imageDetailFixture(
	overrides: Partial<ImageDetail> = {},
): ImageDetail {
	return {
		id: "sha256:abc123def4567890abc123def4567890abc123def4567890abc123def4567890",
		repoTags: [
			"docker.io/library/nginx:1.27",
			"docker.io/library/nginx:latest",
		],
		repoDigests: ["docker.io/library/nginx@sha256:def4567890abc"],
		createdAt: "2026-08-18T10:00:00Z",
		size: 157286400,
		os: "linux",
		architecture: "amd64",
		user: "appuser",
		env: {
			PATH: "/usr/local/sbin:/usr/local/bin",
			NGINX_VERSION: "1.27.1",
		},
		entrypoint: ["/docker-entrypoint.sh"],
		cmd: ["nginx", "-g", "daemon off;"],
		labels: { maintainer: "NGINX Official Image", tier: "frontend" },
		...overrides,
	};
}
// --- end spec 301 #308 ----------------------------------------------------

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

// --- spec 301 ticket #306: container detail fixture ---------------------
// Container detail (GET .../containers/{id}): the extended
// ContainerGetResponse DTO. Default shape is a running web container with
// one named volume, one mount, one network, one exposed port, one secret,
// and a two-variable environment; tests override per scenario.
export function containerDetailFixture(
	overrides: Partial<ContainerDetail> = {},
): ContainerDetail {
	return {
		id: "c-1",
		name: "web",
		state: "running",
		status: "Up 2 hours",
		image: "docker.io/library/nginx:1.27",
		imageId:
			"sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		createdAt: "2026-08-20T10:00:00Z",
		command: ["nginx", "-g", "daemon off;"],
		entrypoint: ["/docker-entrypoint.sh"],
		secrets: [{ name: "db-password", uid: 0, gid: 0, mode: 420 }],
		volumes: [
			{
				name: "app-data",
				source: "/var/lib/containers/storage/volumes/app-data/_data",
				destination: "/data",
				rw: true,
			},
		],
		exposedPorts: [{ containerPort: 8080, protocol: "tcp" }],
		environment: { FOO: "bar", MODE: "prod" },
		mounts: [
			"type=volume,source=/var/lib/containers/storage/volumes/app-data/_data,destination=/data",
		],
		networks: ["app-net"],
		restarts: 0,
		exited: false,
		// The current limits and healthcheck (spec 412): the
		// edit-configuration form's prefill baseline.
		limits: { cpus: 1.5, memory: "512m" },
		healthcheck: {
			command: "curl -f http://localhost/health || exit 1",
			interval: "30s",
			timeout: "5s",
			startPeriod: "0m30s",
			retries: 3,
			disabled: false,
		},
		...overrides,
	};
}
// --- end spec 301 ticket #306 -------------------------------------------

// --- spec 415 ticket #417: container stats fixture ----------------------
// Container stats (GET .../containers/stats and .../containers/{id}/stats):
// the one-shot snapshot DTO. All values are raw numbers on the wire; the
// frontend formats, so the defaults are picked to have unambiguous
// formatBytes outputs ("1.5 KB", "2 GB", …).
export function containerStatsFixture(
	overrides: Partial<ContainerStats> = {},
): ContainerStats {
	return {
		id: "c-1",
		name: "web",
		cpuPercent: 12.34,
		memoryUsage: 1500,
		memoryLimit: 2_000_000_000,
		memoryPercent: 42.1,
		networkBytesIn: 999,
		networkBytesOut: 1_500_000,
		blockBytesIn: 250,
		blockBytesOut: 3_000_000_000,
		pids: 42,
		...overrides,
	};
}

// Envelope of GET .../containers/stats (backend ContainerStatsResult,
// mirroring the containers/pods list envelopes): the snapshot's rows with
// the count and request timestamp. Tests pass their rows; the count always
// matches them.
export function containerStatsListFixture(
	containers: ContainerStats[],
): ContainerStatsListResult {
	return {
		containers,
		total: containers.length,
		timestamp: "2026-10-30T00:00:00Z",
	};
}
// --- end spec 415 ticket #417 -------------------------------------------

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

// The single-quadlet GET's response (backend QuadletGetResponse; issue
// #472): the summary shape plus the file content the detail page renders.
export function quadletDetailFixture(
	overrides: Partial<QuadletDetail> = {},
): QuadletDetail {
	return {
		id: "web",
		name: "web",
		path: "/etc/containers/systemd/web.container",
		unit: "web.service",
		resource: "container",
		state: "running",
		content: "[Container]\nImage=docker.io/library/nginx:1.27\n",
		...overrides,
	};
}

// One entry of the import/convert entry shape {name, resource, content}
// (backend QuadletImportEntry; spec #336): the quadlet name without
// extension, its resource type, and the file content. The convert-compose
// response carries this shape, so tests build both bodies from it.
export function quadletImportEntryFixture(
	overrides: Partial<QuadletImportEntry> = {},
): QuadletImportEntry {
	return {
		name: "web",
		resource: "container",
		content: "[Container]\nImage=docker.io/library/nginx:1.27\n",
		...overrides,
	};
}

// Response of POST /api/hosts/{id}/quadlets/import -> 201
// (backend QuadletImportResponse; spec #336): one imported entry by
// default; tests override `imported`/`failed` per scenario.
export function quadletImportResponseFixture(
	overrides: Partial<QuadletImportResponse> = {},
): QuadletImportResponse {
	return {
		imported: [
			{
				id: "web",
				name: "web",
				path: "/etc/containers/systemd/web.container",
				state: "unknown",
			},
		],
		failed: [],
		...overrides,
	};
}

// Move/copy preview (GET .../quadlets/{id}/move-preview): default shape
// references one podman volume and one host bind path with a clean target;
// tests override `volumes`/`collision`/`running` per scenario. Volume-kind
// entries always carry existsOnTarget/migratable (issue 214), so the
// defaults do too.
export function quadletMovePreviewFixture(
	overrides: Partial<QuadletMovePreview> = {},
): QuadletMovePreview {
	return {
		volumes: [
			{
				source: "pgdata",
				kind: "volume",
				podmanVolume: "pgdata",
				existsOnTarget: false,
				migratable: true,
			},
			{
				source: "/srv/data",
				kind: "hostPath",
				existsOnTarget: false,
				migratable: false,
				reason: "host bind paths are not migrated",
			},
		],
		secrets: [],
		collision: false,
		running: false,
		...overrides,
	};
}

// One secret-kind preview entry (issue 215): a podman secret that exists on
// the source and is new on the target — the transferable shape. Tests
// override `existsOnSource`/`existsOnTarget` per scenario; a host-path
// reference overrides `kind` to "hostPath" (both existence flags then
// render false, the no-claims shape).
export function quadletMovePreviewSecretFixture(
	overrides: Partial<QuadletMovePreviewSecret> = {},
): QuadletMovePreviewSecret {
	return {
		name: "db-password",
		kind: "secret",
		existsOnSource: true,
		existsOnTarget: false,
		...overrides,
	};
}

// One volume-kind preview entry with the issue-214 data fields: a migratable
// reference whose resolved volume is fresh on the target. Tests override
// `existsOnTarget`/`targetSizeBytes`/`migratable`/`reason` per scenario.
export function quadletMoveDataVolumeFixture(
	overrides: Partial<QuadletMovePreviewVolume> = {},
): QuadletMovePreviewVolume {
	return {
		source: "pgdata",
		kind: "volume",
		podmanVolume: "pgdata",
		existsOnTarget: false,
		migratable: true,
		...overrides,
	};
}

// One companion .volume preview entry (issue 217): the source names the
// same-named quadlet file and the preview labels how that file sits on the
// target host. Companion references resolve to podman's `systemd-<name>`
// default unless the file sets VolumeName= (issue 214), so the entry carries
// the resolved name and the always-present data fields too.
export function quadletMoveCompanionVolumeFixture(
	targetState: QuadletMoveCompanionState,
	source = "app-data.volume",
): QuadletMovePreviewVolume {
	return {
		source,
		kind: "volume",
		quadletFile: source,
		targetState,
		podmanVolume: `systemd-${source.replace(/\.volume$/, "")}`,
		existsOnTarget: false,
		migratable: true,
	};
}

// Move/copy transfer envelope (POST .../quadlets/{id}/move): default shape
// is a completed move from m-1 to m-2.
export function quadletMoveResultFixture(
	overrides: Partial<QuadletMoveResult> = {},
): QuadletMoveResult {
	return {
		id: "web",
		name: "web",
		resource: "container",
		status: "moved",
		sourceHostId: "m-1",
		targetHostId: "m-2",
		...overrides,
	};
}

// One data-phase outcome of the volumeMigration block (issue 214): default
// shape is a migrated pgdata volume; skipped entries override `outcome` and
// carry `reason`.
export function quadletMoveVolumeOutcomeFixture(
	overrides: Partial<QuadletMoveVolumeOutcome> = {},
): QuadletMoveVolumeOutcome {
	return {
		ref: "pgdata",
		podmanVolume: "pgdata",
		outcome: "migrated",
		...overrides,
	};
}

// One secret-phase outcome of the secretTransfer block (issue 215): default
// shape is a transferred db-password; skipped entries override `outcome`
// and carry `reason`.
export function quadletMoveSecretOutcomeFixture(
	overrides: Partial<QuadletMoveSecretOutcome> = {},
): QuadletMoveSecretOutcome {
	return {
		name: "db-password",
		outcome: "transferred",
		...overrides,
	};
}

export function networkFixture(
	overrides: Partial<NetworkSummary> = {},
): NetworkSummary {
	return {
		name: "app-net",
		driver: "bridge",
		subnets: ["10.88.0.0/16"],
		createdAt: "2026-08-20T10:00:00Z",
		...overrides,
	};
}

// --- ticket #309 (spec 301): network detail page ---
// Network detail (GET .../networks/{networkName}, backend DTO
// NetworkDetailResponse): default shape is the app-net bridge network with
// one attached container; tests override `containers` per scenario.
export function networkDetailFixture(
	overrides: Partial<NetworkDetail> = {},
): NetworkDetail {
	return {
		name: "app-net",
		id: "9f8e7d6c5b4a",
		driver: "bridge",
		createdAt: "2026-08-20T10:00:00Z",
		subnets: ["10.88.0.0/16"],
		containers: [{ id: "c-1", name: "my-container" }],
		...overrides,
	};
}
// --- end ticket #309 ---

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

// --- spec 352 ticket #355: pods tab ---------------------------------------
// Pod list entry (GET .../pods, backend PodListResponse): default shape is
// a running two-container pod on one network; tests override per scenario.
export function podFixture(overrides: Partial<PodSummary> = {}): PodSummary {
	return {
		id: "p-1",
		name: "app-pod",
		state: "Running",
		createdAt: "2026-08-20T10:00:00Z",
		containerCount: 2,
		networks: ["app-net"],
		labels: {},
		...overrides,
	};
}

// Pod detail (GET .../pods/{podId}, backend PodDetailResponse): the list
// fields plus restart policy, mounts, and the pod's containers; tests
// override per scenario.
export function podDetailFixture(
	overrides: Partial<PodDetail> = {},
): PodDetail {
	return {
		id: "p-1",
		name: "app-pod",
		state: "Running",
		createdAt: "2026-08-20T10:00:00Z",
		restartPolicy: "always",
		mounts: [
			"type=volume,source=/var/lib/containers/storage/volumes/app-data/_data,destination=/data",
		],
		containers: [
			{ id: "c-1", name: "web", state: "running" },
			{ id: "c-infra", name: "app-pod-infra", state: "running" },
		],
		networks: ["app-net"],
		labels: { app: "web" },
		...overrides,
	};
}
// --- end spec 352 ticket #355 ---------------------------------------------

// Secret copy/move preview (GET .../secrets/{id}/move-preview, issue 239):
// default shape is a clean target and one referencing quadlet; tests
// override `collision`/`referencingQuadlets` per scenario.
export function secretMovePreviewFixture(
	overrides: Partial<SecretMovePreview> = {},
): SecretMovePreview {
	return {
		collision: false,
		referencingQuadlets: [{ name: "web.container" }],
		...overrides,
	};
}

// Secret transfer envelope (POST .../secrets/{id}/move, issue 239): the
// plain JSON answer. Default shape is a completed move of db-password from
// m-1 to m-2.
export function secretMoveResultFixture(
	overrides: Partial<SecretMoveResult> = {},
): SecretMoveResult {
	return {
		name: "db-password",
		status: "moved",
		sourceHostId: "m-1",
		targetHostId: "m-2",
		...overrides,
	};
}

// Volume copy/move preview (GET .../volumes/{name}/move-preview, issue 238):
// default shape is a clean target, an idle source, and one referencing
// quadlet; tests override `collision`/`inUse`/`referencingQuadlets` per
// scenario.
export function volumeMovePreviewFixture(
	overrides: Partial<VolumeMovePreview> = {},
): VolumeMovePreview {
	return {
		collision: false,
		inUse: false,
		referencingQuadlets: [{ name: "web.container" }],
		...overrides,
	};
}

// Volume transfer envelope (POST .../volumes/{name}/move, issue 238): the
// terminal complete event's result. Default shape is a completed move of
// app-data from m-1 to m-2.
export function volumeMoveResultFixture(
	overrides: Partial<VolumeMoveResult> = {},
): VolumeMoveResult {
	return {
		name: "app-data",
		status: "moved",
		sourceHostId: "m-1",
		targetHostId: "m-2",
		bytes: 2097152,
		...overrides,
	};
}

// --- issue 376: image copy fixtures ---------------------------------------
// Image copy preview (GET .../images/{imageId}/copy-preview, issue 375):
// default shape is a target that does not hold the image; tests override
// `present` per scenario.
export function imageCopyPreviewFixture(
	overrides: Partial<ImageCopyPreview> = {},
): ImageCopyPreview {
	return {
		present: false,
		...overrides,
	};
}

// Image transfer envelope (POST .../images/{imageId}/copy, issue 375): the
// terminal complete event's result. Default shape is a completed copy of
// the fixture image from m-1 to m-2 with its tag as the loaded name.
export function imageCopyResultFixture(
	overrides: Partial<ImageCopyResult> = {},
): ImageCopyResult {
	return {
		id: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		status: "copied",
		sourceHostId: "m-1",
		targetHostId: "m-2",
		bytes: 2097152,
		names: ["docker.io/library/nginx:1.27"],
		...overrides,
	};
}
// Image copy stream events (the NDJSON transport, issue 375): the
// progress event, the terminal complete event, and the terminal error
// event — the backend's ImageCopyEvent vocabulary. The generic NDJSON
// test builder types pull-stream events, so these travel as raw
// framed-line payloads, not typed DTOs.
export type ImageCopyStreamEvent = Record<string, unknown> & {
	type: string;
	operationId: string;
};

export function imageCopyEventFixture(
	overrides: Record<string, unknown> = {},
): ImageCopyStreamEvent {
	return {
		operationId: "op-1",
		type: "image",
		hostId: "m-1",
		image:
			"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		phase: "start",
		...overrides,
	};
}

export function imageCopyCompleteEventFixture(
	result: ImageCopyResult,
): ImageCopyStreamEvent {
	return {
		operationId: "op-1",
		type: "complete",
		hostId: "m-1",
		result,
	};
}

export function imageCopyErrorEventFixture(
	code: string,
	message: string,
): ImageCopyStreamEvent {
	return {
		operationId: "op-1",
		type: "error",
		hostId: "m-1",
		error: { code, message },
	};
}
// --- end issue 376 ---------------------------------------------------------

export function userFixture(overrides: Partial<UserSummary> = {}): UserSummary {
	return {
		id: "u-1",
		username: "alice",
		...overrides,
	};
}

export function authStatusFixture(
	overrides: Partial<AuthStatusResponse> = {},
): AuthStatusResponse {
	return {
		authenticated: false,
		needsSetup: false,
		...overrides,
	};
}

export function hostFixture(overrides: Partial<Host> = {}): Host {
	// Default shape: the owner's own view — the backend seeds the owner an
	// admin access row and the list endpoint reports isOwner true. Grantee
	// fixtures override `permission` and `isOwner` together.
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
		isOwner: true,
		createdAt: "2026-08-01T10:00:00Z",
		updatedAt: "2026-08-01T10:00:00Z",
		...overrides,
	};
}

// Raw inspect report (GET .../{resource}/{id}/inspect, spec 287): the
// backend returns the podman report as-is, so the client type is an untyped
// JSON object and no DTO fixture shape exists. One shared factory for the
// raw payload, since all four inspect suites consume it; tests override the
// fields they assert against.
export function rawInspectFixture(
	overrides: Partial<Record<string, unknown>> = {},
): Record<string, unknown> {
	return {
		Id: "c-1",
		Name: "/my-container",
		State: { Status: "running", Running: true, Pid: 4242 },
		Config: {
			Image: "docker.io/library/nginx:1.27",
			Labels: { app: "web" },
		},
		...overrides,
	};
}

// HostVersion mirrors GET /api/hosts/{id}/version: both fields are
// null when the remote Podman could not be asked (offline is a valid
// answer, not an error).
export function hostVersionFixture(
	overrides: Partial<HostVersion> = {},
): HostVersion {
	return {
		podmanVersion: "5.4.1",
		apiVersion: "5.4.1",
		...overrides,
	};
}
