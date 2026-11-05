import type {
	AccessGrant,
	Host,
	HostPermission,
	HostResponse,
} from "@shared/types/api";
import { api, hostApiPath } from "@shared/utils/api";

// CreateHostRequest (backend/internal/domain/hosts/service.go):
// the SSH private key contents are read client-side via the File API and
// sent in the request body.
export interface CreateHostInput {
	name: string;
	description?: string;
	hostname: string;
	port: number;
	sshUsername: string;
	sshPrivateKey: string;
	podmanSocketPath?: string;
}

export async function listHosts(): Promise<Host[]> {
	const response = await api.get<Host[]>("/hosts");
	return response.data;
}

// UpdateHostRequest mirrors the backend PUT /hosts/{id} body: empty or
// omitted fields are left untouched, so the SSH key is only sent when a new
// file was uploaded.
export interface UpdateHostInput {
	name: string;
	description: string;
	hostname: string;
	port: number;
	sshUsername: string;
	sshPrivateKey?: string;
	podmanSocketPath: string;
}

export async function getHost(id: string): Promise<Host> {
	const response = await api.get<Host>(hostApiPath(id));
	return response.data;
}

export async function updateHost(
	id: string,
	input: UpdateHostInput,
): Promise<HostResponse> {
	const response = await api.put<HostResponse>(hostApiPath(id), input);
	return response.data;
}

export async function deleteHost(id: string): Promise<void> {
	await api.delete(hostApiPath(id));
}

export async function listHostAccess(hostId: string): Promise<AccessGrant[]> {
	const response = await api.get<AccessGrant[]>(hostApiPath(hostId, "access"));
	return response.data;
}

export async function grantHostAccess(
	hostId: string,
	userId: string,
	permission: HostPermission,
): Promise<AccessGrant> {
	const response = await api.post<AccessGrant>(hostApiPath(hostId, "access"), {
		userId,
		permission,
	});
	return response.data;
}

export async function revokeHostAccess(
	hostId: string,
	userId: string,
): Promise<void> {
	await api.delete(hostApiPath(hostId, "access", userId));
}

// HostStatus mirrors GET /api/hosts/{id}/status
// (backend/internal/domain/hosts/handler.go). checkedAt is RFC3339 UTC.
export interface HostStatus {
	status: "online" | "offline";
	checkedAt: string;
}

export async function getHostStatus(hostId: string): Promise<HostStatus> {
	const response = await api.get<HostStatus>(hostApiPath(hostId, "status"));
	return response.data;
}

// HostVersion mirrors GET /api/hosts/{id}/version
// (backend/internal/domain/hosts/handler.go). Both fields are null when
// the remote Podman could not be asked — offline is a valid answer, not an
// error.
export interface HostVersion {
	podmanVersion: string | null;
	apiVersion: string | null;
}

export async function getHostVersion(hostId: string): Promise<HostVersion> {
	const response = await api.get<HostVersion>(hostApiPath(hostId, "version"));
	return response.data;
}

export async function createHost(
	input: CreateHostInput,
): Promise<HostResponse> {
	const response = await api.post<HostResponse>("/hosts", input);
	return response.data;
}
