import { Text } from "@chakra-ui/react";
import { ErrorDisplay } from "@shared/components/ErrorDisplay";
import { getApiError } from "@shared/utils/api";
import type { ReactNode } from "react";

// The headless owner of the loading/error/empty triad for collection
// sites, tables and non-tables alike: it renders the three states and
// takes `children` for the populated state, owning the state JSX, the
// copy defaults, the styling, and the plural testid derivation
// ({resource}s-loading, -error, -empty). Props-based on purpose: the
// module never touches react-query; callers keep their hooks and hand
// over resolved state. `items` is deliberately non-generic — the module
// never reads item shape, and undefined counts as empty. Headless means
// no item layout and no table chrome; the loading/error/empty copy and
// display still render here, and there is no loading-copy override.
// Branches are mutually exclusive, tried in this order: isPending, then
// error, then undefined-or-empty, else children.
export function ResourceStates({
	resource,
	isPending,
	error,
	items,
	emptyMessage,
	children,
}: {
	resource: string;
	isPending: boolean;
	error: unknown;
	items: readonly unknown[] | undefined;
	emptyMessage?: string;
	children: ReactNode;
}) {
	const plural = `${resource}s`;

	return (
		<>
			{isPending ? (
				<Text data-testid={`${plural}-loading`}>Loading {plural}…</Text>
			) : error ? (
				<ErrorDisplay
					error={getApiError(error)}
					data-testid={`${plural}-error`}
				/>
			) : !items || items.length === 0 ? (
				<Text color="gray.500" data-testid={`${plural}-empty`}>
					{emptyMessage ?? `No ${plural} on this machine yet.`}
				</Text>
			) : (
				children
			)}
		</>
	);
}
