import { Button, Td, Tr } from "@chakra-ui/react";
import { ResourceList } from "@shared/components/ResourceList";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Fragment } from "react";
import { describe, expect, it } from "vitest";
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
