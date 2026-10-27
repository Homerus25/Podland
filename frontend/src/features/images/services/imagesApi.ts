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

// Image as returned by GET /api/machines/{id}/images/{imageId}
// (backend/internal/domain/images/dto.go ImageDetailResponse): the list
// fields combined with the inspect report's config fields.
export interface ImageDetail {
	id: string;
	// Podman reports null tags for dangling images.
	repoTags: string[] | null;
	repoDigests: string[];
	createdAt: string;
	size: number;
	os?: string;
	architecture?: string;
	user?: string;
	env?: Record<string, string>;
	entrypoint?: string[];
	cmd?: string[];
	labels?: Record<string, string>;
}

// The image detail endpoint (spec 301): one image's list fields plus the
// inspect report's config fields.
export async function getImage(
	machineId: string,
	imageId: string,
): Promise<ImageDetail> {
	const response = await api.get<ImageDetail>(
		machineApiPath(machineId, "images", imageId),
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

// GET /api/machines/{id}/images/{imageId}/inspect -> 200 with the raw
// podman inspect report as the top-level body. The report is deliberately
// untyped on the client: no DTO is invented for it.
export async function inspectImage(
	machineId: string,
	imageId: string,
): Promise<Record<string, unknown>> {
	const response = await api.get<Record<string, unknown>>(
		machineApiPath(machineId, "images", imageId, "inspect"),
	);
	return response.data;
}

export function pullStreamUrl(machineId: string): string {
	return machineApiUrl(machineId, "images", "pull");
}

// Response of GET /api/machines/{id}/images/{imageId}/copy-preview
// (backend images dto.go ImageCopyPreviewResponse, issue 375): the copy
// dialog's one fact. present: the target machine already holds the image
// by ID. Information, never a block — the load is idempotent
// (content-addressed), so confirming stays possible either way.
export interface ImageCopyPreview {
	present: boolean;
}

export async function getImageCopyPreview(
	machineId: string,
	imageId: string,
	targetMachineId: string,
): Promise<ImageCopyPreview> {
	const response = await api.get<ImageCopyPreview>(
		machineApiPath(machineId, "images", imageId, "copy-preview"),
		{ params: { targetMachineId } },
	);
	return response.data;
}

// Body of the image copy endpoint (backend images dto.go
// ImageCopyRequest, issue 375). Copy-only: no mode, no force — the source
// image is never touched.
export interface ImageCopyInput {
	targetMachineId: string;
}

// Response of POST /api/machines/{id}/images/{imageId}/copy (backend
// images dto.go ImageCopyResponse, issue 375), carried by the NDJSON
// stream's terminal complete event. status is always "copied" — there is
// no move mode. bytes is the archive that crossed the export→load splice;
// names are the image names the target's load reported (the tags the load
// applied), [] when it reported none.
export interface ImageCopyResult {
	id: string;
	status: "copied";
	sourceMachineId: string;
	targetMachineId: string;
	bytes: number;
	names: string[];
}

// Full stream URL for POST .../images/{imageId}/copy: the endpoint always
// answers NDJSON progress events with one terminal event. The transfer
// body travels in the request, so no query is appended.
export function imageCopyStreamUrl(machineId: string, imageId: string): string {
	return machineApiUrl(machineId, "images", imageId, "copy");
}

// Full URL for GET .../images/{imageId}/export (issue 401): the image's
// tar archive download, consumed by the fetch-based shared download
// helper. No filename is passed along — the server names the archive in
// its Content-Disposition (the short image ID with a .tar suffix), and
// the helper saves under the server-provided name.
export function imageExportUrl(machineId: string, imageId: string): string {
	return machineApiUrl(machineId, "images", imageId, "export");
}
