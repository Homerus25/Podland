import { VolumeList } from "@features/volumes/components/VolumeList";
import type {
	VolumeMovePreview,
	VolumeMoveResult,
	VolumeSummary,
} from "@features/volumes/services/volumesApi";
import type { Host } from "@shared/types/api";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http, type JsonBodyType } from "msw";
import { beforeEach, describe, expect, it } from "vitest";
import {
	hostFixture,
	volumeFixture,
	volumeMovePreviewFixture,
	volumeMoveResultFixture,
} from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";
import { ndjsonResponse } from "@/test/stream";

// The Volumes "Move/copy selected" adapter (spec 486, ticket #490) at the
// MSW/HTTP seam. The batch contract itself — one target and mode for the
// whole selection, per-item previews and warnings, sequential execution, the
// per-item report, the succeeded keys — is pinned once in the shared
// MultiTransferDialog suite. This suite keeps adapter smokes only: the
// toolbar button's presence and gate, the row button's removal, the dialog
// opening with the selected volumes, the volume-domain warnings, and the
// selection clearing only the successes.

const alpha = volumeFixture({ name: "app-data" });
const beta = volumeFixture({ name: "logs-data" });

const source = hostFixture();
const staging = hostFixture({ id: "m-2", name: "staging-01" });
const backup = hostFixture({ id: "m-4", name: "backup-01" });
const readOnly = hostFixture({
	id: "m-3",
	name: "laptop-01",
	permission: "read",
	isOwner: false,
});

let moveRequests: { name: string; body: Record<string, unknown> }[] = [];
let previewRequests: string[] = [];

function resetCounters() {
	moveRequests = [];
	previewRequests = [];
}

function hostsHandler(hosts: Host[]) {
	return http.get("*/api/hosts", () => HttpResponse.json(hosts));
}

function volumesHandler(volumes: () => VolumeSummary[]) {
	return http.get("*/api/hosts/m-1/volumes", () =>
		HttpResponse.json(volumes()),
	);
}

function previewHandler(overrides: Partial<VolumeMovePreview> = {}) {
	return http.get(
		"*/api/hosts/m-1/volumes/:volumeName/move-preview",
		({ params }) => {
			previewRequests.push(String(params.volumeName));
			return HttpResponse.json(volumeMovePreviewFixture(overrides));
		},
	);
}

// The per-item transfer endpoint: the existing single-item route, unchanged,
// answering NDJSON progress events with one terminal event.
function moveHandler(
	result: (name: string) => VolumeMoveResult | HttpResponse<JsonBodyType>,
) {
	return http.post(
		"*/api/hosts/m-1/volumes/:volumeName/move",
		async ({ request, params }) => {
			const name = String(params.volumeName);
			moveRequests.push({
				name,
				body: (await request.json()) as Record<string, unknown>,
			});
			const answer = result(name);
			if (answer instanceof HttpResponse) return answer;
			return ndjsonResponse(
				[],
				[
					`${JSON.stringify({
						operationId: "op-1",
						type: "complete",
						hostId: "m-1",
						result: answer,
					})}\n`,
				],
			);
		},
	);
}

function renderVolumeList(permission: "read" | "write" | "admin" = "admin") {
	return renderWithQuery(
		<VolumeList
			host={hostFixture({ permission, isOwner: permission === "admin" })}
		/>,
		// The row navigates on click, so the list renders inside a router.
		{ chakra: true, router: true },
	);
}

async function selectVolumes(...names: string[]) {
	const user = userEvent.setup();
	for (const name of names) {
		await user.click(screen.getByRole("checkbox", { name: `Select ${name}` }));
	}
	return user;
}

