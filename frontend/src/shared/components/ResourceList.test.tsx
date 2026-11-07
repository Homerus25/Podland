import { Button, Td, Tr } from "@chakra-ui/react";
import { ResourceList } from "@shared/components/ResourceList";
import { act, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Fragment } from "react";
import { describe, expect, it, vi } from "vitest";
import { renderWithQuery } from "@/test/render";
import { breakpointRules } from "@/test/responsiveCss";

type Gadget = { id: string; name: string };

const gadgets: Gadget[] = [
	{ id: "g-1", name: "spinner" },
	{ id: "g-2", name: "doodad" },
];

function renderGadgetRow(gadget: Gadget) {
	return (
		<Tr key={gadget.id} data-testid="gadget-row">
			<Td>{gadget.name}</Td>
		</Tr>
	);
}

function renderResourceList(
	overrides: Partial<Parameters<typeof ResourceList<Gadget>>[0]> = {},
) {
	const props = {
		resource: "gadget",
		isPending: false,
		error: undefined as unknown,
		items: gadgets as Gadget[] | undefined,
		columns: [{ label: "Name" }],
		renderRow: renderGadgetRow,
		toolbar: (
			<Button size="sm" data-testid="gadgets-toolbar-action">
				Do a thing
			</Button>
		),
		...overrides,
	};
	return render(<ResourceList {...props} />);
}

describe("ResourceList composition", () => {
	it("renders the triad through ResourceStates, the table with the toolbar still above it", () => {
		renderResourceList();

		expect(screen.getByTestId("gadgets-table")).toBeInTheDocument();
		expect(screen.getAllByTestId("gadget-row")).toHaveLength(2);
		expect(screen.getByText("spinner")).toBeInTheDocument();
		expect(screen.getByText("doodad")).toBeInTheDocument();
		expect(screen.getByTestId("gadgets-toolbar-action")).toBeInTheDocument();
	});
});

describe("ResourceList permission hint", () => {
	it("swaps the toolbar for the canonical read hint when canManage is false", () => {
		renderResourceList({ canManage: false });

		const hint = screen.getByTestId("gadgets-permission-hint");
		expect(hint).toHaveAttribute("data-requirement", "write");
		expect(
			screen.queryByTestId("gadgets-toolbar-action"),
		).not.toBeInTheDocument();
		expect(screen.getByTestId("gadgets-table")).toBeInTheDocument();
	});

	it("renders ungated with the toolbar and no hint when canManage is omitted", () => {
		renderResourceList();

		expect(screen.getByTestId("gadgets-toolbar-action")).toBeInTheDocument();
		expect(
			screen.queryByTestId("gadgets-permission-hint"),
		).not.toBeInTheDocument();
	});
});

describe("ResourceList columns", () => {
	it("renders one header cell per column, numeric-aligned", () => {
		renderResourceList({
			columns: [{ label: "Name" }, { isNumeric: true }, {}],
		});

		const table = screen.getByTestId("gadgets-table");
		const headers = within(table).getAllByRole("columnheader");
		expect(headers).toHaveLength(3);
		expect(headers[0]).toHaveTextContent("Name");
		expect(headers[1]).toHaveAttribute("data-is-numeric", "true");
		expect(headers[0]).not.toHaveAttribute("data-is-numeric");
		expect(headers[2]).toBeEmptyDOMElement();
	});
});

