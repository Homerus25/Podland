import { pullStreamUrl } from "@features/images/services/imagesApi";
import { invalidateHostResources } from "@shared/queries/useListInvalidatingMutation";
import {
	isStreamEvent,
	type StreamEventState,
} from "@shared/streaming/ndjsonStream";
import { useNdjsonOperation } from "@shared/streaming/streamTransport";
import type { ApiError } from "@shared/types/api";
import { useQueryClient } from "@tanstack/react-query";
import { useCallback, useState } from "react";

export interface PullProgress {
	operationId: string;
	state?: StreamEventState;
}

interface UsePullImageResult {
	isPulling: boolean;
	progress: PullProgress | null;
	error: ApiError | null;
	pull: (image: string) => void;
	cancel: () => void;
}

// The image pull rides the shared stream transport: the abort, supersede,
// terminal, and error-mapping rules are the module's; this hook keeps only
// the pull request, the progress state, and the images invalidation.
export function usePullImage(hostId: string): UsePullImageResult {
	const queryClient = useQueryClient();
	const [progress, setProgress] = useState<PullProgress | null>(null);
	const { isRunning, error, start, cancel } = useNdjsonOperation();

	const pull = useCallback(
		(image: string) => {
			setProgress(null);
			start({
				url: pullStreamUrl(hostId),
				init: { method: "POST", body: JSON.stringify({ image }) },
				onEvent: (value) => {
					if (!isStreamEvent(value)) {
						console.warn(
							"Skipping malformed stream event (unexpected shape):",
							value,
						);
						return;
					}
					if (value.type === "progress") {
						setProgress({
							operationId: value.operationId,
							state: value.state,
						});
					} else if (value.type === "complete") {
						invalidateHostResources(queryClient, hostId, "images");
					}
				},
			});
		},
		[hostId, queryClient, start],
	);

	return { isPulling: isRunning, progress, error, pull, cancel };
}
