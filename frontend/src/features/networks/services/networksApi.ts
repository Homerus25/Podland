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
