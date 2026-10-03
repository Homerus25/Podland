import { QuadletList } from "@features/quadlets/components/QuadletList";
import type {
	QuadletMovePreview,
	QuadletSummary,
} from "@features/quadlets/services/quadletsApi";
import type { Machine } from "@shared/types/api";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http, type JsonBodyType } from "msw";
import { beforeEach, describe, expect, it } from "vitest";
import { errorJson } from "@/test/errors";
import {
	machineFixture,
	quadletFixture,
	quadletMoveCompanionVolumeFixture,
	quadletMoveDataVolumeFixture,
	quadletMovePreviewFixture,
	quadletMovePreviewSecretFixture,
	quadletMoveResultFixture,
	quadletMoveSecretOutcomeFixture,
} from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";
import { liveNdjsonResponse, ndjsonResponse } from "@/test/stream";

const web = quadletFixture();

const hostPathVolume = {
	source: "/srv/data",
	kind: "hostPath" as const,
	existsOnTarget: false,
	migratable: false,
	reason: "host bind paths are not migrated",
};

// The four secret shapes the dialog must render honestly (issue 222): a
// transferable secret new on the target, one already on the target, a
// dangling reference missing on the source too, and a host-path source that
// no store ever holds.
const secretPreview = {
	secrets: [
		quadletMovePreviewSecretFixture(),
		quadletMovePreviewSecretFixture({ name: "tls-cert", existsOnTarget: true }),
		quadletMovePreviewSecretFixture({ name: "ghost", existsOnSource: false }),
		quadletMovePreviewSecretFixture({
			name: "/etc/secrets/api.key",
			kind: "hostPath" as const,
			existsOnSource: false,
			existsOnTarget: false,
		}),
	],
};

const source = machineFixture();
const staging = machineFixture({ id: "m-2", name: "staging-01" });
const backup = machineFixture({ id: "m-4", name: "backup-01" });
const readOnly = machineFixture({
	id: "m-3",
	name: "laptop-01",
	permission: "read",
	isOwner: false,
});

let moveRequests: { count: number; body: Record<string, unknown> } = {
	count: 0,
	body: {},
};
let previewTargets: string[] = [];
let quadletFetches = 0;

function resetCounters() {
	moveRequests = { count: 0, body: {} };
	previewTargets = [];
	quadletFetches = 0;
}

function quadletsHandler(quadlets: () => QuadletSummary[]) {
	return http.get("*/api/machines/m-1/quadlets", () =>
		HttpResponse.json(quadlets()),
	);
}

// The quadlets list with a fetch counter, for suites that assert the
// both-machines invalidation as a real refetch (issue 214's stream path).
function countingQuadletsHandler(quadlets: () => QuadletSummary[]) {
	return http.get("*/api/machines/m-1/quadlets", () => {
		quadletFetches++;
		return HttpResponse.json(quadlets());
	});
}

function machinesHandler(machines: Machine[]) {
	return http.get("*/api/machines", () => HttpResponse.json(machines));
}

function previewHandler(overrides: Partial<QuadletMovePreview> = {}) {
	return http.get(
		"*/api/machines/m-1/quadlets/web/move-preview",
		({ request }) => {
			previewTargets.push(
				new URL(request.url).searchParams.get("targetMachineId") ?? "",
			);
			return HttpResponse.json(quadletMovePreviewFixture(overrides));
		},
	);
}

function moveHandler(
	status = 200,
	body: JsonBodyType = quadletMoveResultFixture(),
	onMoved?: () => void,
) {
	return http.post(
		"*/api/machines/m-1/quadlets/web/move",
		async ({ request }) => {
			moveRequests.count++;
			moveRequests.body = (await request.json()) as Record<string, unknown>;
			if (status !== 200) return HttpResponse.json(body, { status });
			onMoved?.();
			return HttpResponse.json(body);
		},
	);
}

// The migrating transport (issue 214): the same endpoint answered as NDJSON
// events. The events travel as raw framed lines — they are the move domain's
// own event vocabulary, not the shared stream kit's.
function moveStreamHandler(events: Record<string, unknown>[], live = false) {
	const chunks = events.map((event) => `${JSON.stringify(event)}\n`);
	return http.post(
		"*/api/machines/m-1/quadlets/web/move",
		async ({ request }) => {
			moveRequests.count++;
			moveRequests.body = (await request.json()) as Record<string, unknown>;
			return live ? liveNdjsonResponse(chunks, 50) : ndjsonResponse([], chunks);
		},
	);
}

// The shared wiring every dialog test needs: the source list, the machines
// directory (the target picker's source), the preview, and the transfer.
function useMoveHandlers() {
	server.use(
		quadletsHandler(() => [web]),
		machinesHandler([source, staging, readOnly, backup]),
		previewHandler(),
		moveHandler(),
	);
}

async function openMoveDialog() {
	renderWithQuery(<QuadletList machine={source} />, { router: true });
	const user = userEvent.setup();
	await user.click(
		await screen.findByRole("button", { name: "Move or copy web" }),
	);
	expect(await screen.findByTestId("modal")).toBeInTheDocument();
	return user;
}

async function openMoveDialogWithPreview() {
	const user = await openMoveDialog();
	// The default target (first writable machine) previews clean with volumes;
	// the dialog needs the resolved warning before it can be driven further.
	await screen.findByTestId("move-volume-warning");
	return user;
}

// Ready-to-confirm state: preview resolved AND the stranded-data warning
// acknowledged (with volumes present, confirm stays disabled without it).
async function openMoveDialogReadyToConfirm() {
	const user = await openMoveDialogWithPreview();
	await user.click(
		screen.getByRole("checkbox", {
			name: "I understand this data stays behind",
		}),
	);
	await waitFor(() =>
		expect(screen.getByTestId("confirm-move-button")).toBeEnabled(),
	);
	return user;
}

