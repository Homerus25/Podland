import { api, machineApiPath } from "@shared/utils/api";

// Secret as returned by GET /api/machines/{id}/secrets
// (backend/internal/domain/secrets/dto.go SecretListResponse). The backend
// deliberately omits the value: it only ever travels in the admin-only
// reveal path (GET .../secrets/{id}?reveal=true), which this type does not
// model yet.
export interface SecretSummary {
	id: string;
	name: string;
	// Name of the driver that stores the secret (e.g. "file")
	driver: string;
	// ISO8601 creation/update timestamps ("" when the backend reports none)
	createdAt: string;
	updatedAt: string;
	labels?: Record<string, string>;
}

export async function listSecrets(machineId: string): Promise<SecretSummary[]> {
	const response = await api.get<SecretSummary[]>(
		machineApiPath(machineId, "secrets"),
	);
	return response.data;
}

// Response of GET /api/machines/{id}/secrets/{secretId}
// (backend SecretInspectResponse). secretData is present ONLY when the
// request carried ?reveal=true and the caller is admin; the backend omits
// the field entirely otherwise.
export interface SecretInspect extends SecretSummary {
	// The secret's value. Only ever populated by the admin reveal path.
	secretData?: string;
}

export async function getSecret(
	machineId: string,
	secretId: string,
	reveal = false,
): Promise<SecretInspect> {
	const response = await api.get<SecretInspect>(
		machineApiPath(machineId, "secrets", secretId),
		{ params: reveal ? { reveal: "true" } : undefined },
	);
	return response.data;
}

// Body of POST /api/machines/{id}/secrets
// (backend/internal/domain/secrets/dto.go SecretCreateRequest). The backend
// trims the name; the value must arrive byte-identical — meaningful
// whitespace is the whole point of the no-trim rule.
export interface CreateSecretInput {
	name: string;
	value: string;
}

// Response of POST /api/machines/{id}/secrets -> 201
// (backend SecretCreateResponse: entities.SecretCreateReport's ID).
export interface SecretCreated {
	id: string;
}

export async function createSecret(
	machineId: string,
	input: CreateSecretInput,
): Promise<SecretCreated> {
	const response = await api.post<SecretCreated>(
		machineApiPath(machineId, "secrets"),
		input,
	);
	return response.data;
}

// DELETE /api/machines/{id}/secrets/{secretId} -> 204. The list DTO's name
// is the natural row key, so the secret is addressed by name. The backend
// rejects deletion with 409 SECRET_IN_USE when a container references it.
export async function deleteSecret(
	machineId: string,
	secretId: string,
): Promise<void> {
	await api.delete(machineApiPath(machineId, "secrets", secretId));
}

// Mode of POST /api/machines/{id}/secrets/{secretId}/move
// (backend/internal/domain/secrets/dto.go SecretMoveMode*): "copy" reveals
// the value on the source (admin) and creates the secret on the target;
// "move" additionally removes the source secret once the target holds it.
export type SecretMoveMode = "copy" | "move";

// Body of the secret transfer endpoint
// (backend/internal/domain/secrets/dto.go SecretMoveRequest). The name is
// preserved on both machines; there is no rename and no overwrite flag —
// a same-named target secret refuses the transfer with 409.
export interface SecretMoveInput {
	targetMachineId: string;
	mode: SecretMoveMode;
}

// Status of a secret transfer
// (backend/internal/domain/secrets/dto.go SecretMoveStatus*): "copied" for
// a completed copy, "moved" for a completed move, "partial" when the target
// install succeeded but the source removal failed — the secret then exists
// on both machines.
export type SecretMoveStatus = "copied" | "moved" | "partial";

// Response of POST /api/machines/{id}/secrets/{secretId}/move
// (backend/internal/domain/secrets/dto.go SecretMoveResponse), a plain
// JSON envelope — secret values are small, the NDJSON transport stays a
// volume concern. error is the removal-failure detail, populated only when
// status is "partial"; the value never appears here.
export interface SecretMoveResult {
	name: string;
	status: SecretMoveStatus;
	sourceMachineId: string;
	targetMachineId: string;
	error?: string;
}

// One quadlet file on the source machine whose `Secret=` lines resolve to
// the secret (backend/internal/domain/secrets/dto.go
// SecretMoveQuadletRef) — the preview's warning, never a block.
export interface SecretMoveQuadletRef {
	name: string;
}

// Response of GET /api/machines/{id}/secrets/{secretId}/move-preview
// (backend/internal/domain/secrets/dto.go SecretMovePreviewResponse): the
// dialog's facts. collision: the target already holds a same-named secret
// (the transfer refuses, never overwrites). referencingQuadlets: the
// source machine's quadlet files referencing the secret — a warning in
// move mode, moving breaks them.
export interface SecretMovePreview {
	collision: boolean;
	referencingQuadlets: SecretMoveQuadletRef[];
}

export async function getSecretMovePreview(
	machineId: string,
	secretId: string,
	targetMachineId: string,
): Promise<SecretMovePreview> {
	const response = await api.get<SecretMovePreview>(
		machineApiPath(machineId, "secrets", secretId, "move-preview"),
		{ params: { targetMachineId } },
	);
	return response.data;
}

// POST /api/machines/{id}/secrets/{secretId}/move -> 200 with the plain
// JSON transfer envelope. The secret is addressed by name, the row's key.
// A same-named target secret surfaces as 409 SECRET_ALREADY_EXISTS.
export async function moveSecret(
	machineId: string,
	secretId: string,
	input: SecretMoveInput,
): Promise<SecretMoveResult> {
	const response = await api.post<SecretMoveResult>(
		machineApiPath(machineId, "secrets", secretId, "move"),
		input,
	);
	return response.data;
}
