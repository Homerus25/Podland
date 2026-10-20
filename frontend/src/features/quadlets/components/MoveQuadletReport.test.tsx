import {
	MigrationNarration,
	MigrationProgress,
	MoveQuadletReport,
	type MoveQuadletReportSelection,
	SecretNarration,
	selectMoveQuadletReport,
} from "@features/quadlets/components/MoveQuadletReport";
import { screen, within } from "@testing-library/react";
import type { ComponentProps } from "react";
import { describe, expect, it } from "vitest";
import {
	quadletMoveResultFixture,
	quadletMoveSecretOutcomeFixture,
	quadletMoveVolumeOutcomeFixture,
} from "@/test/factories";
import { renderWithQuery } from "@/test/render";

// The module owns narration and progress as data-prop components plus the
// report precedence as an exported pure projection — no queries, no hooks,
// no list harness, no MSW.
type ReportProps = ComponentProps<typeof MoveQuadletReport>;

function renderReport(selection: MoveQuadletReportSelection) {
	const props: ReportProps = {
		selection,
		quadletName: "web",
		sourceName: "prod-web-01",
		targetName: "staging-01",
	};
	renderWithQuery(<MoveQuadletReport {...props} />, { chakra: true });
	return props;
}

const plainMoved = quadletMoveResultFixture({ status: "moved" });
const plainCopiedWithSecrets = quadletMoveResultFixture({
	status: "copied",
	secretTransfer: { secrets: [quadletMoveSecretOutcomeFixture()] },
});
const plainPartial = quadletMoveResultFixture({
	status: "partial",
	error: "failed to remove quadlet: resource running",
});
const streamMoved = quadletMoveResultFixture({
	status: "moved",
	volumeMigration: { volumes: [quadletMoveVolumeOutcomeFixture()] },
});
const streamPartial = quadletMoveResultFixture({
	status: "partial",
	error: "failed to remove quadlet: resource running",
});

describe("selectMoveQuadletReport", () => {
	it("selects nothing while neither transport has answered", () => {
		expect(selectMoveQuadletReport(undefined, null)).toBeNull();
	});

	it("selects nothing for a file-only plain completion", () => {
		expect(selectMoveQuadletReport(plainMoved, null)).toBeNull();
	});

	it("selects the plain transfer's secret outcomes when its answer carries them", () => {
		expect(selectMoveQuadletReport(plainCopiedWithSecrets, null)).toEqual({
			kind: "secret",
			result: plainCopiedWithSecrets,
		});
	});

	it("selects a partial result from either transport", () => {
		expect(selectMoveQuadletReport(plainPartial, null)).toEqual({
			kind: "partial",
			result: plainPartial,
		});
		expect(selectMoveQuadletReport(null, streamPartial)).toEqual({
			kind: "partial",
			result: streamPartial,
		});
	});

	it("lets a partial result outrank a completed migration", () => {
		expect(selectMoveQuadletReport(plainPartial, streamMoved)).toEqual({
			kind: "partial",
			result: streamMoved,
		});
	});

	it("lets a partial result outrank a plain transfer's secret outcomes", () => {
		expect(
			selectMoveQuadletReport(plainCopiedWithSecrets, streamPartial),
		).toEqual({ kind: "partial", result: streamPartial });
	});

	it("selects a completed migration from the stream", () => {
		expect(selectMoveQuadletReport(null, streamMoved)).toEqual({
			kind: "migration",
			result: streamMoved,
		});
		expect(
			selectMoveQuadletReport(
				null,
				quadletMoveResultFixture({ status: "copied" }),
			),
		).toEqual({
			kind: "migration",
			result: quadletMoveResultFixture({ status: "copied" }),
		});
	});

	it("lets a completed migration outrank a plain transfer's secret outcomes", () => {
		expect(
			selectMoveQuadletReport(plainCopiedWithSecrets, streamMoved),
		).toEqual({ kind: "migration", result: streamMoved });
	});
});

