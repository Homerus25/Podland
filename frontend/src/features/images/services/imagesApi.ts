import { api, machineApiPath, machineApiUrl } from "@shared/utils/api";

// Image as returned by GET /api/machines/{id}/images
// (backend/internal/domain/images/dto.go ImageListResponse).
export interface ImageSummary {
	id: string;
	// Podman reports null tags for dangling images.
	repoTags: string[] | null;
	repoDigests: string[];
	parentID?: string;
	createdAt: string;
	size: number;
	os?: string;
	architecture?: string;
	labels?: Record<string, string>;
}

export async function listImages(machineId: string): Promise<ImageSummary[]> {
	const response = await api.get<ImageSummary[]>(
		machineApiPath(machineId, "images"),
	);
	return response.data;
}

// One entry of POST /api/machines/{id}/images/prune -> 200 (backend
// images dto.go ImagePruneResponse): a removed image's ID and reclaimed
// size, plus podman's per-image error when removal failed.
export interface ImagePruneReport {
	id: string;
	size: number;
	error?: string;
}

// Prunes every unused image on the machine. Images in use by containers
// are never touched.
export async function pruneImages(
	machineId: string,
): Promise<ImagePruneReport[]> {
	const response = await api.post<ImagePruneReport[]>(
		machineApiPath(machineId, "images", "prune"),
	);
	return response.data;
}

export async function deleteImage(
	machineId: string,
	imageId: string,
	force = false,
): Promise<void> {
	await api.delete(machineApiPath(machineId, "images", imageId), {
		params: force ? { force: true } : undefined,
	});
}

export function pullStreamUrl(machineId: string): string {
	return machineApiUrl(machineId, "images", "pull");
}
