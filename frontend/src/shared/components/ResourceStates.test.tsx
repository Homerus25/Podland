import { ResourceStates } from "@shared/components/ResourceStates";
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { axiosError } from "@/test/errors";

function renderResourceStates(
	overrides: Partial<Parameters<typeof ResourceStates>[0]> = {},
) {
	const props = {
		resource: "gadget",
		isPending: false,
		error: undefined as unknown,
		items: [{ id: "g-1" }, { id: "g-2" }] as readonly unknown[] | undefined,
		children: <div data-testid="gadget-content">populated</div>,
		...overrides,
	};
	return render(<ResourceStates {...props} />);
}

describe("ResourceStates", () => {
	it("renders loading alone, winning over error and items", () => {
		renderResourceStates({
			isPending: true,
			error: axiosError(500, "GADGET_DOWN", "Gadget exploded"),
		});

		const loading = screen.getByTestId("gadgets-loading");
		expect(loading).toHaveTextContent("Loading gadgets…");
		expect(screen.queryByTestId("gadgets-error")).not.toBeInTheDocument();
		expect(screen.queryByTestId("gadgets-empty")).not.toBeInTheDocument();
		expect(screen.queryByTestId("gadget-content")).not.toBeInTheDocument();
	});

	it("renders the error envelope alone on the derived error testid, winning over items", () => {
		renderResourceStates({
			error: axiosError(404, "GADGET_DOWN", "Gadget exploded"),
		});

		expect(screen.getByTestId("gadgets-error")).toHaveRole("alert");
		expect(screen.getByText("GADGET_DOWN")).toBeInTheDocument();
		expect(screen.getByText("Gadget exploded")).toBeInTheDocument();
		expect(screen.queryByTestId("gadgets-loading")).not.toBeInTheDocument();
		expect(screen.queryByTestId("gadgets-empty")).not.toBeInTheDocument();
		expect(screen.queryByTestId("gadget-content")).not.toBeInTheDocument();
	});

	it("renders the default empty copy for an empty items array", () => {
		renderResourceStates({ items: [] });

		const empty = screen.getByTestId("gadgets-empty");
		expect(empty).toHaveTextContent("No gadgets on this host yet.");
		expect(screen.queryByTestId("gadget-content")).not.toBeInTheDocument();
	});

	it("treats undefined items as empty", () => {
		renderResourceStates({ items: undefined });

		expect(screen.getByTestId("gadgets-empty")).toBeInTheDocument();
		expect(screen.queryByTestId("gadget-content")).not.toBeInTheDocument();
	});

	it("renders the emptyMessage override in place of the default copy", () => {
		renderResourceStates({
			items: [],
			emptyMessage: "No gadgets found. Pull some first.",
		});

		expect(screen.getByTestId("gadgets-empty")).toHaveTextContent(
			"No gadgets found. Pull some first.",
		);
	});

	it("renders children only in the populated state", () => {
		renderResourceStates();

		expect(screen.getByTestId("gadget-content")).toBeInTheDocument();
		expect(screen.queryByTestId("gadgets-loading")).not.toBeInTheDocument();
		expect(screen.queryByTestId("gadgets-error")).not.toBeInTheDocument();
		expect(screen.queryByTestId("gadgets-empty")).not.toBeInTheDocument();
	});

	it("derives the plural testids from the resource noun", () => {
		renderResourceStates({ resource: "widget", items: [] });

		expect(screen.getByTestId("widgets-empty")).toBeInTheDocument();
		expect(screen.queryByTestId("gadgets-empty")).not.toBeInTheDocument();
	});
});
