import { api } from '@shared/utils/api'
import { followLogStream, type LogEvent } from '@shared/streaming/logStream'

const apiBaseUrl = import.meta.env.VITE_API_BASE_URL || '/api'

// Quadlet as returned by GET /api/machines/{id}/quadlets
// (backend/internal/domain/quadlets/dto.go QuadletListResponse; raw is omitted
// in list responses).
export interface QuadletSummary {
  id: string
  name: string
  path: string
  unit: string
  // Resource type (container, pod, image, volume, network)
  resource: string
  // State of the quadlet (unknown, stopped, running, error)
  state: string
  raw?: string
}

export async function listQuadlets(machineId: string): Promise<QuadletSummary[]> {
  const response = await api.get<QuadletSummary[]>(`/machines/${machineId}/quadlets`)
  return response.data
}

// The resource types the backend accepts
// (QuadletCreateRequest validation: oneof container pod image volume network).
// The list endpoint can additionally report "unknown" for unrecognized file
// paths, so QuadletSummary.resource stays a plain string.
export const QUADLET_RESOURCES = ['container', 'pod', 'image', 'volume', 'network'] as const

export type QuadletResource = (typeof QUADLET_RESOURCES)[number]

// Body of POST /api/machines/{id}/quadlets
// (backend/internal/domain/quadlets/dto.go QuadletCreateRequest).
export interface CreateQuadletInput {
  name: string
  resource: QuadletResource
  content: string
}

// Response of POST /api/machines/{id}/quadlets -> 201
// (backend/internal/domain/quadlets/dto.go QuadletCreateResponse).
export interface QuadletCreated {
  id: string
  name: string
  path?: string
  unit?: string
  resource: string
  state: string
  content: string
}

export async function createQuadlet(
  machineId: string,
  input: CreateQuadletInput,
): Promise<QuadletCreated> {
  const response = await api.post<QuadletCreated>(`/machines/${machineId}/quadlets`, input)
  return response.data
}

// Response of GET /api/machines/{id}/quadlets/{quadletId} and of a successful
// update (backend/internal/domain/quadlets/dto.go QuadletGetResponse).
export interface QuadletDetail {
  id: string
  name: string
  path: string
  unit: string
  resource: string
  state: string
  content: string
}

export async function getQuadlet(machineId: string, quadletId: string): Promise<QuadletDetail> {
  const response = await api.get<QuadletDetail>(quadletUrl(machineId, quadletId))
  return response.data
}

// Body of PUT /api/machines/{id}/quadlets/{quadletId}
// (backend/internal/domain/quadlets/dto.go QuadletUpdateRequest). Content
// only: the name comes from the URL path and the resource type is derived
// from the existing quadlet file path, so neither is sent.
export interface UpdateQuadletInput {
  content: string
}

export async function updateQuadlet(
  machineId: string,
  quadletId: string,
  input: UpdateQuadletInput,
): Promise<QuadletDetail> {
  const response = await api.put<QuadletDetail>(quadletUrl(machineId, quadletId), input)
  return response.data
}

function quadletUrl(machineId: string, quadletId: string): string {
  return `/machines/${machineId}/quadlets/${encodeURIComponent(quadletId)}`
}

// DELETE /api/machines/{id}/quadlets/{quadletId} -> 204
// (backend/internal/domain/quadlets/handler.go QuadletHandler.Delete).
// stop stops the unit before the quadlet file is removed and defaults to
// true in the backend; force enables aggressive teardown and defaults to
// false. Both are always sent so the request reflects the chosen options.
export interface DeleteQuadletOptions {
  stop: boolean
  force: boolean
}

export async function deleteQuadlet(
  machineId: string,
  quadletId: string,
  options: DeleteQuadletOptions,
): Promise<void> {
  await api.delete(quadletUrl(machineId, quadletId), {
    params: { stop: options.stop, force: options.force },
  })
}

// POST /api/machines/{id}/quadlets/{quadletId}/start -> 200
// (backend/internal/domain/quadlets/handler.go QuadletHandler.Start).
// Start takes no options per the backend contract.
export async function startQuadlet(machineId: string, quadletId: string): Promise<void> {
  await api.post(`${quadletUrl(machineId, quadletId)}/start`)
}

// Query parameters for the stop and restart endpoints
// (backend/internal/domain/quadlets/handler.go parseLifecycleOptions). force
// defaults to false; timeout is seconds and unset by default, in which case
// it is omitted from the request entirely so systemd's own default applies.
export interface QuadletLifecycleOptions {
  force: boolean
  timeout?: number
}

// POST /api/machines/{id}/quadlets/{quadletId}/stop -> 200
// (backend/internal/domain/quadlets/handler.go QuadletHandler.Stop).
export async function stopQuadlet(
  machineId: string,
  quadletId: string,
  options: QuadletLifecycleOptions,
): Promise<void> {
  await api.post(`${quadletUrl(machineId, quadletId)}/stop`, undefined, {
    params: lifecycleParams(options),
  })
}

// POST /api/machines/{id}/quadlets/{quadletId}/restart -> 200
// (backend/internal/domain/quadlets/handler.go QuadletHandler.Restart);
// same option set as stop.
export async function restartQuadlet(
  machineId: string,
  quadletId: string,
  options: QuadletLifecycleOptions,
): Promise<void> {
  await api.post(`${quadletUrl(machineId, quadletId)}/restart`, undefined, {
    params: lifecycleParams(options),
  })
}

function lifecycleParams(options: QuadletLifecycleOptions) {
  return options.timeout === undefined
    ? { force: options.force }
    : { force: options.force, timeout: options.timeout }
}

// GET /api/machines/{id}/quadlets/{quadletId}/logs?tail=N (no follow)
// returns the journal's last lines as one multiline string
// (backend/internal/domain/quadlets/dto.go QuadletLogsResponse).
export async function fetchQuadletLogs(
  machineId: string,
  quadletId: string,
  tail: number,
): Promise<string[]> {
  const response = await api.get<{ id: string; logs: string }>(
    `${quadletUrl(machineId, quadletId)}/logs`,
    { params: { tail } },
  )
  return response.data.logs.split('\n').filter((line) => line !== '')
}

// Same endpoint with follow=true: NDJSON stream of QuadletLogEvent values.
// Uses plain fetch via the shared stream helper (axios buffers responses).
export async function followQuadletLogs(
  machineId: string,
  quadletId: string,
  tail: number,
  onEvent: (event: LogEvent) => void,
  signal: AbortSignal,
): Promise<void> {
  await followLogStream(
    `${apiBaseUrl}${quadletUrl(machineId, quadletId)}/logs?tail=${tail}&follow=true`,
    onEvent,
    signal,
  )
}
