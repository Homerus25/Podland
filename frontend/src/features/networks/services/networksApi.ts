import { api, hostApiPath } from "@shared/utils/api";

// Network as returned by GET /api/hosts/{id}/networks
// (backend/internal/domain/networks/dto.go NetworkResponse). The DTO
// carries exactly the rendered fields — no id, labels, options, or flags.
export interface NetworkSummary {
	name: string;
	// Network driver (bridge, macvlan, host, …)
	driver: string;
	// Subnets in CIDR form; always present, [] when the network has none
	subnets: string[];
	// RFC3339 creation timestamp
	createdAt: string;
}

export async function listNetworks(hostId: string): Promise<NetworkSummary[]> {
	const response = await api.get<NetworkSummary[]>(
		hostApiPath(hostId, "networks"),
	);
	return response.data;
}

// One attached container on the network detail DTO
// (backend/internal/domain/networks/dto.go NetworkContainerRef): the
// container's ID (the inspect report's Containers map key) and name.
export interface NetworkContainerRef {
	id: string;
	name: string;
}

// Network as returned by GET /api/hosts/{id}/networks/{networkName}
// (backend/internal/domain/networks/dto.go NetworkDetailResponse): the
// fields the network detail page renders, mapped from the inspect report.
export interface NetworkDetail {
	name: string;
	// Network ID from the inspect report
	id: string;
	// Network driver (bridge, macvlan, host, …)
	driver: string;
	// RFC3339 creation timestamp
	createdAt: string;
	// Subnets in CIDR form; always present, [] when the network has none
	subnets: string[];
	// Attached containers (id + name); always present, [] when none
	containers: NetworkContainerRef[];
}

// GET /api/hosts/{id}/networks/{networkName} -> 200 with the detail DTO
// (backend/internal/domain/networks handler Get).
export async function getNetwork(
	hostId: string,
	networkName: string,
): Promise<NetworkDetail> {
	const response = await api.get<NetworkDetail>(
		hostApiPath(hostId, "networks", networkName),
	);
	return response.data;
}

// GET /api/hosts/{id}/networks/{networkName}/inspect -> 200 with the
// raw podman inspect report as the top-level body. The report is deliberately
// untyped on the client: no DTO is invented for it.
export async function inspectNetwork(
	hostId: string,
	networkName: string,
): Promise<Record<string, unknown>> {
	const response = await api.get<Record<string, unknown>>(
		hostApiPath(hostId, "networks", networkName, "inspect"),
	);
	return response.data;
}

// Body of POST /api/hosts/{id}/networks
// (backend/internal/domain/networks/dto.go NetworkCreateRequest).
export interface CreateNetworkInput {
	name: string;
	// Container IDs to attach to the new network in the same call (optional)
	containers?: string[];
}

// One connect pair of useConnectContainers: both connect surfaces (the
// Networks tab's row dialog and the Containers tab's, issue 256) pass the
// target network and the container at mutate time; the multi-select dialog
// (issue 320) sends one pair per picked item.
export interface ConnectContainerVariables {
	networkName: string;
	containerId: string;
}

// POST /api/hosts/{id}/networks -> 201 with the created network in the
// list DTO shape (backend/internal/domain/networks handler Create).
export async function createNetwork(
	hostId: string,
	input: CreateNetworkInput,
): Promise<NetworkSummary> {
	const response = await api.post<NetworkSummary>(
		hostApiPath(hostId, "networks"),
		input,
	);
	return response.data;
}

// One entry of the prune response
// (backend/internal/domain/networks/dto.go NetworkPruneResponse): the
// removed network's name, or the per-network error when podman could not
// remove it. Podman's network prune report carries no size, so there is
// no size field.
export interface NetworkPruneReport {
	name: string;
	error?: string;
}

// POST /api/hosts/{id}/networks/prune -> 200 with the per-network
// report, one entry per removed network, [] when nothing was removed.
// Networks in use by a container are untouched.
export async function pruneNetworks(
	hostId: string,
): Promise<NetworkPruneReport[]> {
	const response = await api.post<NetworkPruneReport[]>(
		hostApiPath(hostId, "networks", "prune"),
	);
	return response.data;
}

// POST /api/hosts/{id}/networks/{networkName}/connect -> 200, empty body
// (backend/internal/domain/networks handler Connect).
export async function connectContainer(
	hostId: string,
	networkName: string,
	containerId: string,
): Promise<void> {
	await api.post(hostApiPath(hostId, "networks", networkName, "connect"), {
		container: containerId,
	});
}
