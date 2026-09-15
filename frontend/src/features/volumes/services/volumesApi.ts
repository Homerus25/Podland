import { api, machineApiPath, machineApiUrl } from "@shared/utils/api";

// Volume as returned by GET /api/machines/{id}/volumes
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

// Volume as returned by GET /api/machines/{id}/volumes/{name}
// (backend volumes dto.go VolumeInspectResponse): the volume's full
// configuration. size is only present when podman reports usage.
// labels/options are always present ({} when empty).
export type VolumeDetail = VolumeSummary;

export async function inspectVolume(
	machineId: string,
	volumeName: string,
): Promise<VolumeDetail> {
	const response = await api.get<VolumeDetail>(
		machineApiPath(machineId, "volumes", volumeName),
	);
	return response.data;
}

// Body of POST /api/machines/{id}/volumes (backend VolumeCreateRequest).
// name is optional: an empty name creates an anonymous volume whose
// generated name podman reports back. Name-format validation is podman's
// job; its rejections surface as 400 INVALID_REQUEST.
export interface VolumeCreateInput {
	name: string;
	labels?: Record<string, string>;
	driver?: string;
	options?: Record<string, string>;
}

// Response of POST /api/machines/{id}/volumes -> 201: the created volume's
// inspect response. For anonymous volumes this carries the podman-generated
// name and anonymous: true.
export type VolumeCreated = VolumeDetail;

export async function createVolume(
	machineId: string,
	input: VolumeCreateInput,
): Promise<VolumeCreated> {
	const response = await api.post<VolumeCreated>(
		machineApiPath(machineId, "volumes"),
		input,
	);
	return response.data;
}

export async function listVolumes(machineId: string): Promise<VolumeSummary[]> {
	const response = await api.get<VolumeSummary[]>(
		machineApiPath(machineId, "volumes"),
	);
	return response.data;
}

// DELETE /api/machines/{id}/volumes/{name}?force=... -> 204. The force flag
// (default false) lets podman stop containers using the volume before
// removal; without it an in-use volume surfaces as 409 VOLUME_IN_USE.
export async function deleteVolume(
	machineId: string,
	volumeName: string,
	force: boolean,
): Promise<void> {
	await api.delete(machineApiPath(machineId, "volumes", volumeName), {
		params: force ? { force: "true" } : undefined,
	});
}

// One entry of POST /api/machines/{id}/volumes/prune -> 200 (backend
// volumes dto.go VolumePruneResponse): a removed volume's name and
// reclaimed size, plus podman's per-volume error when removal failed.
export interface VolumePruneReport {
	name: string;
	size: number;
	error?: string;
}

// Prunes every unused volume on the machine. In-use volumes are never
// touched.
export async function pruneVolumes(
	machineId: string,
): Promise<VolumePruneReport[]> {
	const response = await api.post<VolumePruneReport[]>(
		machineApiPath(machineId, "volumes", "prune"),
	);
	return response.data;
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

// POST /api/machines/{id}/volumes/{name}/populate -> 204. The FormData must
// carry exactly one of the backend's two shapes: one or more "files" parts
// whose filenames are the relative paths inside the volume, or exactly one
// "archive" part holding a ready-made tar. Populate merges: uploaded paths
// overwrite same-named files, everything else is kept. Oversized uploads
// surface as 413 VOLUME_UPLOAD_TOO_LARGE.
export async function populateVolume(
	machineId: string,
	volumeName: string,
	form: FormData,
	onProgress?: (fraction: number) => void,
): Promise<void> {
	const url = new URL(
		machineApiUrl(machineId, "volumes", volumeName, "populate"),
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
export function volumeExportUrl(machineId: string, volumeName: string): string {
	return machineApiUrl(machineId, "volumes", volumeName, "export");
}
