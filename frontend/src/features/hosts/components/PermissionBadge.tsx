import { Badge } from "@chakra-ui/react";
import type { Host } from "@shared/types/api";

export function PermissionBadge({
	permission,
}: {
	permission: Host["permission"];
}) {
	const colorScheme =
		permission === "admin"
			? "purple"
			: permission === "write"
				? "blue"
				: "gray";
	return (
		<Badge colorScheme={colorScheme} data-testid="host-permission">
			{permission}
		</Badge>
	);
}
