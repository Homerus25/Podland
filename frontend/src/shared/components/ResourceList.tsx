import {
	HStack,
	Table,
	TableContainer,
	Tbody,
	Th,
	Thead,
	Tr,
} from "@chakra-ui/react";
import { PermissionHint } from "@shared/components/PermissionHint";
import { ResourceStates } from "@shared/components/ResourceStates";
import { type ReactNode, useState } from "react";

export type ResourceListColumn<T> = {
	label?: string;
	isNumeric?: boolean;
	sortBy?: (item: T) => string | number;
};

// One owner for the machinery every feature list hand-rolled: the
// toolbar-or-hint row, the table chrome, and the plural table testid.
// The loading/error/empty triad is composed through ResourceStates, the
// headless owner those states share with the non-table collection sites.
// Props-based on purpose: the module never
// touches react-query; features keep their hooks and hand over resolved
// state, their columns as data, and their own row elements (rows carry the
// singular testid and their own key). Rows own their interactions: a row
// that is itself clickable keeps its onClick in the feature's row, and the
// action buttons inside such a row are responsible for stopPropagation so a
// button click does not also fire the row action. With `canManage` undefined
// the list is ungated: toolbar always, no hint ever. Column sorting rides the
// same ownership: a column declares an optional `sortBy` accessor, and the
// three-state header cycle (asc, desc, back to server order) is
// component-local state here — no default sort, and the state dies with the
// list.
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
	columns: ResourceListColumn<T>[];
	renderRow: (item: T) => ReactNode;
	toolbar?: ReactNode;
	canManage?: boolean;
	emptyMessage?: string;
}) {
	const plural = `${resource}s`;
	const [sort, setSort] = useState<{
		columnIndex: number;
		direction: "asc" | "desc";
	} | null>(null);

	let sortedItems = items;
	if (sort && items) {
		const accessor = columns[sort.columnIndex]?.sortBy;
		if (accessor) {
			const direction = sort.direction === "asc" ? 1 : -1;
			sortedItems = [...items].sort((a, b) => {
				const keyA = accessor(a);
				const keyB = accessor(b);
				const comparison =
					typeof keyA === "number" && typeof keyB === "number"
						? keyA - keyB
						: String(keyA).localeCompare(String(keyB));
				return comparison * direction;
			});
		}
	}

	const handleSortClick = (columnIndex: number) => {
		setSort((current) => {
			if (!current || current.columnIndex !== columnIndex) {
				return { columnIndex, direction: "asc" };
			}
			if (current.direction === "asc") {
				return { columnIndex, direction: "desc" };
			}
			return null;
		});
	};

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
			<ResourceStates
				resource={resource}
				isPending={isPending}
				error={error}
				items={items}
				emptyMessage={emptyMessage}
			>
				<TableContainer>
					<Table data-testid={`${plural}-table`} variant="simple">
						<Thead>
							<Tr>
								{columns.map((column, index) => {
									const isSortable = column.sortBy !== undefined;
									const isActiveSort = sort?.columnIndex === index;
									return (
										<Th
											// biome-ignore lint/suspicious/noArrayIndexKey: static columns are never reordered; position is their identity
											key={index}
											isNumeric={column.isNumeric}
											cursor={isSortable ? "pointer" : undefined}
											data-testid={
												isSortable ? `sort-${resource}-${index}` : undefined
											}
											aria-sort={
												isActiveSort
													? sort?.direction === "asc"
														? "ascending"
														: "descending"
													: undefined
											}
											onClick={
												isSortable ? () => handleSortClick(index) : undefined
											}
										>
											{column.label}
											{isActiveSort
												? sort?.direction === "asc"
													? " ▲"
													: " ▼"
												: null}
										</Th>
									);
								})}
							</Tr>
						</Thead>
						<Tbody>{sortedItems?.map((item) => renderRow(item))}</Tbody>
					</Table>
				</TableContainer>
			</ResourceStates>
		</>
	);
}