describe("ResourceList sorting", () => {
	type SortableGadget = { id: string; name: string; size: number };

	// Server order is deliberately neither alphabetical nor numeric.
	const sortableGadgets: SortableGadget[] = [
		{ id: "g-1", name: "cherry", size: 10 },
		{ id: "g-2", name: "apple", size: 9 },
		{ id: "g-3", name: "banana", size: 100 },
	];

	function renderSortableRow(gadget: SortableGadget) {
		return (
			<Tr key={gadget.id} data-testid="gadget-row">
				<Td data-testid="gadget-name">{gadget.name}</Td>
				<Td>{gadget.size}</Td>
				<Td />
			</Tr>
		);
	}

	function renderSortableList() {
		return render(
			<ResourceList
				resource="gadget"
				isPending={false}
				error={undefined}
				items={sortableGadgets}
				columns={[
					{ label: "Name", sortBy: (gadget: SortableGadget) => gadget.name },
					{
						label: "Size",
						isNumeric: true,
						sortBy: (gadget: SortableGadget) => gadget.size,
					},
					{},
				]}
				renderRow={renderSortableRow}
			/>,
		);
	}

	function rowNames(): string[] {
		return screen
			.getAllByTestId("gadget-name")
			.map((cell) => cell.textContent ?? "");
	}

	it("cycles a sortable header through asc, desc, and back to server order", async () => {
		const user = userEvent.setup();
		renderSortableList();

		const header = screen.getByTestId("sort-gadget-0");
		expect(rowNames()).toEqual(["cherry", "apple", "banana"]);

		await user.click(header);
		expect(rowNames()).toEqual(["apple", "banana", "cherry"]);

		await user.click(header);
		expect(rowNames()).toEqual(["cherry", "banana", "apple"]);

		await user.click(header);
		expect(rowNames()).toEqual(["cherry", "apple", "banana"]);
	});

	it("marks only the active column with aria-sort", async () => {
		const user = userEvent.setup();
		renderSortableList();

		const nameHeader = screen.getByTestId("sort-gadget-0");
		const sizeHeader = screen.getByTestId("sort-gadget-1");

		expect(nameHeader).not.toHaveAttribute("aria-sort");
		expect(sizeHeader).not.toHaveAttribute("aria-sort");

		await user.click(nameHeader);
		expect(nameHeader).toHaveAttribute("aria-sort", "ascending");
		expect(sizeHeader).not.toHaveAttribute("aria-sort");

		await user.click(nameHeader);
		expect(nameHeader).toHaveAttribute("aria-sort", "descending");
		expect(sizeHeader).not.toHaveAttribute("aria-sort");

		await user.click(nameHeader);
		expect(nameHeader).not.toHaveAttribute("aria-sort");
	});

	it("sorts string keys with localeCompare", async () => {
		const user = userEvent.setup();
		renderSortableList();

		await user.click(screen.getByTestId("sort-gadget-0"));
		expect(rowNames()).toEqual(["apple", "banana", "cherry"]);
	});

	it("sorts number keys numerically, not lexicographically", async () => {
		const user = userEvent.setup();
		renderSortableList();

		await user.click(screen.getByTestId("sort-gadget-1"));
		// Numeric: 9 < 10 < 100. Lexicographic would order 10 < 100 < 9.
		expect(rowNames()).toEqual(["apple", "cherry", "banana"]);
	});

	it("ignores clicks on columns without sortBy", async () => {
		const user = userEvent.setup();
		renderSortableList();

		const table = screen.getByTestId("gadgets-table");
		const actionHeader = within(table).getAllByRole("columnheader")[2];

		expect(actionHeader).not.toHaveAttribute("data-testid");
		await user.click(actionHeader);
		expect(rowNames()).toEqual(["cherry", "apple", "banana"]);
		expect(actionHeader).not.toHaveAttribute("aria-sort");
	});

	it("resets the sort when the list remounts", async () => {
		const user = userEvent.setup();
		const { unmount } = renderSortableList();

		await user.click(screen.getByTestId("sort-gadget-0"));
		expect(rowNames()).toEqual(["apple", "banana", "cherry"]);

		unmount();
		renderSortableList();
		expect(rowNames()).toEqual(["cherry", "apple", "banana"]);
	});

	it("shows a direction arrow on the active column only, with pointer cursor", async () => {
		const user = userEvent.setup();
		renderSortableList();

		const nameHeader = screen.getByTestId("sort-gadget-0");
		const sizeHeader = screen.getByTestId("sort-gadget-1");
		expect(nameHeader.textContent).toBe("Name");
		expect(nameHeader).toHaveStyle("cursor: pointer");

		await user.click(nameHeader);
		expect(nameHeader.textContent).toBe("Name ▲");
		expect(sizeHeader.textContent).toBe("Size");

		await user.click(nameHeader);
		expect(nameHeader.textContent).toBe("Name ▼");

		await user.click(nameHeader);
		expect(nameHeader.textContent).toBe("Name");
	});

	it("derives the sortable header testid from resource and column index", () => {
		renderSortableList();

		expect(screen.getByTestId("sort-gadget-0")).toHaveTextContent("Name");
		expect(screen.getByTestId("sort-gadget-1")).toHaveTextContent("Size");
		expect(screen.queryByTestId("sort-gadget-2")).not.toBeInTheDocument();
	});
});

