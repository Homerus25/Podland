import { Badge, Tooltip } from "@chakra-ui/react";

// Status vocabulary mirrors the backend probe (GET /api/hosts/{id}/status):
// online/offline come from the endpoint, checking/unknown are client-side
// states for an in-flight request and a failed request respectively.
const STATUS_COLOR_SCHEMES: Record<string, string> = {
	online: "green",
	offline: "red",
	checking: "gray",
	unknown: "gray",
};

export function hostStatusColorScheme(status: string): string {
	return STATUS_COLOR_SCHEMES[status.toLowerCase()] ?? "gray";
}

export interface HostStatusBadgeProps {
	status: "online" | "offline" | "checking" | "unknown";
	checkedAt?: string;
}

export function HostStatusBadge({ status, checkedAt }: HostStatusBadgeProps) {
	const badge = (
		<Badge
			colorScheme={hostStatusColorScheme(status)}
			data-testid="host-status-badge"
			aria-label={checkedAt ? `Checked at ${checkedAt}` : undefined}
			tabIndex={0}
		>
			<span
				data-testid="host-status-badge-dot"
				style={{
					display: "inline-block",
					width: 8,
					height: 8,
					borderRadius: "50%",
					marginRight: 4,
					backgroundColor: "currentColor",
				}}
			/>
			<span data-testid="host-status-badge-label">{status}</span>
		</Badge>
	);

	if (!checkedAt) return badge;
	return <Tooltip label={`Checked at ${checkedAt}`}>{badge}</Tooltip>;
}
