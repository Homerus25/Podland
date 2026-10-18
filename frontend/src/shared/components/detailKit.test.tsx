import { Table, Tbody } from "@chakra-ui/react";
import {
	DetailRow,
	isNotFound,
	KeyValueTable,
} from "@shared/components/detailKit";
import { screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { axiosError } from "@/test/errors";
import { renderWithQuery } from "@/test/render";

// The detail kit owns the primitives every resource detail page's overview
// composes: the 404 predicate the pages derive their not-found state from,
// the label/value row, and the key/value table. Each primitive's contract is
// pinned once here; the feature detail suites keep their domain assertions.

describe("isNotFound", () => {
	it("is true for an axios 404 error", () => {
		expect(
			isNotFound(axiosError(404, "CONTAINER_NOT_FOUND", "Container not found")),
		).toBe(true);
	});

	it("is false for other axios errors and for non-axios values", () => {
		expect(isNotFound(axiosError(403, "ACCESS_DENIED", "Access denied"))).toBe(
			false,
		);
		expect(isNotFound(new Error("boom"))).toBe(false);
		expect(isNotFound(null)).toBe(false);
		expect(isNotFound(undefined)).toBe(false);
	});
});

describe("DetailRow", () => {
	it("renders the label and the children in the row's two cells", () => {
		renderWithQuery(
			<Table size="sm">
				<Tbody>
					<DetailRow label="Name">
						<span data-testid="row-value">web</span>
					</DetailRow>
				</Tbody>
			</Table>,
			{ chakra: true },
		);

		const label = screen.getByText("Name");
		const value = screen.getByTestId("row-value");
		expect(label.closest("td")).not.toBeNull();
		expect(value.closest("td")).not.toBeNull();
		expect(label.closest("td")).not.toBe(value.closest("td"));
		expect(label.closest("tr")).toBe(value.closest("tr"));
	});
});

describe("KeyValueTable", () => {
	it("renders the entries as key/value rows under the testid", () => {
		renderWithQuery(
			<KeyValueTable
				testId="kit-labels"
				values={{ team: "platform", env: "prod" }}
			/>,
			{ chakra: true },
		);

		const table = screen.getByTestId("kit-labels");
		expect(table).toHaveTextContent("team");
		expect(table).toHaveTextContent("platform");
		expect(table).toHaveTextContent("env");
		expect(table).toHaveTextContent("prod");
	});

	it("renders None under the testid when the map is empty", () => {
		renderWithQuery(<KeyValueTable testId="kit-labels" values={{}} />, {
			chakra: true,
		});

		expect(screen.getByTestId("kit-labels")).toHaveTextContent("None");
	});

	it("renders None under the testid when the map is missing", () => {
		renderWithQuery(<KeyValueTable testId="kit-labels" values={undefined} />, {
			chakra: true,
		});

		expect(screen.getByTestId("kit-labels")).toHaveTextContent("None");
	});

	it("renders the optional label as a heading above the table", () => {
		renderWithQuery(
			<KeyValueTable
				label="Labels"
				testId="kit-labels"
				values={{ team: "platform" }}
			/>,
			{ chakra: true },
		);

		expect(screen.getByText("Labels")).toBeInTheDocument();
		expect(screen.getByTestId("kit-labels")).toHaveTextContent("team");
	});

	it("omits the heading when no label is given", () => {
		renderWithQuery(
			<KeyValueTable testId="kit-labels" values={{ team: "platform" }} />,
			{ chakra: true },
		);

		expect(screen.queryByText("Labels")).not.toBeInTheDocument();
		expect(screen.getByTestId("kit-labels")).toHaveTextContent("team");
	});
});