// The mobile pinning contract (spec 393): a list marks its name column with
// `isName`, and below the `lg` breakpoint the owner pins that column left
// and the trailing label-less action column right inside the table
// container's horizontal scroll. jsdom cannot prove real 360px layout —
// these tests pin the base styles that produce it; the `lg` overrides ride
// the same responsive props, and the visual fit is a manual browser check.
// Rendered through the harness with the app theme: responsive prop objects
// only resolve under a ChakraProvider.
describe("ResourceList mobile pinning", () => {
	// The lg half of the responsive props — the "desktop unchanged" line —
	// is pinned through the compiled emotion rules (breakpointRules over
	// cssRulesFor); Chakra's lg breakpoint (992px) matches the spec's
	// device ceiling.

	function renderPinnedRow(gadget: Gadget) {
		return (
			<Tr key={gadget.id} data-testid="gadget-row">
				<Td data-testid="gadget-name-cell">{gadget.name}</Td>
				<Td data-testid="gadget-size-cell">{gadget.id}</Td>
				<Td data-testid="gadget-actions-cell" />
			</Tr>
		);
	}

	function renderPinnedList(
		overrides: Partial<Parameters<typeof ResourceList<Gadget>>[0]> = {},
	) {
		return renderWithQuery(
			<ResourceList
				resource="gadget"
				isPending={false}
				error={undefined}
				items={gadgets}
				columns={[
					{
						label: "Name",
						isName: true,
						sortBy: (gadget: Gadget) => gadget.name,
					},
					{ label: "Size" },
					{},
				]}
				renderRow={renderPinnedRow}
				{...overrides}
			/>,
			{ chakra: true },
		);
	}

	it("pins the marked name column left and the trailing action column right at base", () => {
		renderPinnedList();

		const table = screen.getByTestId("gadgets-table");
		const [nameHeader, sizeHeader, actionsHeader] =
			within(table).getAllByRole("columnheader");
		expect(nameHeader).toHaveStyle("position: sticky");
		expect(nameHeader).toHaveStyle("left: 0px");
		expect(actionsHeader).toHaveStyle("position: sticky");
		expect(actionsHeader).toHaveStyle("right: 0px");
		expect(sizeHeader).not.toHaveStyle("position: sticky");

		const row = screen.getAllByTestId("gadget-row")[0];
		const [nameCell, sizeCell, actionsCell] = within(row).getAllByRole("cell");
		expect(nameCell).toHaveStyle("position: sticky");
		expect(nameCell).toHaveStyle("left: 0px");
		expect(actionsCell).toHaveStyle("position: sticky");
		expect(actionsCell).toHaveStyle("right: 0px");
		expect(sizeCell).not.toHaveStyle("position: sticky");
		// The pins are opaque — the page surface color — so the middle
		// columns scroll beneath them instead of showing through.
		expect(nameCell).toHaveStyle(
			"background-color: var(--chakra-colors-gray-50)",
		);
		expect(actionsCell).toHaveStyle(
			"background-color: var(--chakra-colors-gray-50)",
		);
	});

	it("returns the pins to desktop rendering at lg", () => {
		renderPinnedList();

		const table = screen.getByTestId("gadgets-table");
		const [nameHeader, sizeHeader, actionsHeader] =
			within(table).getAllByRole("columnheader");
		const row = screen.getAllByTestId("gadget-row")[0];
		const [nameCell, , actionsCell] = within(row).getAllByRole("cell");

		for (const pinned of [nameHeader, actionsHeader, nameCell, actionsCell]) {
			const { base, lg } = breakpointRules(pinned);
			expect(base).toContain("position:sticky");
			// lg and up: static, transparent, unshadowed — today's rendering.
			expect(lg).toContain("position:static");
			expect(lg).toContain("background-color:var(--chakra-colors-transparent)");
		}
		const middle = breakpointRules(sizeHeader);
		expect(middle.base).not.toContain("position:sticky");
		expect(middle.lg).not.toContain("position:static");
	});

	it("reaches cells inside a fragment row and never pins a spanning error cell", () => {
		renderPinnedList({
			renderRow: (gadget: Gadget) => (
				<Fragment key={gadget.id}>
					<Tr data-testid="gadget-row">
						<Td data-testid="gadget-name-cell">{gadget.name}</Td>
						<Td>{gadget.id}</Td>
						<Td />
					</Tr>
					<Tr data-testid="gadget-error-row">
						<Td colSpan={3} data-testid="gadget-error-cell">
							boom
						</Td>
					</Tr>
				</Fragment>
			),
		});

		const row = screen.getAllByTestId("gadget-row")[0];
		const [nameCell, , actionsCell] = within(row).getAllByRole("cell");
		expect(nameCell).toHaveStyle("position: sticky");
		expect(actionsCell).toHaveStyle("position: sticky");

		const errorCell = screen.getAllByTestId("gadget-error-cell")[0];
		expect(errorCell).not.toHaveStyle("position: sticky");
		expect(errorCell).not.toHaveStyle("left: 0px");
	});

	it("leaves every header and cell unpinned without the name marker", () => {
		renderPinnedList({
			columns: [
				{ label: "Name", sortBy: (gadget: Gadget) => gadget.name },
				{},
				{},
			],
		});

		const table = screen.getByTestId("gadgets-table");
		for (const header of within(table).getAllByRole("columnheader")) {
			expect(header).not.toHaveStyle("position: sticky");
		}
		const row = screen.getAllByTestId("gadget-row")[0];
		for (const cell of within(row).getAllByRole("cell")) {
			expect(cell).not.toHaveStyle("position: sticky");
			expect(cell).not.toHaveStyle(
				"background-color: var(--chakra-colors-gray-50)",
			);
		}
	});

	it("pins no right column when the last column is labeled", () => {
		renderPinnedList({
			columns: [
				{
					label: "Name",
					isName: true,
					sortBy: (gadget: Gadget) => gadget.name,
				},
				{ label: "Created" },
			],
			renderRow: (gadget: Gadget) => (
				<Tr key={gadget.id} data-testid="gadget-row">
					<Td data-testid="gadget-name-cell">{gadget.name}</Td>
					<Td data-testid="gadget-created-cell">yesterday</Td>
				</Tr>
			),
		});

		const nameCell = screen.getAllByTestId("gadget-name-cell")[0];
		const createdCell = screen.getAllByTestId("gadget-created-cell")[0];
		expect(nameCell).toHaveStyle("position: sticky");
		expect(nameCell).toHaveStyle("left: 0px");
		expect(createdCell).not.toHaveStyle("position: sticky");
		expect(createdCell).not.toHaveStyle("right: 0px");
	});

	it("wraps the toolbar wrapper at base instead of overflowing", () => {
		renderPinnedList({
			toolbar: (
				<Button size="sm" data-testid="gadgets-toolbar-action">
					Do a thing
				</Button>
			),
		});

		expect(screen.getByTestId("gadgets-toolbar-action")).toBeInTheDocument();
		const { base, lg } = breakpointRules(screen.getByTestId("gadgets-toolbar"));
		expect(base).toContain("flex-wrap:wrap");
		expect(lg).toContain("flex-wrap:nowrap");
	});

	it("keeps the sort contract on a pinned list", async () => {
		const user = userEvent.setup();
		renderPinnedList();

		const rowNames = () =>
			screen
				.getAllByTestId("gadget-name-cell")
				.map((cell) => cell.textContent ?? "");

		expect(rowNames()).toEqual(["spinner", "doodad"]);
		await user.click(screen.getByTestId("sort-gadget-0"));
		expect(rowNames()).toEqual(["doodad", "spinner"]);
	});
});

