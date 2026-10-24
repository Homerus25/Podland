import { Progress, Text, VStack } from "@chakra-ui/react";
import type { PullProgress } from "@features/images/hooks/usePullImage";
import { formatBytes } from "@shared/utils/format";

export interface PullProgressView {
	status: string;
	message?: string;
	percent?: number;
	byteLine?: string;
}

// The pure projection from the pull stream's progress state: the status
// text with its "Starting pull…" fallback, the optional message, and the
// percent and byte line only when the stream reports amounts.
export function projectPullProgress(
	progress: PullProgress | null,
): PullProgressView {
	const state = progress?.state;
	const current = state?.current;
	const total = state?.total;
	const percent =
		current !== undefined && total !== undefined && total > 0
			? Math.round((current / total) * 100)
			: undefined;

	return {
		status: state?.status ?? "Starting pull…",
		message: state?.message,
		percent,
		byteLine:
			percent !== undefined
				? `${formatBytes(current ?? 0)} / ${formatBytes(total ?? 0)}`
				: undefined,
	};
}

// The single owner of the pull-progress presentation (spec #371, ticket
// #364): the images tab's pull form and the create page's image-not-found
// flow both render through it. Extra props spread onto the root so each
// call site keeps its own test id (the ErrorDisplay precedent).
export function PullProgressDisplay({
	progress,
	...rest
}: { progress: PullProgress | null } & Record<string, unknown>) {
	const view = projectPullProgress(progress);

	return (
		<VStack spacing={1} align="stretch" {...rest}>
			<Text fontSize="sm">{view.status}</Text>
			{view.message ? (
				<Text fontSize="sm" color="gray.500">
					{view.message}
				</Text>
			) : null}
			{view.percent !== undefined ? (
				<>
					<Progress value={view.percent} size="sm" />
					<Text fontSize="sm" color="gray.500">
						{view.byteLine}
					</Text>
				</>
			) : null}
		</VStack>
	);
}
