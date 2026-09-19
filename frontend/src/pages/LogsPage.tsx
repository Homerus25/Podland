import { Button, Heading, HStack, VStack } from "@chakra-ui/react";
import { containerLogs } from "@features/containers/services/containersApi";
import { quadletLogs } from "@features/quadlets/services/quadletsApi";
import { LogViewer } from "@shared/components/LogViewer";
import type { LogSource } from "@shared/streaming/logSource";
import { useLocation, useNavigate, useParams } from "react-router-dom";

type LogKind = "container" | "quadlet";

function isLogKind(value: string | undefined): value is LogKind {
	return value === "container" || value === "quadlet";
}

// Exhaustive over LogKind: a future log-bearing kind is a type error until it
// gets an entry. The kind guard still narrows before the lookup.
const LOG_SOURCES: Record<
	LogKind,
	(machineId: string, targetId: string) => LogSource
> = {
	container: containerLogs,
	quadlet: quadletLogs,
};

// Full-page log viewer for a container or quadlet. The list buttons navigate
// here instead of opening a small modal, so logs get the whole viewport.
// A friendly display name travels via navigation state when available.
export function LogsPage() {
	const { machineId, kind, targetId } = useParams<{
		machineId: string;
		kind: string;
		targetId: string;
	}>();
	const navigate = useNavigate();
	const location = useLocation();
	const name =
		(location.state as { name?: string } | null)?.name ?? targetId ?? "";

	if (!machineId || !targetId || !isLogKind(kind)) {
		return null;
	}

	return (
		<VStack align="stretch" spacing={4}>
			<HStack justify="space-between">
				<Heading size="lg">Logs — {name}</Heading>
				<Button
					variant="outline"
					onClick={() => navigate(-1)}
					data-testid="logs-back-button"
				>
					Back
				</Button>
			</HStack>
			<LogViewer
				source={LOG_SOURCES[kind](machineId, targetId)}
				maxHeight="70vh"
			/>
		</VStack>
	);
}