describe("MoveQuadletReport", () => {
	it("narrates a partial result, naming both machines and the error", () => {
		renderReport({
			kind: "partial",
			result: quadletMoveResultFixture({
				status: "partial",
				error: "failed to remove quadlet: resource running",
				volumeMigration: { volumes: [quadletMoveVolumeOutcomeFixture()] },
				secretTransfer: { secrets: [quadletMoveSecretOutcomeFixture()] },
			}),
		});

		const message = screen.getByTestId("move-partial-message");
		expect(message).toHaveTextContent(
			"The quadlet was installed on “staging-01” but could not be removed from “prod-web-01”. It currently exists on both machines and must be cleaned up manually.",
		);
		expect(message).toHaveTextContent(
			"failed to remove quadlet: resource running",
		);
		expect(
			within(message).getByTestId("move-migration-report"),
		).toBeInTheDocument();
		expect(
			within(message).getByTestId("move-secret-report"),
		).toBeInTheDocument();
	});

	it("omits the error line from a partial result without one", () => {
		renderReport({
			kind: "partial",
			result: quadletMoveResultFixture({ status: "partial" }),
		});

		const message = screen.getByTestId("move-partial-message");
		expect(message).toHaveTextContent("both machines");
		expect(
			within(message).queryByText("failed to remove quadlet: resource running"),
		).not.toBeInTheDocument();
	});

	it("narrates a completed migration with its volume and secret outcomes", () => {
		renderReport({
			kind: "migration",
			result: quadletMoveResultFixture({
				status: "moved",
				volumeMigration: { volumes: [quadletMoveVolumeOutcomeFixture()] },
				secretTransfer: { secrets: [quadletMoveSecretOutcomeFixture()] },
			}),
		});

		const report = screen.getByTestId("move-migration-success");
		expect(report).toHaveTextContent("Moved “web” to “staging-01”.");
		expect(
			within(report).getByTestId("move-migration-report"),
		).toBeInTheDocument();
		expect(
			within(report).getByTestId("move-secret-report"),
		).toBeInTheDocument();
	});

	it("narrates a copied migration alike", () => {
		renderReport({
			kind: "migration",
			result: quadletMoveResultFixture({ status: "copied" }),
		});

		expect(screen.getByTestId("move-migration-success")).toHaveTextContent(
			"Copied “web” to “staging-01”.",
		);
	});

	it("narrates a plain transfer's secret outcomes without the volume narration", () => {
		renderReport({ kind: "secret", result: plainCopiedWithSecrets });

		const report = screen.getByTestId("move-secret-success");
		expect(report).toHaveTextContent("Copied “web” to “staging-01”.");
		expect(
			within(report).getByTestId("move-secret-report"),
		).toBeInTheDocument();
		expect(
			within(report).queryByTestId("move-migration-report"),
		).not.toBeInTheDocument();
	});
});

describe("MigrationNarration", () => {
	it("renders nothing without volume outcomes", () => {
		renderWithQuery(
			<MigrationNarration result={quadletMoveResultFixture()} />,
			{ chakra: true },
		);
		expect(
			screen.queryByTestId("move-migration-report"),
		).not.toBeInTheDocument();
	});

	it("narrates migrated and skipped volumes with their reasons", () => {
		renderWithQuery(
			<MigrationNarration
				result={quadletMoveResultFixture({
					volumeMigration: {
						volumes: [
							quadletMoveVolumeOutcomeFixture(),
							quadletMoveVolumeOutcomeFixture({
								ref: "ghost",
								podmanVolume: "ghost",
								outcome: "skipped",
								reason: "source volume does not exist",
							}),
						],
					},
				})}
			/>,
			{ chakra: true },
		);

		const report = screen.getByTestId("move-migration-report");
		expect(report).toHaveTextContent("Volume data:");
		expect(
			screen.getByTestId("move-migration-outcome-pgdata"),
		).toHaveTextContent("data migrated");
		expect(
			screen.getByTestId("move-migration-outcome-ghost"),
		).toHaveTextContent("skipped — source volume does not exist");
	});
});

describe("SecretNarration", () => {
	it("renders nothing without secret outcomes", () => {
		renderWithQuery(<SecretNarration result={quadletMoveResultFixture()} />, {
			chakra: true,
		});
		expect(screen.queryByTestId("move-secret-report")).not.toBeInTheDocument();
	});

	it("narrates transferred and skipped secrets with their reasons", () => {
		renderWithQuery(
			<SecretNarration
				result={quadletMoveResultFixture({
					secretTransfer: {
						secrets: [
							quadletMoveSecretOutcomeFixture(),
							quadletMoveSecretOutcomeFixture({
								name: "ghost",
								outcome: "skipped",
								reason: "source secret does not exist",
							}),
						],
					},
				})}
			/>,
			{ chakra: true },
		);

		const report = screen.getByTestId("move-secret-report");
		expect(report).toHaveTextContent("Secrets:");
		expect(
			screen.getByTestId("move-secret-outcome-db-password"),
		).toHaveTextContent("value transferred");
		expect(screen.getByTestId("move-secret-outcome-ghost")).toHaveTextContent(
			"skipped — source secret does not exist",
		);
	});
});

describe("MigrationProgress", () => {
	it("renders one line per volume with its latest phase", () => {
		renderWithQuery(
			<MigrationProgress
				progress={[
					{
						ref: "pgdata",
						podmanVolume: "pgdata",
						phase: "bytes",
						bytes: 2097152,
					},
					{
						ref: "ghost",
						podmanVolume: "ghost",
						phase: "skip",
						reason: "source volume does not exist",
					},
				]}
			/>,
			{ chakra: true },
		);

		const progress = screen.getByTestId("move-migration-progress");
		expect(progress).toHaveTextContent("Migrating volume data…");
		expect(
			screen.getByTestId("move-migration-volume-pgdata"),
		).toHaveTextContent("pgdata");
		expect(
			screen.getByTestId("move-migration-volume-pgdata"),
		).toHaveTextContent("2.1 MB exported…");
		expect(screen.getByTestId("move-migration-volume-ghost")).toHaveTextContent(
			"skipped — source volume does not exist",
		);
	});

	it("labels the start and done phases", () => {
		renderWithQuery(
			<MigrationProgress
				progress={[
					{ ref: "a", podmanVolume: "a", phase: "start" },
					{ ref: "b", podmanVolume: "b", phase: "done", bytes: 2097152 },
				]}
			/>,
			{ chakra: true },
		);

		expect(screen.getByTestId("move-migration-volume-a")).toHaveTextContent(
			"exporting…",
		);
		expect(screen.getByTestId("move-migration-volume-b")).toHaveTextContent(
			"done (2.1 MB)",
		);
	});
});
