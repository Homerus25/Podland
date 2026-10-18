import { Button, Td, Tr } from "@chakra-ui/react";
import { ResourceList } from "@shared/components/ResourceList";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";

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
