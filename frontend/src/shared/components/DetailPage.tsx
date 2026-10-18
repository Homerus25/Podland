import {
	Box,
	Button,
	Heading,
	HStack,
	Spinner,
	Text,
	VStack,
} from "@chakra-ui/react";
import { ErrorDisplay } from "@shared/components/ErrorDisplay";
import {
	type DetailResource,
	machineTabPath,
} from "@shared/routes/detailRoute";
import { getApiError } from "@shared/utils/api";
import type { ReactNode } from "react";
import { Link, useParams } from "react-router-dom";

// The structural slice of a react-query useQuery result the layout renders
// for the lazy inspect fetch: the pending flag, the error, and the raw
// report payload. Features keep their query hooks; the module never
// touches react-query.
export interface InspectQuerySlice {
	isPending: boolean;
	error: unknown;
	data: unknown;
}

// One owner of the resource detail page shell (spec 301): the header with
// the item's display name and a back button that preserves the machine
// detail page's active tab (?tab=<resource>, the spec 297 deep link), the
// overview slot for the resource's label/value rows, and the "show inspect
// JSON" collapsible at the bottom. The module never touches react-query:
// the page adapter owns its queries and hands over their structural slices.
export type DetailPageProps = {
	// The item's display name, rendered in the header.
	title: string;
	// The machine tab the back button returns to (?tab=<backTo>).
	backTo: DetailResource;
	// The overview query's pending flag: the loading state renders while true.
	isPending: boolean;
	// The overview query's error, rendered via ErrorDisplay with getApiError.
	error: unknown;
	// True when the overview query 404'd: the not-found state renders.
	notFound: boolean;
	// The lazy inspect query's structural slice (the module's
	// InspectQuerySlice): the page enables the query on first reveal, so
	// the report is fetched on the first "Show inspect JSON" click, never
	// on page mount.
	inspectQuery: InspectQuerySlice;
	// Whether the inspect JSON section is revealed; the page owns this state.
	revealed: boolean;
	onToggleInspect: () => void;
	// The overview: the resource's label/value rows.
	children: ReactNode;
};

export function DetailPage({
	title,
	backTo,
	isPending,
	error,
	notFound,
	inspectQuery,
	revealed,
	onToggleInspect,
	children,
}: DetailPageProps) {
	const { machineId } = useParams<{ machineId: string }>();
	const report = inspectQuery.data;
	const pretty = report === undefined ? "" : JSON.stringify(report, null, 2);

	return (
		<VStack align="stretch" spacing={4}>
			<HStack spacing={4} flexWrap="wrap">
				<Button
					as={Link}
					to={machineTabPath(machineId!, backTo)}
					variant="outline"
					data-testid="detail-back-button"
				>
					Back
				</Button>
				<Heading size="lg" data-testid="detail-page-title">
					{title}
				</Heading>
			</HStack>
			{isPending ? (
				<Box data-testid="detail-page-loading">
					<Spinner size="sm" /> Loading...
				</Box>
			) : notFound ? (
				<Text data-testid="detail-page-not-found">
					This item no longer exists on the machine.
				</Text>
			) : error ? (
				<ErrorDisplay
					error={getApiError(error)}
					data-testid="detail-page-error"
				/>
			) : (
				<Box data-testid="detail-page-overview">{children}</Box>
			)}
			<Box>
				<Button
					variant="outline"
					onClick={onToggleInspect}
					aria-expanded={revealed}
					data-testid="toggle-inspect-button"
				>
					{revealed ? "Hide inspect JSON" : "Show inspect JSON"}
				</Button>
				{revealed ? (
					<Box mt={2}>
						{inspectQuery.isPending ? (
							<Box data-testid="inspect-json-loading">
								<Spinner size="sm" /> Loading inspect data…
							</Box>
						) : inspectQuery.error ? (
							<ErrorDisplay
								error={getApiError(inspectQuery.error)}
								data-testid="inspect-json-error"
							/>
						) : (
							<>
								<Box
									as="pre"
									overflow="auto"
									maxHeight="60vh"
									fontFamily="mono"
									data-testid="inspect-json"
								>
									{pretty}
								</Box>
								{report !== undefined ? (
									<Button
										mt={2}
										data-testid="copy-inspect-button"
										onClick={() => navigator.clipboard.writeText(pretty)}
									>
										Copy JSON
									</Button>
								) : null}
							</>
						)}
					</Box>
				) : null}
			</Box>
		</VStack>
	);
}
