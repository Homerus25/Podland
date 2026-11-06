import type { BulkRemoveEntry, BulkRemoveInput } from "@shared/types/api";
import { api, bulkRemove, hostApiPath, hostApiUrl } from "@shared/utils/api";

// Volume as returned by GET /api/hosts/{id}/volumes
// (backend/internal/domain/volumes volume DTO). size is only present when
// podman reports usage for the volume. labels/options are always present
// ({} when empty).
export interface VolumeSummary {
	name: string;
	driver: string;
	createdAt: string;
	anonymous: boolean;
	labels: Record<string, string>;
	options: Record<string, string>;
	mountpoint: string;
	size?: number;
}

// Volume as returned by GET /api/hosts/{id}/volumes/{name}
// (backend volumes dto.go VolumeInspectResponse): the volume's full
// configuration. size is only present when podman reports usage.
// labels/options are always present ({} when empty).
export type VolumeDetail = VolumeSummary;

export async function inspectVolume(
	hostId: string,
	volumeName: string,
): Promise<VolumeDetail> {
	const response = await api.get<VolumeDetail>(
		hostApiPath(hostId, "volumes", volumeName),
	);
	return response.data;
}

// Body of POST /api/hosts/{id}/volumes (backend VolumeCreateRequest).
// name is optional: an empty name creates an anonymous volume whose
// generated name podman reports back. Name-format validation is podman's
// job; its rejections surface as 400 INVALID_REQUEST.
export interface VolumeCreateInput {
	name: string;
	labels?: Record<string, string>;
	driver?: string;
	options?: Record<string, string>;
}

// Response of POST /api/hosts/{id}/volumes -> 201: the created volume's
// inspect response. For anonymous volumes this carries the podman-generated
// name and anonymous: true.
export type VolumeCreated = VolumeDetail;

export async function createVolume(
	hostId: string,
	input: VolumeCreateInput,
): Promise<VolumeCreated> {
	const response = await api.post<VolumeCreated>(
		hostApiPath(hostId, "volumes"),
		input,
	);
	return response.data;
}

export async function listVolumes(hostId: string): Promise<VolumeSummary[]> {
	const response = await api.get<VolumeSummary[]>(
		hostApiPath(hostId, "volumes"),
	);
	return response.data;
}

// DELETE /api/hosts/{id}/volumes/{name}?force=... -> 204. The force flag
// (default false) lets podman stop containers using the volume before
// removal; without it an in-use volume surfaces as 409 VOLUME_IN_USE.
export async function deleteVolume(
	hostId: string,
	volumeName: string,
	force: boolean,
): Promise<void> {
	await api.delete(hostApiPath(hostId, "volumes", volumeName), {
		params: force ? { force: "true" } : undefined,
	});
}

// One entry of POST /api/hosts/{id}/volumes/prune -> 200 (backend
// volumes dto.go VolumePruneResponse): a removed volume's name and
// reclaimed size, plus podman's per-volume error when removal failed.
export interface VolumePruneReport {
	name: string;
	size: number;
	error?: string;
}

// Prunes every unused volume on the host. In-use volumes are never
// touched.
export async function pruneVolumes(
	hostId: string,
): Promise<VolumePruneReport[]> {
	const response = await api.post<VolumePruneReport[]>(
		hostApiPath(hostId, "volumes", "prune"),
	);
	return response.data;
}

// POST /api/hosts/{id}/volumes/remove -> 200 (backend
// internal/bulkremove Entry): one report entry per requested name, in
// request order. A per-name failure is an entry, never a status code, so
// an in-use volume without force answers a VOLUME_IN_USE entry while the
// rest of the batch still processes. The POST is the shared bulkRemove helper's
// (shared/utils/api.ts): the first pass is plain on the wire, and only the
// report's retry carries force.
export function bulkRemoveVolumes(
	hostId: string,
	input: BulkRemoveInput,
): Promise<BulkRemoveEntry[]> {
	return bulkRemove(hostId, "volumes", input);
}

// Builds the populate request body in exactly one of the backend's two
// shapes: the given plain files as "files" parts (filenames are the
// relative paths inside the volume), or the single tar as the "archive"
// part. Passing both is a caller bug — the modal prevents it client-side.
export function buildPopulateForm(files: File[], archive?: File): FormData {
	const form = new FormData();
	// Browsers only populate webkitRelativePath for directory uploads;
	// plain multi-file picks fall back to the bare name.
	const relativePath = (file: File) => file.webkitRelativePath || file.name;
	if (archive) {
		form.append("archive", archive, archive.name);
		return form;
	}
	for (const file of files) {
		form.append("files", file, relativePath(file));
	}
	return form;
}