// The selection contract (spec 461, ticket 463): selection is opt-in through
// the `selection` prop, the owner keeps the selected-key set as
// component-local state (like the sort state), and the checkbox column is
// injected into the feature-rendered rows by the same cell-cloning machinery
// that owns mobile pinning — no feature row grows a <Td>. The adapter hands
// over the row's stable identity (`getKey`) and may react to changes
// (`onChange`) or open its bulk-remove dialog from the live selection
// (`onRemoveSelected`, the owner-rendered left toolbar button).
describe("ResourceList selection", () => {
	function renderSelectableList(
		overrides: Partial<Parameters<typeof ResourceList<Gadget>>[0]> = {},
	) {
		const onChange = vi.fn();
		const props = {
			resource: "gadget",
			isPending: false,
			error: undefined as unknown,
			items: gadgets as Gadget[] | undefined,
			columns: [{ label: "Name" }],
			renderRow: renderGadgetRow,
			// Selection is write-gated: the adapter must say so explicitly.
			canManage: true,
			selection: { getKey: (gadget: Gadget) => gadget.id, onChange },
			...overrides,
		};
		return { onChange, ...render(<ResourceList {...props} />) };
	}

	function rowCheckboxes(): HTMLElement[] {
		return screen
			.getAllByTestId("gadget-row")
			.map((row) => within(row).getByRole("checkbox"));
	}

	it("renders no checkboxes when the list does not ask for selection", () => {
		renderResourceList();

		expect(screen.queryAllByRole("checkbox")).toHaveLength(0);
	});

	it("renders a checkbox per row plus the select-all header when selection is on", () => {
		renderSelectableList();

		expect(rowCheckboxes()).toHaveLength(2);
		expect(
			screen.getByRole("checkbox", { name: "Select all gadgets" }),
		).toBeInTheDocument();
	});

	it("toggles one row and reports the selected keys", async () => {
		const user = userEvent.setup();
		const { onChange } = renderSelectableList();

		expect(onChange).not.toHaveBeenCalled();

		const [first] = rowCheckboxes();
		await user.click(first);
		expect(first).toBeChecked();
		expect(onChange).toHaveBeenLastCalledWith(["g-1"]);

		await user.click(first);
		expect(first).not.toBeChecked();
		expect(onChange).toHaveBeenLastCalledWith([]);
	});

	it("selects every row from the header, and clears when all are selected", async () => {
		const user = userEvent.setup();
		const { onChange } = renderSelectableList();

		const selectAll = screen.getByRole("checkbox", {
			name: "Select all gadgets",
		});
		await user.click(selectAll);
		expect(selectAll).toBeChecked();
		for (const checkbox of rowCheckboxes()) {
			expect(checkbox).toBeChecked();
		}
		expect(onChange).toHaveBeenLastCalledWith(["g-1", "g-2"]);

		await user.click(selectAll);
		expect(selectAll).not.toBeChecked();
		for (const checkbox of rowCheckboxes()) {
			expect(checkbox).not.toBeChecked();
		}
		expect(onChange).toHaveBeenLastCalledWith([]);
	});

	it("renders the owner's Remove selected button on the left of the toolbar row only while something is selected", async () => {
		const user = userEvent.setup();
		const onRemoveSelected = vi.fn();
		renderSelectableList({
			toolbar: (
				<Button size="sm" data-testid="gadgets-toolbar-action">
					Do a thing
				</Button>
			),
			onRemoveSelected,
		});

		// The bulk action exists only while something is selected: no
		// disabled button, ever.
		expect(
			screen.queryByTestId("remove-selected-gadgets-button"),
		).not.toBeInTheDocument();
		expect(screen.getByTestId("gadgets-toolbar-action")).toBeInTheDocument();

		await user.click(rowCheckboxes()[1]);

		const removeButton = screen.getByTestId("remove-selected-gadgets-button");
		// The left edge is the contract: the button leads the toolbar row,
		// before the adapter's own actions. It leads the selection group,
		// which is itself the toolbar row's first child.
		const toolbar = screen.getByTestId("gadgets-toolbar");
		const group = toolbar.firstElementChild as HTMLElement;
		expect(group).toContainElement(removeButton);
		expect(group.firstElementChild).toBe(removeButton);
		expect(screen.getByTestId("gadgets-toolbar-action")).toBeInTheDocument();

		await user.click(rowCheckboxes()[1]);
		expect(
			screen.queryByTestId("remove-selected-gadgets-button"),
		).not.toBeInTheDocument();
	});

	it("hands the live selection to onRemoveSelected, and clears through the clear it hands back", async () => {
		const user = userEvent.setup();
		const onRemoveSelected = vi.fn();
		const { onChange } = renderSelectableList({ onRemoveSelected });

		await user.click(rowCheckboxes()[0]);
		await user.click(rowCheckboxes()[1]);
		await user.click(screen.getByTestId("remove-selected-gadgets-button"));

		expect(onRemoveSelected).toHaveBeenCalledTimes(1);
		const selection = onRemoveSelected.mock.calls[0][0];
		expect(selection.selectedKeys).toEqual(["g-1", "g-2"]);
		// The projection is the selected rows themselves, in list order.
		expect(selection.selectedItems.map((item: Gadget) => item.name)).toEqual([
			"spinner",
			"doodad",
		]);

		act(() => selection.clear());
		expect(rowCheckboxes()[0]).not.toBeChecked();
		expect(onChange).toHaveBeenLastCalledWith([]);
	});

	it("renders no Remove selected button for a read-only caller", () => {
		renderSelectableList({ canManage: false, onRemoveSelected: vi.fn() });

		expect(
			screen.queryByTestId("remove-selected-gadgets-button"),
		).not.toBeInTheDocument();
		expect(screen.getByTestId("gadgets-permission-hint")).toBeInTheDocument();
	});

	it("hides the checkbox column from read-only users", () => {
		renderSelectableList({ canManage: false });

		expect(screen.queryAllByRole("checkbox")).toHaveLength(0);
		expect(screen.getByTestId("gadgets-permission-hint")).toBeInTheDocument();
	});

	// Selectability is write-gated, and an omitted `canManage` is not a
	// write grant: a list that asks for selection without saying who may
	// write renders no checkboxes rather than offering a read-only caller
	// an action that would 403.
	it("hides the checkbox column when canManage is omitted", () => {
		renderSelectableList({ canManage: undefined });

		expect(screen.queryAllByRole("checkbox")).toHaveLength(0);
	});

	it("resets the selection when the list remounts", async () => {
		const user = userEvent.setup();
		const { unmount } = renderSelectableList();

		await user.click(rowCheckboxes()[0]);
		expect(rowCheckboxes()[0]).toBeChecked();

		unmount();
		renderSelectableList();
		for (const checkbox of rowCheckboxes()) {
			expect(checkbox).not.toBeChecked();
		}
	});

	it("keeps the selection when the list is sorted", async () => {
		const user = userEvent.setup();
		const { onChange } = renderSelectableList({
			columns: [{ label: "Name", sortBy: (gadget: Gadget) => gadget.name }],
		});

		await user.click(screen.getByRole("checkbox", { name: "Select g-1" }));
		expect(onChange).toHaveBeenCalledTimes(1);

		await user.click(screen.getByTestId("sort-gadget-0"));

		// Server order was spinner, doodad; ascending puts doodad first.
		const [first, second] = screen.getAllByTestId("gadget-row");
		expect(within(first).getByText("doodad")).toBeInTheDocument();
		expect(within(first).getByRole("checkbox")).not.toBeChecked();
		expect(within(second).getByText("spinner")).toBeInTheDocument();
		expect(within(second).getByRole("checkbox")).toBeChecked();
		// Reordering is not a selection change.
		expect(onChange).toHaveBeenCalledTimes(1);
	});

	// The spanning row-error cells the container, pod, and quadlet rows
	// render must widen with the checkbox column, or the table is invalid.
	function renderErrorRow(gadget: Gadget) {
		return (
			<Fragment key={gadget.id}>
				<Tr data-testid="gadget-row">
					<Td>{gadget.name}</Td>
				</Tr>
				<Tr data-testid="gadget-error-row">
					<Td colSpan={1} data-testid="gadget-error-cell">
						boom
					</Td>
				</Tr>
			</Fragment>
		);
	}

	it("grows a spanning row-error cell by one column while selection is on", () => {
		renderSelectableList({ renderRow: renderErrorRow });

		expect(screen.getAllByTestId("gadget-error-cell")[0]).toHaveAttribute(
			"colspan",
			"2",
		);
		// A spanning row is an error row, not a data row: no checkbox.
		expect(
			within(screen.getAllByTestId("gadget-error-row")[0]).queryAllByRole(
				"checkbox",
			),
		).toHaveLength(0);
	});

	it("leaves the spanning cell's colSpan alone without selection", () => {
		renderResourceList({ renderRow: renderErrorRow });

		expect(screen.getAllByTestId("gadget-error-cell")[0]).toHaveAttribute(
			"colspan",
			"1",
		);
	});

	it("keeps the pins on the name and action columns and never pins the checkbox column", () => {
		renderWithQuery(
			<ResourceList
				resource="gadget"
				isPending={false}
				error={undefined}
				items={gadgets}
				columns={[
					{
						label: "Name",
						isName: true,
						sortBy: (gadget: Gadget) => gadget.name,
					},
					{ label: "Size" },
					{},
				]}
				renderRow={(gadget: Gadget) => (
					<Tr key={gadget.id} data-testid="gadget-row">
						<Td data-testid="gadget-name-cell">{gadget.name}</Td>
						<Td data-testid="gadget-size-cell">{gadget.id}</Td>
						<Td data-testid="gadget-actions-cell" />
					</Tr>
				)}
				selection={{ getKey: (gadget: Gadget) => gadget.id }}
				canManage
			/>,
			{ chakra: true },
		);

		const table = screen.getByTestId("gadgets-table");
		const [selectHeader, nameHeader, , actionsHeader] =
			within(table).getAllByRole("columnheader");
		expect(selectHeader).not.toHaveStyle("position: sticky");
		expect(nameHeader).toHaveStyle("position: sticky");
		expect(nameHeader).toHaveStyle("left: 0px");
		expect(actionsHeader).toHaveStyle("position: sticky");
		expect(actionsHeader).toHaveStyle("right: 0px");

		const row = screen.getAllByTestId("gadget-row")[0];
		const [selectCell, nameCell, , actionsCell] =
			within(row).getAllByRole("cell");
		expect(selectCell).not.toHaveStyle("position: sticky");
		expect(nameCell).toHaveStyle("position: sticky");
		expect(nameCell).toHaveStyle("left: 0px");
		expect(actionsCell).toHaveStyle("position: sticky");
		expect(actionsCell).toHaveStyle("right: 0px");
	});
});

