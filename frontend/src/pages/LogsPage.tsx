import { Button, Heading, HStack, useToast, VStack } from "@chakra-ui/react";
import { containerLogs } from "@features/containers/services/containersApi";
import { podLogs } from "@features/pods/services/podsApi";
import { quadletLogs } from "@features/quadlets/services/quadletsApi";
import { LogViewer } from "@shared/components/LogViewer";
import {
	isLogKind,
	type LogKind,
	type LogsPageState,
} from "@shared/logs/logRoute";
import { type LogSource, toApiError } from "@shared/streaming/logSource";
import { downloadText, sanitizeFilename } from "@shared/utils/download";
import { useState } from "react";
import { useLocation, useNavigate, useParams } from "react-router-dom";

// Exhaustive over LogKind (owned by the route kit): a future log-bearing kind
// is a type error until it gets an entry. The kit's kind guard still narrows
// before the lookup. The record stays here because it needs the feature
// services; the dependency direction stays features → shared.
const LOG_SOURCES: Record<
	LogKind,
	(hostId: string, targetId: string) => LogSource
> = {
	container: containerLogs,
	quadlet: quadletLogs,
	pod: podLogs,
};

// Full-page log viewer for a container, quadlet, or pod. The list buttons navigate
// here instead of opening a small modal, so logs get the whole viewport.
// A friendly display name travels via navigation state when available.
export function LogsPage() {
	const { hostId, kind, targetId } = useParams<{
		hostId: string;
		kind: string;
		targetId: string;
	}>();
	const navigate = useNavigate();
	const location = useLocation();
	const state = location.state as LogsPageState | null;
	const name = state?.name ?? targetId ?? "";
	const [isDownloading, setIsDownloading] = useState(false);
	const toast = useToast();

	if (!hostId || !targetId || !isLogKind(kind)) {
		return null;
	}

	const source = LOG_SOURCES[kind](hostId, targetId);

	// One-shot snapshot download: fetches the entire history through the
	// source's fetch-all and saves it as a file. It never touches the
	// viewer's tail view or an active follow stream; a failure surfaces as
	// a toast, not inline view state.
	const handleDownload = async () => {
		setIsDownloading(true);
		try {
			const text = await source.fetchAll();
			downloadText(text, `${sanitizeFilename(name)}-logs.txt`);
		} catch (error) {
			toast({
				title: `Failed to download logs: ${name}`,
				// The source rejects with an ApiError-shaped object (the kit's
				// contract), not a raw axios error, so the kit's adapter reads it.
				description: toApiError(error).message,
				status: "error",
				isClosable: true,
			});
		} finally {
			setIsDownloading(false);
		}
	};

	return (
		<VStack align="stretch" spacing={4}>
			<HStack justify="space-between">
				{/* A long container/quadlet name is the one overflow risk on
				 * this page; the clamp engages only when it overflows, so
				 * desktop headings that fit are unchanged (spec #393). */}
				<Heading size="lg" noOfLines={1} minW={0}>
					Logs — {name}
				</Heading>
				<HStack spacing={2}>
					<Button
						variant="outline"
						isLoading={isDownloading}
						onClick={handleDownload}
						data-testid="logs-download-button"
					>
						Download
					</Button>
					<Button
						variant="outline"
						onClick={() => navigate(-1)}
						data-testid="logs-back-button"
					>
						Back
					</Button>
				</HStack>
			</HStack>
			<LogViewer source={source} maxHeight="70vh" />
		</VStack>
	);
}
