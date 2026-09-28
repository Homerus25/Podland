import { createLogSource, type LogSource } from "@shared/streaming/logSource";
import { api, machineApiPath, machineApiUrl } from "@shared/utils/api";

// Quadlet as returned by GET /api/machines/{id}/quadlets
// (backend/internal/domain/quadlets/dto.go QuadletListResponse; raw is omitted
// in list responses).
export interface QuadletSummary {
	id: string;
	name: string;
	path: string;
	unit: string;
	// Resource type (container, pod, image, volume, network)
	resource: string;
	// State of the quadlet (unknown, stopped, running, error)
	state: string;
	raw?: string;
}

export async function listQuadlets(
	machineId: string,
): Promise<QuadletSummary[]> {
	const response = await api.get<QuadletSummary[]>(
		machineApiPath(machineId, "quadlets"),
	);
	return response.data;
}

// The resource types the backend accepts
// (QuadletCreateRequest validation: oneof container pod image volume network).
// The list endpoint can additionally report "unknown" for unrecognized file
// paths, so QuadletSummary.resource stays a plain string.
export const QUADLET_RESOURCES = [
	"container",
	"pod",
	"image",
	"volume",
	"network",
] as const;

export type QuadletResource = (typeof QUADLET_RESOURCES)[number];

// Body of POST /api/machines/{id}/quadlets
// (backend/internal/domain/quadlets/dto.go QuadletCreateRequest).
export interface CreateQuadletInput {
	name: string;
	resource: QuadletResource;
	content: string;
}

// Response of POST /api/machines/{id}/quadlets -> 201
// (backend/internal/domain/quadlets/dto.go QuadletCreateResponse).
export interface QuadletCreated {
	id: string;
	name: string;
	path?: string;
	unit?: string;
	resource: string;
	state: string;
	content: string;
}

export async function createQuadlet(
	machineId: string,
	input: CreateQuadletInput,
): Promise<QuadletCreated> {
	const response = await api.post<QuadletCreated>(
		machineApiPath(machineId, "quadlets"),
		input,
	);
	return response.data;
}

// Response of GET /api/machines/{id}/quadlets/{quadletId} and of a successful
// update (backend/internal/domain/quadlets/dto.go QuadletGetResponse).
export interface QuadletDetail {
	id: string;
	name: string;
	path: string;
	unit: string;
	resource: string;
	state: string;
	content: string;
}

export async function getQuadlet(
	machineId: string,
	quadletId: string,
): Promise<QuadletDetail> {
	const response = await api.get<QuadletDetail>(
		machineApiPath(machineId, "quadlets", quadletId),
	);
	return response.data;
}

// Body of PUT /api/machines/{id}/quadlets/{quadletId}
// (backend/internal/domain/quadlets/dto.go QuadletUpdateRequest). Content
// only: the name comes from the URL path and the resource type is derived
// from the existing quadlet file path, so neither is sent.
export interface UpdateQuadletInput {
	content: string;
}

export async function updateQuadlet(
	machineId: string,
	quadletId: string,
	input: UpdateQuadletInput,
): Promise<QuadletDetail> {
	const response = await api.put<QuadletDetail>(
		machineApiPath(machineId, "quadlets", quadletId),
		input,
	);
	return response.data;
}

// DELETE /api/machines/{id}/quadlets/{quadletId} -> 204
// (backend/internal/domain/quadlets/handler.go QuadletHandler.Delete).
// stop stops the unit before the quadlet file is removed and defaults to
// true in the backend; force enables aggressive teardown and defaults to
// false. Both are always sent so the request reflects the chosen options.
export interface DeleteQuadletOptions {
	stop: boolean;
	force: boolean;
}

export async function deleteQuadlet(
	machineId: string,
	quadletId: string,
	options: DeleteQuadletOptions,
): Promise<void> {
	await api.delete(machineApiPath(machineId, "quadlets", quadletId), {
		params: { stop: options.stop, force: options.force },
	});
}

// POST /api/machines/{id}/quadlets/{quadletId}/start -> 200
// (backend/internal/domain/quadlets/handler.go QuadletHandler.Start).
// Start takes no options per the backend contract.
export async function startQuadlet(
	machineId: string,
	quadletId: string,
): Promise<void> {
	await api.post(machineApiPath(machineId, "quadlets", quadletId, "start"));
}

// Query parameters for the stop and restart endpoints
// (backend/internal/domain/quadlets/handler.go parseLifecycleOptions). There
// is no force parameter (systemd ignores --force for stop/restart, issue
// 205). timeout is seconds and unset by default, in which case it is omitted
// from the request entirely so systemd's own default applies.
export interface QuadletLifecycleOptions {
	timeout?: number;
}

// POST /api/machines/{id}/quadlets/{quadletId}/stop -> 200
// (backend/internal/domain/quadlets/handler.go QuadletHandler.Stop).
export async function stopQuadlet(
	machineId: string,
	quadletId: string,
	options: QuadletLifecycleOptions,
): Promise<void> {
	return postLifecycle(machineId, quadletId, "stop", options);
}

// POST /api/machines/{id}/quadlets/{quadletId}/restart -> 200
// (backend/internal/domain/quadlets/handler.go QuadletHandler.Restart);
// same option set as stop.
export async function restartQuadlet(
	machineId: string,
	quadletId: string,
	options: QuadletLifecycleOptions,
): Promise<void> {
	return postLifecycle(machineId, quadletId, "restart", options);
}

// Shared stop/restart body: the endpoints differ only in the path segment,
// and both take the same query parameters via lifecycleParams.
async function postLifecycle(
	machineId: string,
	quadletId: string,
	action: string,
	options: QuadletLifecycleOptions,
): Promise<void> {
	await api.post(
		machineApiPath(machineId, "quadlets", quadletId, action),
		undefined,
		{
			params: lifecycleParams(options),
		},
	);
}

function lifecycleParams(options: QuadletLifecycleOptions) {
	return options.timeout === undefined ? {} : { timeout: options.timeout };
}

// Log source for GET /api/machines/{machineId}/quadlets/{quadletId}/logs:
// the kit owns the buffered tail fetch and the follow stream
// (backend/internal/domain/quadlets handler pair).
export function quadletLogs(machineId: string, quadletId: string): LogSource {
	return createLogSource(
		machineApiPath(machineId, "quadlets", quadletId, "logs"),
		machineApiUrl(machineId, "quadlets", quadletId, "logs"),
	);
}

// Full stream URL for POST .../quadlets/auto-update; the ?dryRun=true query
// is the builder's, not the caller's (no query on a real run).
export function autoUpdateStreamUrl(
	machineId: string,
	options: { dryRun: boolean },
): string {
	const query = options.dryRun ? "?dryRun=true" : "";
	return `${machineApiUrl(machineId, "quadlets", "auto-update")}${query}`;
}
