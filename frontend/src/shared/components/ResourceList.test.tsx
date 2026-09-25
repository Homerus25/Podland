import { Button, Td, Tr } from "@chakra-ui/react";
import { ResourceList } from "@shared/components/ResourceList";
import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { axiosError } from "@/test/errors";

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

describe("ResourceList triad", () => {
	it("renders loading alone, with the toolbar still above it", () => {
		renderResourceList({ isPending: true, items: undefined });

		const loading = screen.getByTestId("gadgets-loading");
		expect(loading).toHaveTextContent("Loading gadgets…");
		expect(screen.getByTestId("gadgets-toolbar-action")).toBeInTheDocument();
		expect(screen.queryByTestId("gadgets-error")).not.toBeInTheDocument();
		expect(screen.queryByTestId("gadgets-empty")).not.toBeInTheDocument();
		expect(screen.queryByTestId("gadgets-table")).not.toBeInTheDocument();
	});

	it("renders the error envelope alone on the derived error testid", () => {
		renderResourceList({
			error: axiosError(404, "GADGET_DOWN", "Gadget exploded"),
		});

		expect(screen.getByTestId("gadgets-error")).toHaveRole("alert");
		expect(screen.getByText("GADGET_DOWN")).toBeInTheDocument();
		expect(screen.getByText("Gadget exploded")).toBeInTheDocument();
		expect(screen.getByTestId("gadgets-toolbar-action")).toBeInTheDocument();
		expect(screen.queryByTestId("gadgets-loading")).not.toBeInTheDocument();
		expect(screen.queryByTestId("gadgets-empty")).not.toBeInTheDocument();
		expect(screen.queryByTestId("gadgets-table")).not.toBeInTheDocument();
	});

	it("renders the empty state alone with the default copy", () => {
		renderResourceList({ items: [] });

		const empty = screen.getByTestId("gadgets-empty");
		expect(empty).toHaveTextContent("No gadgets on this machine yet.");
		expect(screen.getByTestId("gadgets-toolbar-action")).toBeInTheDocument();
		expect(screen.queryByTestId("gadgets-loading")).not.toBeInTheDocument();
		expect(screen.queryByTestId("gadgets-error")).not.toBeInTheDocument();
		expect(screen.queryByTestId("gadgets-table")).not.toBeInTheDocument();
	});

	it("renders the empty state with the emptyMessage override", () => {
		renderResourceList({
			items: [],
			emptyMessage: "No gadgets found. Pull some first.",
		});

		expect(screen.getByTestId("gadgets-empty")).toHaveTextContent(
			"No gadgets found. Pull some first.",
		);
	});

	it("renders the table alone with derived testids, headers, and feature rows", () => {
		renderResourceList();

		expect(screen.getByTestId("gadgets-table")).toBeInTheDocument();
		expect(screen.getAllByTestId("gadget-row")).toHaveLength(2);
		expect(screen.getByText("spinner")).toBeInTheDocument();
		expect(screen.getByText("doodad")).toBeInTheDocument();
		expect(screen.getByTestId("gadgets-toolbar-action")).toBeInTheDocument();
		expect(screen.queryByTestId("gadgets-loading")).not.toBeInTheDocument();
		expect(screen.queryByTestId("gadgets-error")).not.toBeInTheDocument();
		expect(screen.queryByTestId("gadgets-empty")).not.toBeInTheDocument();
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
