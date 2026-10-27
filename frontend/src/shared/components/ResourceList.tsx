import {
	HStack,
	Table,
	TableContainer,
	Tbody,
	Td,
	Th,
	Thead,
	Tr,
} from "@chakra-ui/react";
import { PermissionHint } from "@shared/components/PermissionHint";
import { ResourceStates } from "@shared/components/ResourceStates";
import {
	Children,
	cloneElement,
	Fragment,
	isValidElement,
	type ReactNode,
	useState,
} from "react";

export type ResourceListColumn<T> = {
	label?: string;
	isNumeric?: boolean;
	// The name-column marker (spec 393): a list marks the column that pins
	// left below `lg`. The owner owns every other part of the pinning — the
	// trailing label-less action column pins right, and both pins reach the
	// feature-rendered body cells, not just the headers.
	isName?: boolean;
	sortBy?: (item: T) => string | number;
};

// The mobile pin styles (spec 393), applied below `lg` only — at `lg` and
// up every value returns to today's rendering (static, transparent, no
// shadow), so desktop is unchanged. The surface background mirrors the
// theme's body background (gray.50, gray.800 in dark mode) so the middle
// columns scrolling beneath the pins never show through them. One factory
// for both sides so they cannot drift: the name pin sticks left, the
// actions pin right, and the shadow points inward from the pinned edge.
function pinnedCellProps(side: "left" | "right") {
	return {
		position: { base: "sticky", lg: "static" },
		[side]: { base: "0", lg: "auto" },
		backgroundColor: { base: "gray.50", lg: "transparent" },
		_dark: { backgroundColor: { base: "gray.800", lg: "transparent" } },
		boxShadow: {
			base: `${side === "left" ? "4px" : "-4px"} 0 8px -4px rgba(0, 0, 0, 0.15)`,
			lg: "none",
		},
	};
}

const pinnedNameCellProps = pinnedCellProps("left");
const pinnedActionsCellProps = pinnedCellProps("right");

// Rows are feature-rendered, so the pin contract has to reach the feature
// <Td>s, not just the headers this module renders. The owner walks the
// row's rendered node: fragments (a data row with sibling error rows or
// modals) are recursed into, and each <Tr>'s <Td> children are pinned by
// cell position — the same position the columns array describes. Cells
// with a colSpan (a row-level error row) span columns, so they are never
// pin targets.
function pinTableCells(
	children: ReactNode,
	nameIndex: number,
	actionsIndex: number | null,
): ReactNode {
	let cellIndex = -1;
	return Children.map(children, (child) => {
		if (!isValidElement(child)) return child;
		cellIndex += 1;
		if (child.type !== Td || child.props.colSpan !== undefined) {
			return child;
		}
		if (cellIndex === nameIndex) {
			return cloneElement(child, pinnedNameCellProps);
		}
		if (actionsIndex !== null && cellIndex === actionsIndex) {
			return cloneElement(child, pinnedActionsCellProps);
		}
		return child;
	});
}

function pinRowCells(
	node: ReactNode,
	nameIndex: number,
	actionsIndex: number | null,
): ReactNode {
	if (!isValidElement(node)) return node;
	if (node.type === Fragment) {
		return cloneElement(node, {
			children: Children.map(node.props.children, (child) =>
				pinRowCells(child, nameIndex, actionsIndex),
			),
		});
	}
	if (node.type === Tr) {
		return cloneElement(node, {
			children: pinTableCells(node.props.children, nameIndex, actionsIndex),
		});
	}
	return node;
}

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
	// Pinning activates only when a list marks its name column; an
	// unmarked list renders exactly as before. The right pin is the
	// trailing action column — the last column when it is label-less —
	// so conditional permission columns and multi-action lists (the
	// quadlets' four trailing columns) need no extra markers: only the
	// final column pins, the columns before it scroll as middle columns.
	const nameIndex = columns.findIndex((column) => column.isName);
	const isPinned = nameIndex !== -1;
	const lastColumn =
		columns.length > 0 ? columns[columns.length - 1] : undefined;
	const actionsIndex =
		isPinned && lastColumn?.label === undefined ? columns.length - 1 : null;
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
					// The toolbar wrapper wraps its buttons below `lg` instead of
					// overflowing the viewport (spec 393); Chakra's Stack spaces
					// wrapped children through gap, so the rows stay evenly spaced.
					<HStack
						justify="flex-end"
						mb={2}
						flexWrap={{ base: "wrap", lg: "nowrap" }}
						data-testid={`${plural}-toolbar`}
					>
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
									const pinnedCellProps =
										index === nameIndex
											? pinnedNameCellProps
											: index === actionsIndex
												? pinnedActionsCellProps
												: {};
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
											{...pinnedCellProps}
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
						<Tbody>
							{sortedItems?.map((item) => {
								const row = renderRow(item);
								return isPinned
									? pinRowCells(row, nameIndex, actionsIndex)
									: row;
							})}
						</Tbody>
					</Table>
				</TableContainer>
			</ResourceStates>
		</>
	);
}
