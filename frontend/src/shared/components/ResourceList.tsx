import {
	Button,
	Checkbox,
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

// Row selection (spec 461, ticket 463) is opt-in: a list that passes no
// `selection` prop renders exactly as it did before. The adapter supplies
// the row's stable identity — the container/image/pod/quadlet/secret `id`,
// the volume/network `name` — never the row index, so the selection survives
// sorting and refetches. `onChange` is the adapter's reaction seam; the
// owner keeps the selected-key set itself, so the adapter never holds a
// second copy of the state.
export type ResourceListSelection<T> = {
	getKey: (item: T) => string;
	onChange?: (selectedKeys: string[]) => void;
};

// What the owner hands back to the adapter: the live selection, projected
// onto the current items. `selectedItems` follows the list's current order
// (so a sorted list reports its selection sorted too) and drops keys that
// are no longer in `items`; `clear` is the post-mutation reset the adapter
// calls once a bulk action has removed the selected rows. `clearKeys` is the
// partial reset a batch run needs (spec 486, ticket #487): it drops exactly
// the named keys, so a bulk transfer clears only the items that succeeded
// and leaves the failures selected for the retry. Both resets go through the
// same `applySelection`, so `onChange` fires either way.
export type ResourceListSelectionState<T> = {
	selectedKeys: string[];
	selectedItems: T[];
	clear: () => void;
	clearKeys: (keys: string[]) => void;
};

// The snapshot a batch adapter takes when it opens its dialog (spec 486): the
// selected items as they stood at that moment, plus the selection's own
// partial reset. The list owns the selection, so the adapter never keeps a
// second copy of it — it holds this snapshot and hands `clearKeys` back when
// the run finishes, so the report keeps naming the items after the list drops
// the successes from its selection.
export type ResourceListSelectionSnapshot<T> = {
	items: T[];
	clearKeys: (keys: string[]) => void;
};

export function snapshotSelection<T>(
	selection: ResourceListSelectionState<T>,
): ResourceListSelectionSnapshot<T> {
	return { items: selection.selectedItems, clearKeys: selection.clearKeys };
}

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

// A clickable row is a button: a Tr carrying onClick gets this hover
// affordance — tint plus shadow and a 1px lift — so the row reads as
// actionable under the cursor. Applied in the same row walk that owns the
// pins and the checkbox column, so every feature row gets it for free.
const clickableRowHoverProps = {
	transition:
		"background-color 0.15s ease-in-out, box-shadow 0.15s ease-in-out, transform 0.15s ease-in-out",
	_hover: {
		backgroundColor: "gray.100",
		_dark: { backgroundColor: "gray.700" },
		boxShadow: "0 2px 10px rgba(0, 0, 0, 0.18)",
		transform: "translateY(-1px)",
	},
};

// Rows are feature-rendered, so the pin contract has to reach the feature
// <Td>s, not just the headers this module renders. The owner walks the
// row's rendered node: fragments (a data row with sibling error rows or
// modals) are recursed into, and each <Tr>'s <Td> children are pinned by
// cell position — the same position the columns array describes. Cells
// with a colSpan (a row-level error row) span columns, so they are never
// pin targets.
//
// The same walk owns the selection column (spec 461, ticket 463): the
// checkbox cell is prepended here rather than by the feature rows, so the
// seven feature rows keep their exact <Td> list. A spanning row is a
// row-level error, not a data row — it gets no checkbox, and its colSpan
// grows by one so the table stays valid while the checkbox column exists.
// The checkbox column is a middle column: it is never a pin target, so it
// scrolls with the rest of the middle.
function decorateRowCells(
	children: ReactNode,
	selectCell: ReactNode | null,
	nameIndex: number,
	actionsIndex: number | null,
): ReactNode {
	// A row whose cells span columns is a row-level error, not a data row:
	// it gets no checkbox, and its span grows with the checkbox column so
	// the table stays valid.
	const isSpanningRow = Children.toArray(children).some(
		(child) => isValidElement(child) && child.props.colSpan !== undefined,
	);
	const decorated: ReactNode[] = [];
	if (selectCell !== null && !isSpanningRow) decorated.push(selectCell);
	let cellIndex = -1;
	Children.forEach(children, (child) => {
		if (!isValidElement(child)) {
			decorated.push(child);
			return;
		}
		cellIndex += 1;
		if (child.type !== Td) {
			decorated.push(child);
			return;
		}
		if (child.props.colSpan !== undefined) {
			decorated.push(
				selectCell === null
					? child
					: cloneElement(child, { colSpan: child.props.colSpan + 1 }),
			);
			return;
		}
		if (cellIndex === nameIndex) {
			decorated.push(cloneElement(child, pinnedNameCellProps));
			return;
		}
		if (actionsIndex !== null && cellIndex === actionsIndex) {
			decorated.push(cloneElement(child, pinnedActionsCellProps));
			return;
		}
		decorated.push(child);
	});
	// Children.toArray assigns the positional keys the feature's static <Td>s
	// do not carry, so the rebuilt cell list never trips React's key warning.
	return Children.toArray(decorated);
}

// The row walk: recurses fragments, and on each Tr applies the clickable-row
// hover (a Tr with onClick) plus the cell decoration below.
function decorateRow(
	node: ReactNode,
	selectCell: ReactNode | null,
	nameIndex: number,
	actionsIndex: number | null,
): ReactNode {
	if (!isValidElement(node)) return node;
	if (node.type === Fragment) {
		return cloneElement(node, {
			children: Children.map(node.props.children, (child) =>
				decorateRow(child, selectCell, nameIndex, actionsIndex),
			),
		});
	}
	if (node.type === Tr) {
		return cloneElement(node, {
			...(node.props.onClick ? clickableRowHoverProps : {}),
			children: decorateRowCells(
				node.props.children,
				selectCell,
				nameIndex,
				actionsIndex,
			),
		});
	}
	return node;
}

// One owner for the plumbing every feature list hand-rolled: the
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
// list. Row selection rides the same ownership: the adapter opts in with a
// `selection` prop carrying the row's stable identity, and the owner keeps
// the selected-key set (component-local, so it dies with the list too),
// renders the checkbox column and the select-all header, and hands the live
// selection back through `onChange` and the owner-rendered "Remove selected"
// button's `onRemoveSelected` handler.
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
	selection,
	canSelect,
	onRemoveSelected,
	selectionActions,
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
	// Selection is opt-in and gated on who may act through it: the caller
	// that can use a selection action. That is the toolbar's write grant by
	// default — no `selection` prop, or a caller that has not been granted
	// write, means no checkbox column at all, so a read-only list never
	// offers a selection it cannot act on. The gate is `canManage === true`,
	// not `!== false`: an omitted `canManage` means "ungated" for the toolbar
	// (which then renders), but it is not a write grant, so a list that asks
	// for selection without saying who may write renders no checkboxes
	// rather than offering a read-only caller an action that would 403. A
	// list whose selection action rides a LOWER capability than its
	// toolbar's write grant passes `canSelect` explicitly (spec 486,
	// ticket #491): the containers list's bulk connect rides
	// manage-networks while its toolbar rides manage-containers, so a
	// network-only admin still gets the checkboxes and the connect button —
	// the adapter gates its own toolbar buttons and the owner keeps Remove
	// selected on the toolbar's write grant.
	selection?: ResourceListSelection<T>;
	canSelect?: boolean;
	onRemoveSelected?: (selection: ResourceListSelectionState<T>) => void;
	// The adapter's own selection-driven toolbar buttons (spec 486, ticket
	// #487), rendered in the selection group beside the owner's Remove
	// selected button. Gated like the checkboxes (the selection gate, so a
	// `canSelect` list renders them below its toolbar's write grant), and
	// rendered ALWAYS — never gated on a non-empty selection, because the
	// adapter's buttons disable themselves (a bulk transfer's button is
	// permanently visible and disabled while nothing is selected).
	// Read-only callers keep the permission-hint swap untouched.
	selectionActions?: (selection: ResourceListSelectionState<T>) => ReactNode;
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
	// Selection is opt-in and gated on who may act through it (spec 486,
	// ticket #491): the toolbar's write grant by default, or the adapter's
	// explicit `canSelect` when its selection action rides a lower
	// capability than its toolbar does — the containers list's bulk
	// connect rides manage-networks while its toolbar rides
	// manage-containers. The gate is a real grant, not `!== false`: an
	// omitted `canManage` means "ungated" for the toolbar (which then
	// renders), but it is not a write grant, so a list that asks for
	// selection without saying who may act renders no checkboxes rather
	// than offering a read-only caller an action that would 403.
	const getKey = selection?.getKey;
	const selectionCapable = canSelect ?? canManage === true;
	const isSelectable = getKey !== undefined && selectionCapable;
	const [selectedKeys, setSelectedKeys] = useState<string[]>([]);

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

	// The selection is projected onto the current rows on every render, so
	// it follows the sort order and never holds a stale item: keys that are
	// no longer in `items` stay in `selectedKeys` (the adapter clears them
	// through `clear` once its bulk action has removed the rows) but drop
	// out of `selectedItems`.
	const selectedItems =
		isSelectable && getKey
			? (sortedItems ?? []).filter((item) =>
					selectedKeys.includes(getKey(item)),
				)
			: [];
	const allSelected =
		isSelectable &&
		getKey !== undefined &&
		(sortedItems?.length ?? 0) > 0 &&
		(sortedItems ?? []).every((item) => selectedKeys.includes(getKey(item)));

	const applySelection = (next: string[]) => {
		setSelectedKeys(next);
		selection?.onChange?.(next);
	};

	const toggleRow = (key: string) => {
		applySelection(
			selectedKeys.includes(key)
				? selectedKeys.filter((selected) => selected !== key)
				: [...selectedKeys, key],
		);
	};

	// Select-all is a toggle over every row: select them all when they are
	// not all selected, clear the selection when they are.
	const toggleAll = () => {
		if (!getKey) return;
		applySelection(allSelected ? [] : (sortedItems ?? []).map(getKey));
	};

	const selectionState: ResourceListSelectionState<T> = {
		selectedKeys,
		selectedItems,
		clear: () => applySelection([]),
		clearKeys: (keys) =>
			applySelection(selectedKeys.filter((key) => !keys.includes(key))),
	};

	// The "Remove selected" button (spec 461, ticket #493) is owner-rendered,
	// not a toolbar slot the adapter fills: every selectable list hands over
	// only its reaction (`onRemoveSelected`), and the owner draws the one
	// button — label, red outline, derived testid, and its left-edge place
	// on the toolbar row — so the placement cannot drift per feature. Like
	// the adapter's own selection actions (spec 486), the button renders
	// permanently — never gated on a non-empty selection — and disables
	// itself while nothing is selected. The gate is the toolbar's write
	// grant — `canManage === true`, not the wider selection gate a
	// `canSelect` list opens for its own action — because removal is a
	// container-write act even on a list whose connect action rides
	// manage-networks.
	const removeSelectedButton =
		onRemoveSelected && canManage === true ? (
			<Button
				size="sm"
				colorScheme="red"
				variant="outline"
				isDisabled={selectedKeys.length === 0}
				onClick={() => onRemoveSelected(selectionState)}
				data-testid={`remove-selected-${plural}-button`}
			>
				Remove selected
			</Button>
		) : null;

	// The selection group (spec 486, ticket #487): the owner's Remove
	// selected button and the adapter's own selection actions share one
	// left-edge wrapper, Remove selected first. The auto margin lives on the
	// wrapper, not on the button, so the group stays at the left of its line
	// even when the toolbar wraps below `lg` (spec 393) and the group ends up
	// alone on a wrapped line. The adapter's actions render for a
	// selection-capable caller only and always — they disable themselves —
	// so the group's presence is the union of the two.
	const selectionActionsNode =
		selectionCapable && selectionActions
			? selectionActions(selectionState)
			: null;
	const selectionGroup =
		removeSelectedButton || selectionActionsNode ? (
			<HStack spacing={2} marginRight="auto">
				{removeSelectedButton}
				{selectionActionsNode}
			</HStack>
		) : null;

	return (
		<>
			{canManage === false && !selectionCapable ? (
				<PermissionHint
					scope={plural}
					requirement="write"
					resource={resource}
					mb={2}
				/>
			) : (
				(toolbar || selectionGroup) && (
					// The toolbar wrapper wraps its buttons below `lg` instead of
					// overflowing the viewport (spec 393); Chakra's Stack spaces
					// wrapped children through gap, so the rows stay evenly spaced.
					// The selection group leads the row at the left edge and its
					// auto margin holds the adapter's own toolbar to the right.
					<HStack
						justify="flex-end"
						mb={2}
						flexWrap={{ base: "wrap", lg: "nowrap" }}
						data-testid={`${plural}-toolbar`}
					>
						{selectionGroup}
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
								{isSelectable ? (
									// The select-all cell leads the header but is never a
									// pin target: the checkbox column scrolls as a middle
									// column, and the pin indices stay in column space.
									<Th>
										<Checkbox
											aria-label={`Select all ${plural}`}
											isChecked={allSelected}
											onChange={toggleAll}
										/>
									</Th>
								) : null}
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
								const key = getKey?.(item);
								const selectCell =
									isSelectable && key !== undefined ? (
										// The checkbox cell is the owner's, so it also owns
										// keeping the click off a clickable row's action: a
										// feature row that navigates on click must not
										// navigate when the user is only selecting it.
										<Td
											key="select"
											onClick={(event) => event.stopPropagation()}
										>
											<Checkbox
												aria-label={`Select ${key}`}
												isChecked={selectedKeys.includes(key)}
												onChange={() => toggleRow(key)}
											/>
										</Td>
									) : null;
								return decorateRow(row, selectCell, nameIndex, actionsIndex);
							})}
						</Tbody>
					</Table>
				</TableContainer>
			</ResourceStates>
		</>
	);
}
