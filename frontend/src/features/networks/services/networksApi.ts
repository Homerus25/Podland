import { api, machineApiPath } from "@shared/utils/api";

// Network as returned by GET /api/machines/{id}/networks
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

export async function listNetworks(
	machineId: string,
): Promise<NetworkSummary[]> {
	const response = await api.get<NetworkSummary[]>(
		machineApiPath(machineId, "networks"),
	);
	return response.data;
}

// GET /api/machines/{id}/networks/{networkName}/inspect -> 200 with the
// raw podman inspect report as the top-level body. The report is deliberately
// untyped on the client: no DTO is invented for it.
export async function inspectNetwork(
	machineId: string,
	networkName: string,
): Promise<Record<string, unknown>> {
	const response = await api.get<Record<string, unknown>>(
		machineApiPath(machineId, "networks", networkName, "inspect"),
	);
	return response.data;
}

// Body of POST /api/machines/{id}/networks
// (backend/internal/domain/networks/dto.go NetworkCreateRequest).
export interface CreateNetworkInput {
	name: string;
	// Container IDs to attach to the new network in the same call (optional)
	containers?: string[];
}

// Variables of useConnectContainer: both connect surfaces (the Networks
// tab's row dialog and the Containers tab's, issue 256) pass the target
// network and the container at mutate time.
export interface ConnectContainerVariables {
	networkName: string;
	containerId: string;
}

// POST /api/machines/{id}/networks -> 201 with the created network in the
// list DTO shape (backend/internal/domain/networks handler Create).
export async function createNetwork(
	machineId: string,
	input: CreateNetworkInput,
): Promise<NetworkSummary> {
	const response = await api.post<NetworkSummary>(
		machineApiPath(machineId, "networks"),
		input,
	);
	return response.data;
}

// POST /api/machines/{id}/networks/{networkName}/connect -> 200, empty body
// (backend/internal/domain/networks handler Connect).
export async function connectContainer(
	machineId: string,
	networkName: string,
	containerId: string,
): Promise<void> {
	await api.post(
		machineApiPath(machineId, "networks", networkName, "connect"),
		{
			container: containerId,
		},
	);
}
