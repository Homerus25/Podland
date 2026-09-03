import { api } from "@shared/utils/api";

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
		`/machines/${encodeURIComponent(machineId)}/volumes/${encodeURIComponent(volumeName)}`,
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
		`/machines/${encodeURIComponent(machineId)}/volumes`,
		input,
	);
	return response.data;
}

export async function listVolumes(machineId: string): Promise<VolumeSummary[]> {
	const response = await api.get<VolumeSummary[]>(
		`/machines/${encodeURIComponent(machineId)}/volumes`,
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
	await api.delete(
		`/machines/${encodeURIComponent(machineId)}/volumes/${encodeURIComponent(volumeName)}`,
		{ params: force ? { force: "true" } : undefined },
	);
}