// POST /api/hosts/{id}/volumes/{name}/populate -> 204. The FormData must
// carry exactly one of the backend's two shapes: one or more "files" parts
// whose filenames are the relative paths inside the volume, or exactly one
// "archive" part holding a ready-made tar. Populate merges: uploaded paths
// overwrite same-named files, everything else is kept. Oversized uploads
// surface as 413 VOLUME_UPLOAD_TOO_LARGE.
export async function populateVolume(
	hostId: string,
	volumeName: string,
	form: FormData,
	onProgress?: (fraction: number) => void,
): Promise<void> {
	const url = new URL(
		hostApiUrl(hostId, "volumes", volumeName, "populate"),
		window.location.origin,
	).toString();
	await api.post(url, form, {
		adapter: "fetch",
		onUploadProgress: onProgress
			? (event) => {
					if (event.total) onProgress(event.loaded / event.total);
				}
			: undefined,
	});
}

// Full download URL for GET .../volumes/{name}/export, for the fetch-based
// downloadFile (which carries no axios baseURL of its own).
export function volumeExportUrl(hostId: string, volumeName: string): string {
	return hostApiUrl(hostId, "volumes", volumeName, "export");
}

// Mode of POST /api/hosts/{id}/volumes/{name}/move
// (backend/internal/domain/volumes/dto.go VolumeMoveMode*): "copy" creates
// the target volume and streams the data into it; "move" additionally
// removes the source volume once the target holds the data.
export type VolumeMoveMode = "copy" | "move";

// Body of the volume transfer endpoint
// (backend/internal/domain/volumes/dto.go VolumeMoveRequest). force is
// move-mode vocabulary (the delete flow's flag): a move of an in-use source
// refuses with 409 VOLUME_IN_USE without it; copy never removes anything
// and omits it entirely.
export interface VolumeMoveInput {
	targetHostId: string;
	mode: VolumeMoveMode;
	force?: boolean;
}

// Status of a volume transfer
// (backend/internal/domain/volumes/dto.go VolumeMoveStatus*): "copied" for
// a completed copy, "moved" for a completed move, "partial" when the data
// crossed but the source removal failed — the volume then exists on both
// hosts.
export type VolumeMoveStatus = "copied" | "moved" | "partial";

// Response of POST /api/hosts/{id}/volumes/{name}/move
// (backend/internal/domain/volumes/dto.go VolumeMoveResponse), carried by
// the NDJSON stream's terminal complete event. bytes is the data that
// crossed; error is the removal-failure detail, populated only when
// status is "partial".
export interface VolumeMoveResult {
	name: string;
	status: VolumeMoveStatus;
	sourceHostId: string;
	targetHostId: string;
	bytes: number;
	error?: string;
}

// One quadlet file on the source host whose `Volume=` lines resolve to
// the volume (backend/internal/domain/volumes/dto.go
// VolumeMoveQuadletRef) — the preview's warning, never a block.
export interface VolumeMoveQuadletRef {
	name: string;
}

// Response of GET /api/hosts/{id}/volumes/{name}/move-preview
// (backend/internal/domain/volumes/dto.go VolumeMovePreviewResponse): the
// dialog's facts. collision: the target already holds a same-named volume
// (the transfer refuses, never overwrites). inUse: a container on the
// source host mounts the volume (a move refuses without force).
export interface VolumeMovePreview {
	collision: boolean;
	inUse: boolean;
	referencingQuadlets: VolumeMoveQuadletRef[];
}

export async function getVolumeMovePreview(
	hostId: string,
	volumeName: string,
	targetHostId: string,
): Promise<VolumeMovePreview> {
	const response = await api.get<VolumeMovePreview>(
		hostApiPath(hostId, "volumes", volumeName, "move-preview"),
		{ params: { targetHostId } },
	);
	return response.data;
}

// Full stream URL for POST .../volumes/{name}/move: the endpoint always
// answers NDJSON progress events with one terminal event. The transfer
// body travels in the request, so no query is appended.
export function volumeMoveStreamUrl(
	hostId: string,
	volumeName: string,
): string {
	return hostApiUrl(hostId, "volumes", volumeName, "move");
}
