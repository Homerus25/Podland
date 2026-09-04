import { Button, Heading, HStack, VStack } from "@chakra-ui/react";
import {
	fetchContainerLogs,
	followContainerLogs,
} from "@features/containers/services/containersApi";
import {
	fetchQuadletLogs,
	followQuadletLogs,
} from "@features/quadlets/services/quadletsApi";
import { type LogSource, LogViewer } from "@shared/components/LogViewer";
import { getApiError } from "@shared/utils/api";
import { useLocation, useNavigate, useParams } from "react-router-dom";

type LogKind = "container" | "quadlet";

function isLogKind(value: string | undefined): value is LogKind {
	return value === "container" || value === "quadlet";
}

function buildSource(
	kind: LogKind,
	machineId: string,
	targetId: string,
): LogSource {
	if (kind === "container") {
		return {
			fetchTail: async (tail) => {
				try {
					return await fetchContainerLogs(machineId, targetId, tail);
				} catch (error) {
					throw getApiError(error);
				}
			},
			follow: (tail, onEvent, signal) =>
				followContainerLogs(machineId, targetId, tail, onEvent, signal),
		};
	}
	return {
		fetchTail: async (tail) => {
			try {
				return await fetchQuadletLogs(machineId, targetId, tail);
			} catch (error) {
				throw getApiError(error);
			}
		},
		follow: (tail, onEvent, signal) =>
			followQuadletLogs(machineId, targetId, tail, onEvent, signal),
	};
}

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
				source={buildSource(kind, machineId, targetId)}
				maxHeight="70vh"
			/>
		</VStack>
	);
}