// The selection-action seam (spec 486, ticket #487): a list whose selection
// drives more than one bulk action hands the owner a render function for its
// own toolbar buttons. The owner renders them in the selection group beside
// the Remove selected button — for a write-capable caller only, and always,
// never gated on a non-empty selection (the adapter's buttons disable
// themselves). `clearKeys` is the partial reset a batch run needs: only the
// items that succeeded leave the selection, so failures stay selected for
// the retry.
describe("ResourceList selection actions", () => {
	function renderActionList(
		overrides: Partial<Parameters<typeof ResourceList<Gadget>>[0]> = {},
	) {
		const onChange = vi.fn();
		const props = {
			resource: "gadget",
			isPending: false,
			error: undefined as unknown,
			items: gadgets as Gadget[] | undefined,
			columns: [{ label: "Name" }],
			renderRow: renderGadgetRow,
			canManage: true,
			selection: { getKey: (gadget: Gadget) => gadget.id, onChange },
			...overrides,
		};
		return { onChange, ...render(<ResourceList {...props} />) };
	}

	function rowCheckboxes(): HTMLElement[] {
		return screen
			.getAllByTestId("gadget-row")
			.map((row) => within(row).getByRole("checkbox"));
	}

	it("renders the adapter's selection actions permanently for a write-capable caller", async () => {
		const user = userEvent.setup();
		renderActionList({
			selectionActions: (selection) => (
				<Button
					size="sm"
					isDisabled={selection.selectedKeys.length === 0}
					data-testid="bulk-transfer-gadgets-button"
				>
					Move/copy selected
				</Button>
			),
		});

		// Always rendered — the adapter's button disables itself, so the
		// toolbar never grows or shrinks as the selection changes.
		const button = screen.getByTestId("bulk-transfer-gadgets-button");
		expect(button).toBeDisabled();

		await user.click(rowCheckboxes()[0]);
		expect(screen.getByTestId("bulk-transfer-gadgets-button")).toBeEnabled();

		await user.click(rowCheckboxes()[0]);
		expect(screen.getByTestId("bulk-transfer-gadgets-button")).toBeDisabled();
	});

	it("renders the selection actions after Remove selected in the left group", async () => {
		const user = userEvent.setup();
		renderActionList({
			onRemoveSelected: vi.fn(),
			selectionActions: () => (
				<Button size="sm" data-testid="bulk-transfer-gadgets-button">
					Move/copy selected
				</Button>
			),
		});

		await user.click(rowCheckboxes()[0]);

		const toolbar = screen.getByTestId("gadgets-toolbar");
		const group = toolbar.firstElementChild as HTMLElement;
		const removeButton = screen.getByTestId("remove-selected-gadgets-button");
		const transferButton = screen.getByTestId("bulk-transfer-gadgets-button");
		expect(group).toContainElement(removeButton);
		expect(group).toContainElement(transferButton);
		expect(group.firstElementChild).toBe(removeButton);
		expect(removeButton.nextElementSibling).toBe(transferButton);
	});

	it("renders no selection actions for a read-only caller", () => {
		renderActionList({
			canManage: false,
			selectionActions: () => (
				<Button size="sm" data-testid="bulk-transfer-gadgets-button">
					Move/copy selected
				</Button>
			),
		});

		expect(
			screen.queryByTestId("bulk-transfer-gadgets-button"),
		).not.toBeInTheDocument();
		expect(screen.getByTestId("gadgets-permission-hint")).toBeInTheDocument();
	});

	// The canSelect gate (spec 486, ticket #491): a list whose selection
	// action rides a lower capability than its toolbar's write grant
	// passes `canSelect` explicitly, and the whole selection seam renders
	// below the write grant — checkboxes and the adapter's own action for
	// a selection-capable caller — while the owner's Remove selected
	// button stays on the write grant (removal is a write act even on a
	// list whose connect action rides a lower capability) and the hint
	// stays away, because the caller is not below both capabilities.
	it("renders the selection seam below the write grant when canSelect is passed, keeping Remove selected on the write grant", async () => {
		const user = userEvent.setup();
		const onRemoveSelected = vi.fn();
		renderActionList({
			canManage: false,
			canSelect: true,
			onRemoveSelected,
			selectionActions: () => (
				<Button size="sm" data-testid="bulk-transfer-gadgets-button">
					Move/copy selected
				</Button>
			),
		});

		expect(rowCheckboxes()).toHaveLength(2);
		expect(
			screen.getByTestId("bulk-transfer-gadgets-button"),
		).toBeInTheDocument();
		expect(
			screen.queryByTestId("gadgets-permission-hint"),
		).not.toBeInTheDocument();

		// Remove selected exists only while something is selected and only
		// on the write grant: selecting a row must not conjure it.
		await user.click(rowCheckboxes()[0]);
		expect(
			screen.queryByTestId("remove-selected-gadgets-button"),
		).not.toBeInTheDocument();
		expect(onRemoveSelected).not.toHaveBeenCalled();
	});

	it("hands the live selection state to the selection actions", async () => {
		const user = userEvent.setup();
		const seen: string[][] = [];
		renderActionList({
			selectionActions: (selection) => {
				seen.push(selection.selectedKeys);
				return (
					<Button
						size="sm"
						onClick={() => selection.clearKeys([selection.selectedKeys[0]])}
						data-testid="bulk-transfer-gadgets-button"
					>
						Move/copy selected
					</Button>
				);
			},
		});

		await user.click(rowCheckboxes()[0]);
		await user.click(rowCheckboxes()[1]);
		expect(seen[seen.length - 1]).toEqual(["g-1", "g-2"]);
	});

	it("clearKeys drops only the named keys and fires onChange", async () => {
		const user = userEvent.setup();
		let live:
			| Parameters<
					NonNullable<
						Parameters<typeof ResourceList<Gadget>>[0]["selectionActions"]
					>
			  >[0]
			| null = null;
		const { onChange } = renderActionList({
			selectionActions: (selection) => {
				live = selection;
				return (
					<Button size="sm" data-testid="bulk-transfer-gadgets-button">
						Move/copy selected
					</Button>
				);
			},
		});

		await user.click(rowCheckboxes()[0]);
		await user.click(rowCheckboxes()[1]);
		expect(onChange).toHaveBeenLastCalledWith(["g-1", "g-2"]);

		act(() => live?.clearKeys(["g-1"]));

		// Only the named key left the selection: the failure stays selected
		// for the retry.
		expect(onChange).toHaveBeenLastCalledWith(["g-2"]);
		expect(rowCheckboxes()[0]).not.toBeChecked();
		expect(rowCheckboxes()[1]).toBeChecked();
	});
});