describe("QuadletList move/copy", () => {
	beforeEach(resetCounters);

	it("hides the Move/Copy action below the manage-quadlets capability", async () => {
		server.use(quadletsHandler(() => [web]));
		renderWithQuery(
			<QuadletList
				machine={machineFixture({ permission: "read", isOwner: false })}
			/>,
			{ router: true },
		);

		expect(await screen.findByTestId("quadlet-row")).toBeInTheDocument();
		expect(
			screen.queryByRole("button", { name: "Move or copy web" }),
		).not.toBeInTheDocument();
		// The header drops the move/copy column with the rows, so the read-only
		// table stays aligned.
		expect(screen.getAllByRole("columnheader")).toHaveLength(6);
	});

	it("offers one dialog listing only writable machines, source excluded", async () => {
		useMoveHandlers();
		await openMoveDialog();

		const select = screen.getByTestId("move-target-select");
		const names = within(select)
			.getAllByRole("option")
			.map((option) => option.textContent);
		expect(names).toEqual(["staging-01", "backup-01"]);
		expect(names).not.toContain("prod-web-01");
		expect(names).not.toContain("laptop-01");
	});

	it("refetches the preview when the target changes", async () => {
		useMoveHandlers();
		const user = await openMoveDialogReadyToConfirm();

		await user.selectOptions(screen.getByTestId("move-target-select"), "m-4");

		await waitFor(() => expect(previewTargets).toEqual(["m-2", "m-4"]));
	});

	it("resets the volume acknowledgement when the target changes (issue 207)", async () => {
		useMoveHandlers();
		const user = await openMoveDialogReadyToConfirm();

		await user.selectOptions(screen.getByTestId("move-target-select"), "m-4");
		await waitFor(() => expect(previewTargets).toEqual(["m-2", "m-4"]));

		// The acknowledgment answered m-2's preview; the fresh target's
		// volume facts demand a fresh look before confirm unlocks again.
		expect(screen.getByTestId("confirm-move-button")).toBeDisabled();
		await user.click(
			screen.getByRole("checkbox", {
				name: "I understand this data stays behind",
			}),
		);
		expect(screen.getByTestId("confirm-move-button")).toBeEnabled();
	});

	it("warns move mode: lists volumes with kind labels and demands explicit confirmation", async () => {
		useMoveHandlers();
		const user = await openMoveDialogWithPreview();

		const warning = screen.getByTestId("move-volume-warning");
		expect(warning).toHaveTextContent("pgdata");
		expect(warning).toHaveTextContent("podman volume");
		expect(warning).toHaveTextContent("/srv/data");
		expect(warning).toHaveTextContent("host path");

		// The preview has resolved and the warning is shown, but the transfer
		// still waits for the explicit acknowledgment of the stranded data.
		expect(screen.getByTestId("confirm-move-button")).toBeDisabled();
		await user.click(
			screen.getByRole("checkbox", {
				name: "I understand this data stays behind",
			}),
		);
		expect(screen.getByTestId("confirm-move-button")).toBeEnabled();

		// Delete-modal defaults: stop on, force off.
		expect(
			screen.getByRole("checkbox", { name: "Stop the unit before moving" }),
		).toBeChecked();
		expect(screen.getByRole("checkbox", { name: "Force" })).not.toBeChecked();

		await user.click(screen.getByTestId("confirm-move-button"));

		await waitFor(() => expect(moveRequests.count).toBe(1));
		expect(moveRequests.body).toEqual({
			targetMachineId: "m-2",
			mode: "move",
			stop: true,
			force: false,
		});
	});

	it("warns copy mode alike, without the move-only stop/force options", async () => {
		useMoveHandlers();
		const user = await openMoveDialogWithPreview();

		await user.click(
			screen.getByRole("radio", { name: "Copy (keep this machine's quadlet)" }),
		);

		expect(screen.queryByTestId("move-stop-option")).not.toBeInTheDocument();
		expect(screen.queryByTestId("move-force-option")).not.toBeInTheDocument();
		expect(screen.getByTestId("confirm-move-button")).toBeDisabled();
		await user.click(
			screen.getByRole("checkbox", {
				name: "I understand this data stays behind",
			}),
		);

		await user.click(screen.getByTestId("confirm-move-button"));

		await waitFor(() => expect(moveRequests.count).toBe(1));
		expect(moveRequests.body).toEqual({
			targetMachineId: "m-2",
			mode: "copy",
		});
	});

	it("disables confirm on a collision with a clear message", async () => {
		server.use(
			quadletsHandler(() => [web]),
			machinesHandler([source, staging, readOnly, backup]),
			previewHandler({ collision: true }),
			moveHandler(),
		);
		const user = await openMoveDialog();

		expect(
			await screen.findByTestId("move-collision-warning"),
		).toHaveTextContent("already exists on staging-01");
		// Even the explicit volume acknowledgment must not unlock a doomed
		// transfer.
		await user.click(
			screen.getByRole("checkbox", {
				name: "I understand this data stays behind",
			}),
		);
		expect(screen.getByTestId("confirm-move-button")).toBeDisabled();
		expect(moveRequests.count).toBe(0);
	});

	it("keeps confirm disabled while the preview is pending", async () => {
		server.use(
			quadletsHandler(() => [web]),
			machinesHandler([source, staging, readOnly, backup]),
			http.get(
				"*/api/machines/m-1/quadlets/web/move-preview",
				() => new Promise<Response>(() => {}),
			),
		);
		await openMoveDialog();

		expect(
			await screen.findByTestId("move-preview-pending"),
		).toBeInTheDocument();
		expect(screen.getByTestId("confirm-move-button")).toBeDisabled();
	});

	it("shows a failed preview and keeps confirm disabled", async () => {
		server.use(
			quadletsHandler(() => [web]),
			machinesHandler([source, staging, readOnly, backup]),
			http.get("*/api/machines/m-1/quadlets/web/move-preview", () =>
				HttpResponse.json(
					errorJson("ACCESS_DENIED", "Access denied", "req-71"),
					{ status: 403 },
				),
			),
		);
		await openMoveDialog();

		// A client-side envelope passes through the shared error policy; a 5xx
		// preview failure collapses to the generic error and renders the same.
		expect(await screen.findByText("ACCESS_DENIED")).toBeInTheDocument();
		expect(screen.getByTestId("confirm-move-button")).toBeDisabled();
	});

	it("closes on success and refetches the source list so the quadlet is gone", async () => {
		let quadlets = [web];
		server.use(
			quadletsHandler(() => quadlets),
			machinesHandler([source, staging, readOnly, backup]),
			previewHandler({ volumes: [] }),
			moveHandler(200, quadletMoveResultFixture(), () => {
				quadlets = [];
			}),
		);
		const user = await openMoveDialog();
		await waitFor(() =>
			expect(screen.getByTestId("confirm-move-button")).toBeEnabled(),
		);

		await user.click(screen.getByTestId("confirm-move-button"));

		await waitFor(() =>
			expect(screen.queryByTestId("modal")).not.toBeInTheDocument(),
		);
		await waitFor(() =>
			expect(screen.queryByText("web")).not.toBeInTheDocument(),
		);
		expect(moveRequests.count).toBe(1);
	});

	it("keeps the dialog open on a partial result, naming the both-machines state", async () => {
		useMoveHandlers();
		server.use(
			moveHandler(
				200,
				quadletMoveResultFixture({
					status: "partial",
					error: "failed to remove quadlet: resource running",
				}),
			),
		);
		const user = await openMoveDialogReadyToConfirm();

		await user.click(screen.getByTestId("confirm-move-button"));

		const message = await screen.findByTestId("move-partial-message");
		expect(message).toHaveTextContent("staging-01");
		expect(message).toHaveTextContent("prod-web-01");
		expect(message).toHaveTextContent("both machines");
		expect(message).toHaveTextContent(
			"failed to remove quadlet: resource running",
		);
		expect(screen.getByTestId("modal")).toBeInTheDocument();
		expect(screen.queryByTestId("confirm-move-button")).not.toBeInTheDocument();

		await user.click(screen.getByTestId("close-move-report-button"));
		await waitFor(() =>
			expect(screen.queryByTestId("modal")).not.toBeInTheDocument(),
		);
	});

	it("surfaces a failed transfer in the error region and stays open", async () => {
		server.use(
			quadletsHandler(() => [web]),
			machinesHandler([source, staging, readOnly, backup]),
			previewHandler({ volumes: [] }),
			moveHandler(403, errorJson("ACCESS_DENIED", "Access denied", "req-72")),
		);
		const user = await openMoveDialog();
		await waitFor(() =>
			expect(screen.getByTestId("confirm-move-button")).toBeEnabled(),
		);

		await user.click(screen.getByTestId("confirm-move-button"));

		expect(await screen.findByText("ACCESS_DENIED")).toBeInTheDocument();
		expect(screen.getByTestId("modal")).toBeInTheDocument();
		expect(screen.getByTestId("confirm-move-button")).toBeInTheDocument();
	});

	it("shows no companion option when the preview reports no companions", async () => {
		useMoveHandlers();
		await openMoveDialogWithPreview();

		expect(
			screen.queryByTestId("move-companion-option"),
		).not.toBeInTheDocument();
	});

	it("offers the companion option when the preview reports companions, defaulting on", async () => {
		server.use(
			quadletsHandler(() => [web]),
			machinesHandler([source, staging, readOnly, backup]),
			previewHandler({
				volumes: [quadletMoveCompanionVolumeFixture("absent"), hostPathVolume],
			}),
			moveHandler(),
		);
		await openMoveDialogWithPreview();

		const companionOption = screen.getByRole("checkbox", {
			name: /Transfer the volume quadlet definitions/,
		});
		expect(companionOption).toBeChecked();
		expect(
			screen.getByText(
				"Transfer the volume quadlet definitions (app-data.volume)",
			),
		).toBeInTheDocument();
	});

	it("annotates companion entries in the volume list", async () => {
		server.use(
			quadletsHandler(() => [web]),
			machinesHandler([source, staging, readOnly, backup]),
			previewHandler({
				volumes: [
					quadletMoveCompanionVolumeFixture("absent"),
					quadletMoveCompanionVolumeFixture("identical", "db.volume"),
					quadletMoveCompanionVolumeFixture("differs", "cache.volume"),
					hostPathVolume,
				],
			}),
			moveHandler(),
		);
		await openMoveDialogWithPreview();

		const rows = screen.getAllByTestId("move-volume-row");
		expect(rows[0]).toHaveTextContent("definition will be copied");
		expect(rows[1]).toHaveTextContent("already present on target");
		expect(rows[2]).toHaveTextContent("differs on target");
		expect(
			within(rows[3]).queryByTestId("move-companion-note"),
		).not.toBeInTheDocument();
	});

	it("sends transferVolumeQuadlets: false when the companion option is unchecked", async () => {
		server.use(
			quadletsHandler(() => [web]),
			machinesHandler([source, staging, readOnly, backup]),
			previewHandler({
				volumes: [quadletMoveCompanionVolumeFixture("absent"), hostPathVolume],
			}),
			moveHandler(),
		);
		const user = await openMoveDialogWithPreview();

		await user.click(
			screen.getByRole("checkbox", {
				name: /Transfer the volume quadlet definitions/,
			}),
		);
		await user.click(
			screen.getByRole("checkbox", {
				name: "I understand this data stays behind",
			}),
		);
		await waitFor(() =>
			expect(screen.getByTestId("confirm-move-button")).toBeEnabled(),
		);
		await user.click(screen.getByTestId("confirm-move-button"));

		await waitFor(() => expect(moveRequests.count).toBe(1));
		expect(moveRequests.body).toEqual({
			targetMachineId: "m-2",
			mode: "move",
			stop: true,
			force: false,
			transferVolumeQuadlets: false,
		});
	});

	it("sends transferVolumeQuadlets: true in copy mode when the option stays checked", async () => {
		server.use(
			quadletsHandler(() => [web]),
			machinesHandler([source, staging, readOnly, backup]),
			previewHandler({
				volumes: [quadletMoveCompanionVolumeFixture("absent"), hostPathVolume],
			}),
			moveHandler(),
		);
		const user = await openMoveDialogWithPreview();

		await user.click(
			screen.getByRole("radio", { name: "Copy (keep this machine's quadlet)" }),
		);
		await user.click(
			screen.getByRole("checkbox", {
				name: "I understand this data stays behind",
			}),
		);
		await waitFor(() =>
			expect(screen.getByTestId("confirm-move-button")).toBeEnabled(),
		);
		await user.click(screen.getByTestId("confirm-move-button"));

		await waitFor(() => expect(moveRequests.count).toBe(1));
		expect(moveRequests.body).toEqual({
			targetMachineId: "m-2",
			mode: "copy",
			transferVolumeQuadlets: true,
		});
	});

	it("disables confirm when a companion differs on the target, naming the conflict", async () => {
		server.use(
			quadletsHandler(() => [web]),
			machinesHandler([source, staging, readOnly, backup]),
			previewHandler({
				volumes: [quadletMoveCompanionVolumeFixture("differs"), hostPathVolume],
			}),
			moveHandler(),
		);
		const user = await openMoveDialogWithPreview();

		const warning = screen.getByTestId("move-companion-conflict-warning");
		expect(warning).toHaveTextContent("app-data.volume");
		expect(warning).toHaveTextContent("staging-01");
		expect(warning).toHaveTextContent("different content");

		// Even the explicit data acknowledgment must not unlock a doomed
		// transfer — a differing companion blocks confirm like a collision.
		await user.click(
			screen.getByRole("checkbox", {
				name: "I understand this data stays behind",
			}),
		);
		expect(screen.getByTestId("confirm-move-button")).toBeDisabled();
		expect(moveRequests.count).toBe(0);
	});

	it("keeps the data-stays-behind warning in both modes and both checkbox states", async () => {
		server.use(
			quadletsHandler(() => [web]),
			machinesHandler([source, staging, readOnly, backup]),
			previewHandler({
				volumes: [quadletMoveCompanionVolumeFixture("absent"), hostPathVolume],
			}),
			moveHandler(),
		);
		const user = await openMoveDialogWithPreview();

		// Move mode, companions on: the warning names the source machine.
		expect(screen.getByTestId("move-volume-warning")).toHaveTextContent(
			"stays behind on prod-web-01",
		);

		// Copy mode, companions off: the data never travels, so the warning
		// holds regardless.
		await user.click(
			screen.getByRole("radio", { name: "Copy (keep this machine's quadlet)" }),
		);
		await user.click(
			screen.getByRole("checkbox", {
				name: /Transfer the volume quadlet definitions/,
			}),
		);
		expect(screen.getByTestId("move-volume-warning")).toHaveTextContent(
			"stays behind on prod-web-01",
		);
	});

	it("hides the migrate and overwrite options when nothing is migratable", async () => {
		server.use(
			quadletsHandler(() => [web]),
			machinesHandler([source, staging, readOnly, backup]),
			previewHandler({
				volumes: [
					quadletMoveDataVolumeFixture({
						migratable: false,
						reason: "source volume does not exist",
					}),
					hostPathVolume,
				],
			}),
			moveHandler(),
		);
		await openMoveDialogWithPreview();

		expect(screen.queryByTestId("move-migrate-option")).not.toBeInTheDocument();
		expect(
			screen.queryByTestId("move-overwrite-option"),
		).not.toBeInTheDocument();
	});

	it("offers the migrate option default-off when a volume is migratable, without the overwrite option on a fresh target", async () => {
		useMoveHandlers();
		await openMoveDialogWithPreview();

		const migrateOption = screen.getByRole("checkbox", {
			name: /Migrate volume data to “staging-01”/,
		});
		expect(migrateOption).not.toBeChecked();
		expect(
			screen.queryByTestId("move-overwrite-option"),
		).not.toBeInTheDocument();
		// Not running: no hot-export caveat even with migration available.
		await userEvent.click(migrateOption);
		expect(
			screen.queryByTestId("move-hot-export-warning"),
		).not.toBeInTheDocument();
	});

	it("renders the volume data facts per entry (issue 214)", async () => {
		server.use(
			quadletsHandler(() => [web]),
			machinesHandler([source, staging, readOnly, backup]),
			previewHandler({
				volumes: [
					quadletMoveDataVolumeFixture({
						sizeBytes: 1572864,
						existsOnTarget: true,
						targetSizeBytes: 2048,
					}),
					quadletMoveDataVolumeFixture({
						source: "ghost",
						podmanVolume: "ghost",
						migratable: false,
						reason: "source volume does not exist",
					}),
					hostPathVolume,
				],
			}),
			moveHandler(),
		);
		await openMoveDialogWithPreview();

		const rows = screen.getAllByTestId("move-volume-row");
		expect(
			within(rows[0]).getByTestId("move-volume-resolved"),
		).toHaveTextContent("resolves to “pgdata” (1.6 MB)");
		expect(within(rows[0]).getByTestId("move-volume-target")).toHaveTextContent(
			"already on target (2 KB)",
		);
		expect(
			within(rows[1]).getByTestId("move-volume-resolved"),
		).toHaveTextContent("resolves to “ghost”");
		expect(within(rows[1]).getByTestId("move-volume-target")).toHaveTextContent(
			"new on target",
		);
		expect(within(rows[1]).getByTestId("move-volume-reason")).toHaveTextContent(
			"source volume does not exist",
		);
		// A host path claims nothing: its reason only.
		expect(
			within(rows[2]).queryByTestId("move-volume-resolved"),
		).not.toBeInTheDocument();
		expect(
			within(rows[2]).queryByTestId("move-volume-target"),
		).not.toBeInTheDocument();
		expect(within(rows[2]).getByTestId("move-volume-reason")).toHaveTextContent(
			"host bind paths are not migrated",
		);
	});

	it("shows the hot-export caveat when the source is running and migration is checked", async () => {
		server.use(
			quadletsHandler(() => [web]),
			machinesHandler([source, staging, readOnly, backup]),
			previewHandler({ running: true }),
			moveHandler(),
		);
		const user = await openMoveDialogWithPreview();

		expect(
			screen.queryByTestId("move-hot-export-warning"),
		).not.toBeInTheDocument();
		await user.click(
			screen.getByRole("checkbox", { name: /Migrate volume data/ }),
		);
		expect(screen.getByTestId("move-hot-export-warning")).toHaveTextContent(
			"running",
		);
	});

	it("runs a checked migration over the NDJSON transport and narrates the outcomes", async () => {
		server.use(
			countingQuadletsHandler(() => [web]),
			machinesHandler([source, staging, readOnly, backup]),
			previewHandler(),
			moveStreamHandler(
				[
					{
						operationId: "op-1",
						type: "volume",
						machineId: "m-1",
						ref: "pgdata",
						podmanVolume: "pgdata",
						phase: "start",
					},
					{
						operationId: "op-1",
						type: "volume",
						machineId: "m-1",
						ref: "pgdata",
						podmanVolume: "pgdata",
						phase: "done",
						bytes: 2097152,
					},
					{
						operationId: "op-1",
						type: "volume",
						machineId: "m-1",
						ref: "ghost",
						podmanVolume: "ghost",
						phase: "skip",
						reason: "source volume does not exist",
					},
					{
						operationId: "op-1",
						type: "complete",
						machineId: "m-1",
						result: quadletMoveResultFixture({
							status: "moved",
							volumeMigration: {
								volumes: [
									{
										ref: "pgdata",
										podmanVolume: "pgdata",
										outcome: "migrated",
									},
									{
										ref: "ghost",
										podmanVolume: "ghost",
										outcome: "skipped",
										reason: "source volume does not exist",
									},
								],
							},
						}),
					},
				],
				true,
			),
		);
		const user = await openMoveDialogReadyToConfirm();
		const before = quadletFetches;

		await user.click(
			screen.getByRole("checkbox", { name: /Migrate volume data/ }),
		);
		await user.click(screen.getByTestId("confirm-move-button"));

		// The transfer rode the stream endpoint with the migration flag — the
		// plain request's stop/force vocabulary still rides along in move mode.
		await waitFor(() => expect(moveRequests.count).toBe(1));
		expect(moveRequests.body).toEqual({
			targetMachineId: "m-2",
			mode: "move",
			stop: true,
			force: false,
			migrateVolumeData: true,
		});

		// Per-volume progress off the generic transport, the latest phase per
		// volume winning while the stream runs.
		const pgdata = await screen.findByTestId("move-migration-volume-pgdata");
		const ghost = await screen.findByTestId("move-migration-volume-ghost");
		await waitFor(() => {
			expect(pgdata).toHaveTextContent("done (2.1 MB)");
			expect(ghost).toHaveTextContent("skipped — source volume does not exist");
		});

		// The terminal event narrates in place instead of closing: migrated
		// and skipped volumes with their reasons, the source list refetching
		// behind the dialog.
		const report = await screen.findByTestId("move-migration-success");
		expect(report).toHaveTextContent("Moved");
		expect(report).toHaveTextContent("staging-01");
		expect(
			screen.queryByTestId("move-migration-progress"),
		).not.toBeInTheDocument();
		await waitFor(() => expect(quadletFetches).toBeGreaterThan(before));
		expect(
			screen.getByTestId("move-migration-outcome-pgdata"),
		).toHaveTextContent("data migrated");
		expect(
			screen.getByTestId("move-migration-outcome-ghost"),
		).toHaveTextContent("skipped — source volume does not exist");
		expect(screen.queryByTestId("confirm-move-button")).not.toBeInTheDocument();

		await user.click(screen.getByTestId("close-move-report-button"));
		await waitFor(() =>
			expect(screen.queryByTestId("modal")).not.toBeInTheDocument(),
		);
	});

	it("sends overwriteVolumes only with a checked migration onto an existing target volume", async () => {
		server.use(
			quadletsHandler(() => [web]),
			machinesHandler([source, staging, readOnly, backup]),
			previewHandler({
				volumes: [
					quadletMoveDataVolumeFixture({ existsOnTarget: true }),
					hostPathVolume,
				],
			}),
			moveStreamHandler([
				{
					operationId: "op-1",
					type: "complete",
					machineId: "m-1",
					result: quadletMoveResultFixture({
						status: "copied",
						volumeMigration: {
							volumes: [
								{
									ref: "pgdata",
									podmanVolume: "pgdata",
									outcome: "migrated",
								},
							],
						},
					}),
				},
			]),
		);
		const user = await openMoveDialogReadyToConfirm();

		// existsOnTarget surfaced the overwrite option, default off.
		const overwriteOption = screen.getByRole("checkbox", {
			name: /Overwrite existing target volumes/,
		});
		expect(overwriteOption).not.toBeChecked();

		await user.click(
			screen.getByRole("checkbox", { name: /Migrate volume data/ }),
		);
		await user.click(overwriteOption);
		await user.click(screen.getByTestId("confirm-move-button"));

		await waitFor(() => expect(moveRequests.count).toBe(1));
		expect(moveRequests.body).toEqual({
			targetMachineId: "m-2",
			mode: "move",
			stop: true,
			force: false,
			migrateVolumeData: true,
			overwriteVolumes: true,
		});
	});

	it("keeps the plain request unchanged when migration stays unchecked", async () => {
		server.use(
			quadletsHandler(() => [web]),
			machinesHandler([source, staging, readOnly, backup]),
			previewHandler({
				volumes: [
					quadletMoveDataVolumeFixture({ existsOnTarget: true }),
					hostPathVolume,
				],
			}),
			moveHandler(),
		);
		const user = await openMoveDialogReadyToConfirm();

		await user.click(screen.getByTestId("confirm-move-button"));

		await waitFor(() => expect(moveRequests.count).toBe(1));
		expect(moveRequests.body).toEqual({
			targetMachineId: "m-2",
			mode: "move",
			stop: true,
			force: false,
		});
	});

	it("keeps a migrating partial result open with both machines and the outcomes", async () => {
		server.use(
			quadletsHandler(() => [web]),
			machinesHandler([source, staging, readOnly, backup]),
			previewHandler(),
			moveStreamHandler([
				{
					operationId: "op-1",
					type: "complete",
					machineId: "m-1",
					result: quadletMoveResultFixture({
						status: "partial",
						error: "failed to remove quadlet: resource running",
						volumeMigration: {
							volumes: [
								{
									ref: "pgdata",
									podmanVolume: "pgdata",
									outcome: "migrated",
								},
							],
						},
					}),
				},
			]),
		);
		const user = await openMoveDialogReadyToConfirm();

		await user.click(
			screen.getByRole("checkbox", { name: /Migrate volume data/ }),
		);
		await user.click(screen.getByTestId("confirm-move-button"));

		// The partial contract holds on the stream transport too, with the
		// data outcomes narrated beneath the both-machines message.
		const message = await screen.findByTestId("move-partial-message");
		expect(message).toHaveTextContent("both machines");
		expect(message).toHaveTextContent(
			"failed to remove quadlet: resource running",
		);
		expect(
			within(message).getByTestId("move-migration-report"),
		).toBeInTheDocument();
		expect(
			within(message).getByTestId("move-migration-outcome-pgdata"),
		).toHaveTextContent("data migrated");
		expect(screen.queryByTestId("confirm-move-button")).not.toBeInTheDocument();
	});

	it("maps VOLUME_MIGRATION_TARGET_NOT_EMPTY to the overwrite hint and a retryable confirm", async () => {
		server.use(
			quadletsHandler(() => [web]),
			machinesHandler([source, staging, readOnly, backup]),
			previewHandler(),
			moveStreamHandler([
				{
					operationId: "op-1",
					type: "volume",
					machineId: "m-1",
					ref: "pgdata",
					podmanVolume: "pgdata",
					phase: "start",
				},
				{
					operationId: "op-1",
					type: "error",
					machineId: "m-1",
					error: {
						code: "VOLUME_MIGRATION_TARGET_NOT_EMPTY",
						message: "target volume pgdata is not empty",
					},
				},
			]),
		);
		const user = await openMoveDialogReadyToConfirm();

		await user.click(
			screen.getByRole("checkbox", { name: /Migrate volume data/ }),
		);
		await user.click(screen.getByTestId("confirm-move-button"));

		// The stream's refusal surfaces with its code and the volume-naming
		// message, plus the hint that names the way out.
		expect(
			await screen.findByText("VOLUME_MIGRATION_TARGET_NOT_EMPTY"),
		).toBeInTheDocument();
		expect(
			screen.getByText("target volume pgdata is not empty"),
		).toBeInTheDocument();
		expect(screen.getByTestId("move-overwrite-hint")).toHaveTextContent(
			"Overwrite existing target volumes",
		);
		// The dialog stays open and the transfer can be corrected and retried.
		expect(screen.getByTestId("confirm-move-button")).toBeEnabled();
		expect(screen.getByTestId("modal")).toBeInTheDocument();
	});

	it("does not smuggle a stale migration into a target whose preview reports nothing migratable", async () => {
		// The first target previews a migratable volume; the backup machine's
		// preview reports only a dangling reference and a host path.
		server.use(
			quadletsHandler(() => [web]),
			machinesHandler([source, staging, readOnly, backup]),
			http.get(
				"*/api/machines/m-1/quadlets/web/move-preview",
				({ request }) => {
					const target = new URL(request.url).searchParams.get(
						"targetMachineId",
					);
					previewTargets.push(target ?? "");
					return HttpResponse.json(
						quadletMovePreviewFixture(
							target === "m-4"
								? {
										volumes: [
											quadletMoveDataVolumeFixture({
												migratable: false,
												reason: "source volume does not exist",
											}),
											hostPathVolume,
										],
									}
								: {},
						),
					);
				},
			),
			moveHandler(),
		);
		const user = await openMoveDialogWithPreview();

		await user.click(
			screen.getByRole("checkbox", { name: /Migrate volume data/ }),
		);
		await user.selectOptions(screen.getByTestId("move-target-select"), "m-4");

		// The checkbox disappears with the preview, and the checked state it
		// leaves behind must not switch the transport on confirm.
		await waitFor(() =>
			expect(
				screen.queryByTestId("move-migrate-option"),
			).not.toBeInTheDocument(),
		);
		await user.click(
			screen.getByRole("checkbox", {
				name: "I understand this data stays behind",
			}),
		);
		await waitFor(() =>
			expect(screen.getByTestId("confirm-move-button")).toBeEnabled(),
		);
		await user.click(screen.getByTestId("confirm-move-button"));

		await waitFor(() => expect(moveRequests.count).toBe(1));
		expect(moveRequests.body).toEqual({
			targetMachineId: "m-4",
			mode: "move",
			stop: true,
			force: false,
		});
	});

	it("renders the secrets warning with kind and existence facts in both modes (issue 222)", async () => {
		server.use(
			quadletsHandler(() => [web]),
			machinesHandler([source, staging, readOnly, backup]),
			previewHandler(secretPreview),
			moveHandler(),
		);
		const user = await openMoveDialogWithPreview();

		const warning = screen.getByTestId("move-secret-warning");
		expect(warning).toHaveTextContent(
			"Secret values do not move with the file — they stay behind on prod-web-01",
		);
		const rows = screen.getAllByTestId("move-secret-row");
		// Transferable: a podman secret, fresh on the target.
		expect(rows[0]).toHaveTextContent("db-password");
		expect(rows[0]).toHaveTextContent("podman secret");
		expect(
			within(rows[0]).queryByTestId("move-secret-source"),
		).not.toBeInTheDocument();
		expect(within(rows[0]).getByTestId("move-secret-target")).toHaveTextContent(
			"new on target",
		);
		// Same name already on the target: the stranded-dependency warning.
		expect(within(rows[1]).getByTestId("move-secret-target")).toHaveTextContent(
			"already on target",
		);
		// Dangling: renders honestly as missing here.
		expect(within(rows[2]).getByTestId("move-secret-source")).toHaveTextContent(
			"missing on this machine",
		);
		// A host path claims nothing: podman reads it off the filesystem.
		expect(
			within(rows[3]).queryByTestId("move-secret-source"),
		).not.toBeInTheDocument();
		expect(
			within(rows[3]).queryByTestId("move-secret-target"),
		).not.toBeInTheDocument();

		// The warning holds in copy mode alike — the stranded values are the
		// same truth either way, and so does the admin affordance.
		await user.click(
			screen.getByRole("radio", { name: "Copy (keep this machine's quadlet)" }),
		);
		expect(screen.getByTestId("move-secret-warning")).toBeInTheDocument();
		expect(
			screen.getByTestId("move-transfer-secrets-option"),
		).toBeInTheDocument();
	});

	it("offers the transfer-secrets option default-off only for an admin caller with a transferable secret (issue 222)", async () => {
		server.use(
			quadletsHandler(() => [web]),
			machinesHandler([source, staging, readOnly, backup]),
			previewHandler(secretPreview),
			moveHandler(),
		);
		const user = await openMoveDialogWithPreview();

		const option = screen.getByTestId("move-transfer-secrets-option");
		expect(option).not.toBeChecked();

		// A preview reporting no secrets takes the option and the warning away.
		server.use(previewHandler());
		await user.selectOptions(screen.getByTestId("move-target-select"), "m-4");
		await waitFor(() =>
			expect(
				screen.queryByTestId("move-transfer-secrets-option"),
			).not.toBeInTheDocument(),
		);
		expect(screen.queryByTestId("move-secret-warning")).not.toBeInTheDocument();
	});

	it("hides the transfer-secrets option when nothing is transferable but keeps the warning (issue 222)", async () => {
		server.use(
			quadletsHandler(() => [web]),
			machinesHandler([source, staging, readOnly, backup]),
			previewHandler({
				secrets: [
					quadletMovePreviewSecretFixture({
						name: "ghost",
						existsOnSource: false,
					}),
					quadletMovePreviewSecretFixture({
						name: "/etc/secrets/api.key",
						kind: "hostPath",
						existsOnSource: false,
						existsOnTarget: false,
					}),
				],
			}),
			moveHandler(),
		);
		await openMoveDialogWithPreview();

		expect(screen.getByTestId("move-secret-warning")).toBeInTheDocument();
		expect(
			screen.queryByTestId("move-transfer-secrets-option"),
		).not.toBeInTheDocument();
	});

	it("hides the transfer-secrets option for a non-admin caller but keeps the warning (issue 222)", async () => {
		server.use(
			quadletsHandler(() => [web]),
			machinesHandler([source, staging, readOnly, backup]),
			previewHandler(secretPreview),
			moveHandler(),
		);
		renderWithQuery(
			<QuadletList
				machine={machineFixture({ permission: "write", isOwner: false })}
			/>,
			{ router: true },
		);
		const user = userEvent.setup();
		await user.click(
			await screen.findByRole("button", { name: "Move or copy web" }),
		);

		expect(
			await screen.findByTestId("move-secret-warning"),
		).toBeInTheDocument();
		expect(
			screen.queryByTestId("move-transfer-secrets-option"),
		).not.toBeInTheDocument();
	});

	it("sends no transferSecrets when the option is offered but stays unchecked (issue 222)", async () => {
		server.use(
			quadletsHandler(() => [web]),
			machinesHandler([source, staging, readOnly, backup]),
			previewHandler(secretPreview),
			moveHandler(),
		);
		const user = await openMoveDialogReadyToConfirm();

		await user.click(screen.getByTestId("confirm-move-button"));

		await waitFor(() => expect(moveRequests.count).toBe(1));
		expect(moveRequests.body).toEqual({
			targetMachineId: "m-2",
			mode: "move",
			stop: true,
			force: false,
		});
	});

	it("sends transferSecrets: true when the option is checked — no transport switch (issue 222)", async () => {
		server.use(
			quadletsHandler(() => [web]),
			machinesHandler([source, staging, readOnly, backup]),
			previewHandler(secretPreview),
			moveHandler(),
		);
		const user = await openMoveDialogReadyToConfirm();

		await user.click(screen.getByTestId("move-transfer-secrets-option"));
		await user.click(screen.getByTestId("confirm-move-button"));

		// The value transfer rides the plain request — no NDJSON, no progress UI.
		await waitFor(() => expect(moveRequests.count).toBe(1));
		expect(moveRequests.body).toEqual({
			targetMachineId: "m-2",
			mode: "move",
			stop: true,
			force: false,
			transferSecrets: true,
		});
	});

	it("does not smuggle a stale secret transfer into a target whose preview reports none (issue 222)", async () => {
		server.use(
			quadletsHandler(() => [web]),
			machinesHandler([source, staging, readOnly, backup]),
			http.get(
				"*/api/machines/m-1/quadlets/web/move-preview",
				({ request }) => {
					const target = new URL(request.url).searchParams.get(
						"targetMachineId",
					);
					previewTargets.push(target ?? "");
					return HttpResponse.json(
						quadletMovePreviewFixture(
							target === "m-4" ? { secrets: [] } : secretPreview,
						),
					);
				},
			),
			moveHandler(),
		);
		const user = await openMoveDialogWithPreview();

		await user.click(screen.getByTestId("move-transfer-secrets-option"));
		await user.selectOptions(screen.getByTestId("move-target-select"), "m-4");

		// The checkbox disappears with the preview, and the checked state it
		// leaves behind must not move values the preview no longer offers.
		await waitFor(() =>
			expect(
				screen.queryByTestId("move-transfer-secrets-option"),
			).not.toBeInTheDocument(),
		);
		await user.click(
			screen.getByRole("checkbox", {
				name: "I understand this data stays behind",
			}),
		);
		await waitFor(() =>
			expect(screen.getByTestId("confirm-move-button")).toBeEnabled(),
		);
		await user.click(screen.getByTestId("confirm-move-button"));

		await waitFor(() => expect(moveRequests.count).toBe(1));
		expect(moveRequests.body).toEqual({
			targetMachineId: "m-4",
			mode: "move",
			stop: true,
			force: false,
		});
	});

	it("narrates the secretTransfer outcomes on a plain transfer and stays open (issue 222)", async () => {
		server.use(
			quadletsHandler(() => [web]),
			machinesHandler([source, staging, readOnly, backup]),
			previewHandler(secretPreview),
			moveHandler(
				200,
				quadletMoveResultFixture({
					status: "copied",
					secretTransfer: {
						secrets: [
							quadletMoveSecretOutcomeFixture(),
							quadletMoveSecretOutcomeFixture({
								name: "tls-cert",
								outcome: "skipped",
								reason: "a secret with this name already exists on the target",
							}),
							quadletMoveSecretOutcomeFixture({
								name: "ghost",
								outcome: "skipped",
								reason: "source secret does not exist",
							}),
						],
					},
				}),
			),
		);
		const user = await openMoveDialogReadyToConfirm();

		await user.click(screen.getByTestId("move-transfer-secrets-option"));
		await user.click(screen.getByTestId("confirm-move-button"));

		// The outcomes are the answer's point: the dialog stays open on them
		// the way the migrating transport's report does.
		const report = await screen.findByTestId("move-secret-success");
		expect(report).toHaveTextContent("Copied");
		expect(report).toHaveTextContent("staging-01");
		expect(
			screen.getByTestId("move-secret-outcome-db-password"),
		).toHaveTextContent("value transferred");
		expect(
			screen.getByTestId("move-secret-outcome-tls-cert"),
		).toHaveTextContent(
			"skipped — a secret with this name already exists on the target",
		);
		expect(screen.getByTestId("move-secret-outcome-ghost")).toHaveTextContent(
			"skipped — source secret does not exist",
		);
		expect(screen.queryByTestId("confirm-move-button")).not.toBeInTheDocument();

		await user.click(screen.getByTestId("close-move-report-button"));
		await waitFor(() =>
			expect(screen.queryByTestId("modal")).not.toBeInTheDocument(),
		);
	});

	it("narrates secretTransfer beside volumeMigration in the stream's terminal report (issue 222)", async () => {
		server.use(
			quadletsHandler(() => [web]),
			machinesHandler([source, staging, readOnly, backup]),
			previewHandler(secretPreview),
			moveStreamHandler([
				{
					operationId: "op-1",
					type: "complete",
					machineId: "m-1",
					result: quadletMoveResultFixture({
						status: "moved",
						volumeMigration: {
							volumes: [
								{
									ref: "pgdata",
									podmanVolume: "pgdata",
									outcome: "migrated",
								},
							],
						},
						secretTransfer: {
							secrets: [
								quadletMoveSecretOutcomeFixture(),
								quadletMoveSecretOutcomeFixture({
									name: "tls-cert",
									outcome: "skipped",
									reason:
										"a secret with this name already exists on the target",
								}),
							],
						},
					}),
				},
			]),
		);
		const user = await openMoveDialogReadyToConfirm();

		await user.click(
			screen.getByRole("checkbox", { name: /Migrate volume data/ }),
		);
		await user.click(screen.getByTestId("move-transfer-secrets-option"));
		await user.click(screen.getByTestId("confirm-move-button"));

		// Both flags ride one request; the secrets land in the terminal
		// event's envelope beside the volume outcomes — no progress UI of
		// their own.
		await waitFor(() => expect(moveRequests.count).toBe(1));
		expect(moveRequests.body).toEqual({
			targetMachineId: "m-2",
			mode: "move",
			stop: true,
			force: false,
			transferSecrets: true,
			migrateVolumeData: true,
		});
		const report = await screen.findByTestId("move-migration-success");
		expect(
			screen.getByTestId("move-migration-outcome-pgdata"),
		).toHaveTextContent("data migrated");
		expect(
			within(report).getByTestId("move-secret-outcome-db-password"),
		).toHaveTextContent("value transferred");
		expect(
			within(report).getByTestId("move-secret-outcome-tls-cert"),
		).toHaveTextContent(
			"skipped — a secret with this name already exists on the target",
		);
	});

	it("narrates skipped secrets under a partial result too (issue 222)", async () => {
		server.use(
			quadletsHandler(() => [web]),
			machinesHandler([source, staging, readOnly, backup]),
			previewHandler(secretPreview),
			moveHandler(
				200,
				quadletMoveResultFixture({
					status: "partial",
					error: "failed to remove quadlet: resource running",
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
				}),
			),
		);
		const user = await openMoveDialogReadyToConfirm();

		await user.click(screen.getByTestId("move-transfer-secrets-option"));
		await user.click(screen.getByTestId("confirm-move-button"));

		const message = await screen.findByTestId("move-partial-message");
		expect(
			within(message).getByTestId("move-secret-outcome-db-password"),
		).toHaveTextContent("value transferred");
		expect(
			within(message).getByTestId("move-secret-outcome-ghost"),
		).toHaveTextContent("skipped — source secret does not exist");
	});

	it("maps SECRET_TRANSFER_FAILED on the stream error to an actionable message naming the secret (issue 222)", async () => {
		server.use(
			quadletsHandler(() => [web]),
			machinesHandler([source, staging, readOnly, backup]),
			previewHandler(secretPreview),
			moveStreamHandler([
				{
					operationId: "op-1",
					type: "error",
					machineId: "m-1",
					error: {
						code: "SECRET_TRANSFER_FAILED",
						message: "Secret db-password: failed to create secret on target",
					},
				},
			]),
		);
		const user = await openMoveDialogReadyToConfirm();

		await user.click(
			screen.getByRole("checkbox", { name: /Migrate volume data/ }),
		);
		await user.click(screen.getByTestId("move-transfer-secrets-option"));
		await user.click(screen.getByTestId("confirm-move-button"));

		// The sentinel's code and secret-naming message surface, plus the way
		// out: nothing was installed, and a re-run skips what already landed.
		expect(
			await screen.findByText("SECRET_TRANSFER_FAILED"),
		).toBeInTheDocument();
		expect(
			screen.getByText("Secret db-password: failed to create secret on target"),
		).toBeInTheDocument();
		expect(screen.getByTestId("move-secret-hint")).toHaveTextContent(
			"Nothing was installed",
		);
		expect(screen.getByTestId("confirm-move-button")).toBeEnabled();
		expect(screen.getByTestId("modal")).toBeInTheDocument();
	});
});
