import {
	TransferReport,
	transferProgressLabel,
} from "@shared/transfers/transferPresentation";
import { screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { renderWithQuery } from "@/test/render";

// The presentation module's suite pins the two rituals from props — no
// queries, no hooks, no list harness, no MSW.
describe("transferProgressLabel", () => {
	it("labels the start phase", () => {
		expect(transferProgressLabel({ phase: "start" })).toBe("exporting…");
	});

	it("labels the bytes phase with the formatted byte count", () => {
		expect(transferProgressLabel({ phase: "bytes", bytes: 2097152 })).toBe(
			"2.1 MB exported…",
		);
	});

	it("labels the bytes phase without a count as zero", () => {
		expect(transferProgressLabel({ phase: "bytes" })).toBe("0 B exported…");
	});

	it("labels the done phase with the formatted byte count", () => {
		expect(transferProgressLabel({ phase: "done", bytes: 2097152 })).toBe(
			"done (2.1 MB)",
		);
	});

	it("labels the done phase without a count as zero", () => {
		expect(transferProgressLabel({ phase: "done" })).toBe("done (0 B)");
	});

	it("labels a skip with its reason", () => {
		expect(
			transferProgressLabel({
				phase: "skip",
				reason: "source volume does not exist",
			}),
		).toBe("skipped — source volume does not exist");
	});

	it("labels a skip without a reason as plain skipped — no dangling dash", () => {
		expect(transferProgressLabel({ phase: "skip" })).toBe("skipped");
	});
});

describe("TransferReport", () => {
	it("renders the moved header under the default success testid", () => {
		renderWithQuery(
			<TransferReport
				header={{
					status: "moved",
					itemName: "app-data",
					targetName: "staging-01",
				}}
			/>,
			{ chakra: true },
		);

		expect(screen.getByTestId("move-success-report")).toHaveTextContent(
			"Moved “app-data” to “staging-01”.",
		);
	});

	it("renders the copied header", () => {
		renderWithQuery(
			<TransferReport
				header={{
					status: "copied",
					itemName: "db-password",
					targetName: "staging-01",
				}}
			/>,
			{ chakra: true },
		);

		expect(screen.getByTestId("move-success-report")).toHaveTextContent(
			"Copied “db-password” to “staging-01”.",
		);
	});

	it("carries the byte-count suffix when the header has bytes", () => {
		renderWithQuery(
			<TransferReport
				header={{
					status: "moved",
					itemName: "app-data",
					targetName: "staging-01",
					bytes: 2097152,
				}}
			/>,
			{ chakra: true },
		);

		expect(screen.getByTestId("move-success-report")).toHaveTextContent(
			"Moved “app-data” to “staging-01” (2.1 MB of data).",
		);
	});

	it("nests the partial block under the header with the pinned both-hosts tail and the error line", () => {
		renderWithQuery(
			<TransferReport
				header={{
					status: "partial",
					itemName: "app-data",
					targetName: "staging-01",
					bytes: 2097152,
				}}
				partial={{
					cause: "The data crossed but the source volume could not be removed.",
					error: "failed to remove volume: volume is in use",
				}}
			/>,
			{ chakra: true },
		);

		const report = screen.getByTestId("move-success-report");
		expect(report).toHaveTextContent(
			"Copied “app-data” to “staging-01” (2.1 MB of data).",
		);
		const partial = within(report).getByTestId("move-partial-message");
		expect(partial).toHaveTextContent(
			"The data crossed but the source volume could not be removed. It currently exists on both hosts and must be cleaned up manually.",
		);
		expect(partial).toHaveTextContent(
			"failed to remove volume: volume is in use",
		);
		// The cause line and the error line — two paragraphs, no more.
		expect(partial.querySelectorAll("p")).toHaveLength(2);
	});

	it("omits the error line from a partial block without one", () => {
		renderWithQuery(
			<TransferReport
				header={{
					status: "partial",
					itemName: "db-password",
					targetName: "staging-01",
				}}
				partial={{
					cause:
						"The value crossed but the source secret could not be removed.",
				}}
			/>,
			{ chakra: true },
		);

		const partial = screen.getByTestId("move-partial-message");
		expect(partial).toHaveTextContent(
			"The value crossed but the source secret could not be removed. It currently exists on both hosts and must be cleaned up manually.",
		);
		// The cause line alone — no error paragraph.
		expect(partial.querySelectorAll("p")).toHaveLength(1);
	});

	it("renders the children slot inside the report", () => {
		renderWithQuery(
			<TransferReport
				header={{
					status: "moved",
					itemName: "web",
					targetName: "staging-01",
				}}
			>
				<div data-testid="domain-extra">Volume data:</div>
			</TransferReport>,
			{ chakra: true },
		);

		const report = screen.getByTestId("move-success-report");
		expect(within(report).getByTestId("domain-extra")).toHaveTextContent(
			"Volume data:",
		);
	});

	it("passes the adapter's own outer testid through", () => {
		renderWithQuery(
			<TransferReport
				testid="move-migration-success"
				header={{
					status: "moved",
					itemName: "web",
					targetName: "staging-01",
				}}
			/>,
			{ chakra: true },
		);

		expect(screen.getByTestId("move-migration-success")).toHaveTextContent(
			"Moved “web” to “staging-01”.",
		);
		expect(screen.queryByTestId("move-success-report")).not.toBeInTheDocument();
	});

	it("passes the partial block's testid through when it nests", () => {
		renderWithQuery(
			<TransferReport
				header={{
					status: "partial",
					itemName: "app-data",
					targetName: "staging-01",
				}}
				partial={{
					cause: "The data crossed but the source volume could not be removed.",
				}}
				partialTestid="move-partial-block"
			/>,
			{ chakra: true },
		);

		expect(screen.getByTestId("move-partial-block")).toHaveTextContent(
			"both hosts",
		);
		expect(
			screen.queryByTestId("move-partial-message"),
		).not.toBeInTheDocument();
	});

	it("carries a headerless partial report's content directly under the outer testid", () => {
		renderWithQuery(
			<TransferReport
				testid="move-partial-message"
				spacing={3}
				partial={{
					cause:
						"The quadlet was installed on “staging-01” but could not be removed from “prod-web-01”.",
					error: "failed to remove quadlet: resource running",
				}}
			>
				<div data-testid="domain-extra" />
			</TransferReport>,
			{ chakra: true },
		);

		// One element owns the partial testid — the content is not nested
		// under a second partial wrapper.
		const message = screen.getByTestId("move-partial-message");
		expect(message).toHaveTextContent(
			"The quadlet was installed on “staging-01” but could not be removed from “prod-web-01”. It currently exists on both hosts and must be cleaned up manually.",
		);
		expect(message).toHaveTextContent(
			"failed to remove quadlet: resource running",
		);
		expect(within(message).getByTestId("domain-extra")).toBeInTheDocument();
		expect(screen.queryByTestId("move-success-report")).not.toBeInTheDocument();
	});
});
