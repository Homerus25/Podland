import type { BulkRemoveEntry, BulkRemoveInput } from "@shared/types/api";
import { api, bulkRemove, hostApiPath, hostApiUrl } from "@shared/utils/api";

// Image as returned by GET /api/hosts/{id}/images
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

export async function listImages(hostId: string): Promise<ImageSummary[]> {
	const response = await api.get<ImageSummary[]>(hostApiPath(hostId, "images"));
	return response.data;
}

// Image as returned by GET /api/hosts/{id}/images/{imageId}
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
	hostId: string,
	imageId: string,
): Promise<ImageDetail> {
	const response = await api.get<ImageDetail>(
		hostApiPath(hostId, "images", imageId),
	);
	return response.data;
}

// One entry of POST /api/hosts/{id}/images/prune -> 200 (backend
// images dto.go ImagePruneResponse): a removed image's ID and reclaimed
// size, plus podman's per-image error when removal failed.
export interface ImagePruneReport {
	id: string;
	size: number;
	error?: string;
}

// Prunes every unused image on the host. Images in use by containers
// are never touched.
export async function pruneImages(hostId: string): Promise<ImagePruneReport[]> {
	const response = await api.post<ImagePruneReport[]>(
		hostApiPath(hostId, "images", "prune"),
	);
	return response.data;
}

export async function deleteImage(
	hostId: string,
	imageId: string,
	force = false,
): Promise<void> {
	await api.delete(hostApiPath(hostId, "images", imageId), {
		params: force ? { force: true } : undefined,
	});
}

// GET /api/hosts/{id}/images/{imageId}/inspect -> 200 with the raw
// podman inspect report as the top-level body. The report is deliberately
// untyped on the client: no DTO is invented for it.
export async function inspectImage(
	hostId: string,
	imageId: string,
): Promise<Record<string, unknown>> {
	const response = await api.get<Record<string, unknown>>(
		hostApiPath(hostId, "images", imageId, "inspect"),
	);
	return response.data;
}

export function pullStreamUrl(hostId: string): string {
	return hostApiUrl(hostId, "images", "pull");
}

// Response of GET /api/hosts/{id}/images/{imageId}/copy-preview
// (backend images dto.go ImageCopyPreviewResponse, issue 375): the copy
// dialog's one fact. present: the target host already holds the image
// by ID. Information, never a block — the load is idempotent
// (content-addressed), so confirming stays possible either way.
export interface ImageCopyPreview {
	present: boolean;
}

export async function getImageCopyPreview(
	hostId: string,
	imageId: string,
	targetHostId: string,
): Promise<ImageCopyPreview> {
	const response = await api.get<ImageCopyPreview>(
		hostApiPath(hostId, "images", imageId, "copy-preview"),
		{ params: { targetHostId } },
	);
	return response.data;
}

// Body of the image copy endpoint (backend images dto.go
// ImageCopyRequest, issue 375). Copy-only: no mode, no force — the source
// image is never touched.
export interface ImageCopyInput {
	targetHostId: string;
}

// Response of POST /api/hosts/{id}/images/{imageId}/copy (backend
// images dto.go ImageCopyResponse, issue 375), carried by the NDJSON
// stream's terminal complete event. status is always "copied" — there is
// no move mode. bytes is the archive that crossed the export→load splice;
// names are the image names the target's load reported (the tags the load
// applied), [] when it reported none.
export interface ImageCopyResult {
	id: string;
	status: "copied";
	sourceHostId: string;
	targetHostId: string;
	bytes: number;
	names: string[];
}

// Full stream URL for POST .../images/{imageId}/copy: the endpoint always
// answers NDJSON progress events with one terminal event. The transfer
// body travels in the request, so no query is appended.
export function imageCopyStreamUrl(hostId: string, imageId: string): string {
	return hostApiUrl(hostId, "images", imageId, "copy");
}

// Full URL for GET .../images/{imageId}/export (issue 401): the image's
// tar archive download, consumed by the fetch-based shared download
// helper. No filename is passed along — the server names the archive in
// its Content-Disposition (the short image ID with a .tar suffix), and
// the helper saves under the server-provided name.
export function imageExportUrl(hostId: string, imageId: string): string {
	return hostApiUrl(hostId, "images", imageId, "export");
}

// POST /api/hosts/{id}/images/remove -> 200 (backend
// internal/bulkremove Entry): one report entry per requested id, in request
// order. A per-id failure is an entry, never a status code, so an in-use
// image without force answers an IMAGE_USED_BY_CONTAINERS entry while the
// rest of the batch still processes. The POST is the shared bulkRemove helper's
// (shared/utils/api.ts): the first pass is plain on the wire, and only the
// report's retry carries force.
export function bulkRemoveImages(
	hostId: string,
	input: BulkRemoveInput,
): Promise<BulkRemoveEntry[]> {
	return bulkRemove(hostId, "images", input);
}
