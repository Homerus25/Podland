import {
	HStack,
	Table,
	TableContainer,
	Tbody,
	Text,
	Th,
	Thead,
	Tr,
} from "@chakra-ui/react";
import { ErrorDisplay } from "@shared/components/ErrorDisplay";
import { PermissionHint } from "@shared/components/PermissionHint";
import { getApiError } from "@shared/utils/api";
import type { ReactNode } from "react";

export type ResourceListColumn = {
	label?: string;
	isNumeric?: boolean;
};

// One owner for the machinery every feature list hand-rolled: the
// toolbar-or-hint row, the loading/error/empty/table triad, the table
// chrome, and the plural testids. Props-based on purpose: the module never
// touches react-query; features keep their hooks and hand over resolved
// state, their columns as data, and their own row elements (rows carry the
// singular testid and their own key). Rows own their interactions: a row
// that is itself clickable keeps its onClick in the feature's row, and the
// action buttons inside such a row are responsible for stopPropagation so a
// button click does not also fire the row action. With `canManage` undefined
// the list is ungated: toolbar always, no hint ever.
export function ResourceList<T>({
	resource,
	isPending,
	error,
	items,
	columns,
	renderRow,
	toolbar,
	canManage,
	emptyMessage,
}: {
	resource: string;
	isPending: boolean;
	error: unknown;
	items: T[] | undefined;
	columns: ResourceListColumn[];
	renderRow: (item: T) => ReactNode;
	toolbar?: ReactNode;
	canManage?: boolean;
	emptyMessage?: string;
}) {
	const plural = `${resource}s`;

	return (
		<>
			{canManage === false ? (
				<PermissionHint
					scope={plural}
					requirement="write"
					resource={resource}
					mb={2}
				/>
			) : (
				toolbar && (
					<HStack justify="flex-end" mb={2}>
						{toolbar}
					</HStack>
				)
			)}
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
				<TableContainer>
					<Table data-testid={`${plural}-table`} variant="simple">
						<Thead>
							<Tr>
								{columns.map((column, index) => (
									// biome-ignore lint/suspicious/noArrayIndexKey: static columns are never reordered; position is their identity
									<Th key={index} isNumeric={column.isNumeric}>
										{column.label}
									</Th>
								))}
							</Tr>
						</Thead>
						<Tbody>{items.map((item) => renderRow(item))}</Tbody>
					</Table>
				</TableContainer>
			)}
		</>
	);
}