describe("VolumeList move/copy selected", () => {
	beforeEach(resetCounters);

	it("renders no row Move/Copy button any more, keeping Export and Populate", async () => {
		server.use(
			volumesHandler(() => [alpha, beta]),
			hostsHandler([source, staging, readOnly, backup]),
		);
		renderVolumeList();

		const rows = await screen.findAllByTestId("volume-row");
		expect(rows).toHaveLength(2);
		for (const row of rows) {
			expect(
				within(row).queryByRole("button", { name: /move or copy/i }),
			).not.toBeInTheDocument();
		}
		// The row keeps its surviving actions.
		expect(
			within(rows[0]).getByRole("button", { name: `Export ${alpha.name}` }),
		).toBeInTheDocument();
		expect(
			within(rows[0]).getByRole("button", { name: `Populate ${alpha.name}` }),
		).toBeInTheDocument();
	});

	it("renders Move/copy selected permanently for a manage-volumes caller, disabled on an empty selection", async () => {
		server.use(
			volumesHandler(() => [alpha, beta]),
			hostsHandler([source, staging, readOnly, backup]),
		);
		renderVolumeList();
		await screen.findByText(alpha.name);

		// Permanently rendered — the button disables itself rather than
		// appearing and disappearing with the selection.
		const button = screen.getByTestId("move-selected-volumes-button");
		expect(button).toBeDisabled();

		await selectVolumes(alpha.name);
		expect(screen.getByTestId("move-selected-volumes-button")).toBeEnabled();
	});

	it("is absent below manage-volumes, where the permission hint replaces the toolbar", async () => {
		server.use(
			volumesHandler(() => [alpha, beta]),
			hostsHandler([source, staging, readOnly, backup]),
		);
		renderVolumeList("read");
		await screen.findByText(alpha.name);

		expect(
			screen.queryByTestId("move-selected-volumes-button"),
		).not.toBeInTheDocument();
		// The read-only swap is the module's, unchanged: no toolbar at all.
		expect(
			screen.queryByTestId("create-volume-button"),
		).not.toBeInTheDocument();
	});

	it("opens one dialog naming the selected volumes, sending nothing yet", async () => {
		server.use(
			volumesHandler(() => [alpha, beta]),
			hostsHandler([source, staging, readOnly, backup]),
			previewHandler(),
			moveHandler((name) => volumeMoveResultFixture({ name })),
		);
		renderVolumeList();
		await screen.findByText(alpha.name);
		const user = await selectVolumes(alpha.name, beta.name);

		await user.click(screen.getByTestId("move-selected-volumes-button"));

		const modal = await screen.findByTestId("modal");
		expect(modal).toHaveTextContent("Move or copy 2 volumes");
		const blocks = screen.getAllByTestId("multi-transfer-item");
		expect(blocks).toHaveLength(2);
		expect(within(blocks[0]).getByText(alpha.name)).toBeInTheDocument();
		expect(within(blocks[1]).getByText(beta.name)).toBeInTheDocument();
		// One target and one mode for the whole batch.
		expect(screen.getAllByTestId("move-target-select")).toHaveLength(1);
		expect(screen.getAllByTestId("move-mode-radio")).toHaveLength(1);
		expect(moveRequests).toHaveLength(0);
	});

	it("clears only the successes from the selection after a run", async () => {
		server.use(
			volumesHandler(() => [alpha, beta]),
			hostsHandler([source, staging, readOnly, backup]),
			previewHandler(),
			moveHandler((name) =>
				name === beta.name
					? ndjsonResponse(
							[],
							[
								`${JSON.stringify({
									operationId: "op-1",
									type: "error",
									hostId: "m-1",
									error: {
										code: "VOLUME_ALREADY_EXISTS",
										message:
											"Volume logs-data already exists on the target host",
									},
								})}\n`,
							],
						)
					: volumeMoveResultFixture({ name }),
			),
		);
		renderVolumeList();
		await screen.findByText(alpha.name);
		const user = await selectVolumes(alpha.name, beta.name);

		await user.click(screen.getByTestId("move-selected-volumes-button"));
		await screen.findByTestId("modal");
		await waitFor(() =>
			expect(screen.getByTestId("confirm-multi-transfer-button")).toBeEnabled(),
		);

		await user.click(screen.getByTestId("confirm-multi-transfer-button"));

		// The report is the run's completion signal; what it says is the
		// shared dialog's contract, pinned in its own suite.
		await screen.findByTestId("multi-transfer-report");

		// The succeeded volume leaves the selection; the failed one stays
		// selected for the retry.
		await user.click(screen.getByTestId("close-multi-transfer-report-button"));
		await waitFor(() =>
			expect(screen.queryByTestId("modal")).not.toBeInTheDocument(),
		);
		expect(
			screen.getByRole("checkbox", { name: `Select ${alpha.name}` }),
		).not.toBeChecked();
		expect(
			screen.getByRole("checkbox", { name: `Select ${beta.name}` }),
		).toBeChecked();
	});

	it("renders each volume's referencing-quadlet warning under its own name, with the dangling note in move mode only", async () => {
		server.use(
			volumesHandler(() => [alpha, beta]),
			hostsHandler([source, staging, readOnly, backup]),
			previewHandler({
				referencingQuadlets: [
					{ name: "web.container" },
					{ name: "db.container" },
				],
			}),
		);
		renderVolumeList();
		await screen.findByText(alpha.name);
		const user = await selectVolumes(alpha.name, beta.name);

		await user.click(screen.getByTestId("move-selected-volumes-button"));
		await screen.findByTestId("modal");

		const blocks = screen.getAllByTestId("multi-transfer-item");
		for (const block of blocks) {
			const warning = within(block).getByTestId("move-referencing-warning");
			expect(warning).toHaveTextContent("web.container");
			expect(warning).toHaveTextContent("db.container");
			expect(warning).toHaveTextContent("keep pointing at prod-web-01");
			expect(warning).toHaveTextContent("references dangle");
		}

		// Copy keeps the source volume, so nothing dangles.
		await user.click(
			screen.getByRole("radio", { name: "Copy (keep this host's volume)" }),
		);
		for (const block of screen.getAllByTestId("multi-transfer-item")) {
			expect(
				within(block).getByTestId("move-referencing-warning"),
			).not.toHaveTextContent("references dangle");
		}
	});

	it("notes an in-use volume in move mode without offering the force checkbox", async () => {
		server.use(
			volumesHandler(() => [alpha]),
			hostsHandler([source, staging, readOnly, backup]),
			previewHandler({ inUse: true }),
		);
		renderVolumeList();
		await screen.findByText(alpha.name);
		const user = await selectVolumes(alpha.name);

		await user.click(screen.getByTestId("move-selected-volumes-button"));
		await screen.findByTestId("modal");

		const block = screen.getByTestId("multi-transfer-item");
		expect(within(block).getByTestId("move-inuse-warning")).toHaveTextContent(
			"container on this host",
		);
		// The batch sends no force: the note names the way out instead of
		// offering a checkbox the run would ignore.
		expect(screen.queryByTestId("move-force-option")).not.toBeInTheDocument();

		// Copy removes nothing, so the note goes with the mode.
		await user.click(
			screen.getByRole("radio", { name: "Copy (keep this host's volume)" }),
		);
		expect(screen.queryByTestId("move-inuse-warning")).not.toBeInTheDocument();
	});
});
