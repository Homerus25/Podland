import { QuadletList } from "@features/quadlets/components/QuadletList";
import type {
	QuadletMovePreview,
	QuadletSummary,
} from "@features/quadlets/services/quadletsApi";
import type { Host } from "@shared/types/api";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http, type JsonBodyType } from "msw";
import { beforeEach, describe, expect, it } from "vitest";
import { errorJson } from "@/test/errors";
import {
	hostFixture,
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

const source = hostFixture();
const staging = hostFixture({ id: "m-2", name: "staging-01" });
const backup = hostFixture({ id: "m-4", name: "backup-01" });
const readOnly = hostFixture({
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
	return http.get("*/api/hosts/m-1/quadlets", () =>
		HttpResponse.json(quadlets()),
	);
}

// The quadlets list with a fetch counter, for suites that assert the
// both-hosts invalidation as a real refetch (issue 214's stream path).
function countingQuadletsHandler(quadlets: () => QuadletSummary[]) {
	return http.get("*/api/hosts/m-1/quadlets", () => {
		quadletFetches++;
		return HttpResponse.json(quadlets());
	});
}

function hostsHandler(hosts: Host[]) {
	return http.get("*/api/hosts", () => HttpResponse.json(hosts));
}

function previewHandler(overrides: Partial<QuadletMovePreview> = {}) {
	return http.get(
		"*/api/hosts/m-1/quadlets/web/move-preview",
		({ request }) => {
			previewTargets.push(
				new URL(request.url).searchParams.get("targetHostId") ?? "",
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
	return http.post("*/api/hosts/m-1/quadlets/web/move", async ({ request }) => {
		moveRequests.count++;
		moveRequests.body = (await request.json()) as Record<string, unknown>;
		if (status !== 200) return HttpResponse.json(body, { status });
		onMoved?.();
		return HttpResponse.json(body);
	});
}

// The migrating transport (issue 214): the same endpoint answered as NDJSON
// events. The events travel as raw framed lines — they are the move domain's
// own event vocabulary, not the shared stream kit's.
function moveStreamHandler(events: Record<string, unknown>[], live = false) {
	const chunks = events.map((event) => `${JSON.stringify(event)}\n`);
	return http.post("*/api/hosts/m-1/quadlets/web/move", async ({ request }) => {
		moveRequests.count++;
		moveRequests.body = (await request.json()) as Record<string, unknown>;
		return live ? liveNdjsonResponse(chunks, 50) : ndjsonResponse([], chunks);
	});
}

// The live transport spaces its chunks with real timers so the narration
// renders per event; under full-suite parallel load those delays inflate
// past testing-library's default 1s waitFor budget, so the live-stream
// test's finds state their own.
const FIND_TIMEOUT = 5000;

// The shared wiring every dialog test needs: the source list, the hosts
// directory (the target picker's source), the preview, and the transfer.
function useMoveHandlers() {
	server.use(
		quadletsHandler(() => [web]),
		hostsHandler([source, staging, readOnly, backup]),
		previewHandler(),
		moveHandler(),
	);
}

async function openMoveDialog() {
	renderWithQuery(<QuadletList host={source} />, { router: true });
	const user = userEvent.setup();
	await user.click(
		await screen.findByRole("button", { name: "Move or copy web" }),
	);
	expect(await screen.findByTestId("modal")).toBeInTheDocument();
	return user;
}

async function openMoveDialogWithPreview() {
	const user = await openMoveDialog();
	// The default target (first writable host) previews clean with volumes;
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
				host={hostFixture({ permission: "read", isOwner: false })}
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

	it("offers one dialog listing only writable hosts, source excluded", async () => {
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

	it("demands explicit acknowledgment of the stranded-data warning in move mode", async () => {
		useMoveHandlers();
		const user = await openMoveDialogWithPreview();

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
			targetHostId: "m-2",
			mode: "move",
			stop: true,
			force: false,
		});
	});

	it("keeps the acknowledgment gate in copy mode, without the move-only stop/force options", async () => {
		useMoveHandlers();
		const user = await openMoveDialogWithPreview();

		await user.click(
			screen.getByRole("radio", { name: "Copy (keep this host's quadlet)" }),
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
			targetHostId: "m-2",
			mode: "copy",
		});
	});

	it("keeps the stranded-data warning in copy mode with the companion option off", async () => {
		server.use(
			quadletsHandler(() => [web]),
			hostsHandler([source, staging, readOnly, backup]),
			previewHandler({
				volumes: [quadletMoveCompanionVolumeFixture("absent"), hostPathVolume],
			}),
			moveHandler(),
		);
		const user = await openMoveDialogWithPreview();

		await user.click(
			screen.getByRole("radio", { name: "Copy (keep this host's quadlet)" }),
		);
		await user.click(
			screen.getByRole("checkbox", {
				name: /Transfer the volume quadlet definitions/,
			}),
		);

		// The warning holds in copy mode with the companions opt-out: the
		// data still stays behind, the companion note answers the
		// unchecked option, and confirm still waits for the
		// acknowledgment.
		expect(screen.getByTestId("move-volume-warning")).toHaveTextContent(
			"it stays behind on prod-web-01",
		);
		expect(
			screen.getByText("definition will not be transferred"),
		).toBeInTheDocument();
		expect(screen.getByTestId("confirm-move-button")).toBeDisabled();
	});

	it("disables confirm on a collision with a clear message", async () => {
		server.use(
			quadletsHandler(() => [web]),
			hostsHandler([source, staging, readOnly, backup]),
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
			hostsHandler([source, staging, readOnly, backup]),
			http.get(
				"*/api/hosts/m-1/quadlets/web/move-preview",
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
			hostsHandler([source, staging, readOnly, backup]),
			http.get("*/api/hosts/m-1/quadlets/web/move-preview", () =>
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
			hostsHandler([source, staging, readOnly, backup]),
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

	it("keeps the dialog open on a partial result", async () => {
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

		// The report slot takes over: the dialog stays open on it, and the
		// footer's Close dismisses it.
		expect(
			await screen.findByTestId("move-partial-message"),
		).toBeInTheDocument();
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
			hostsHandler([source, staging, readOnly, backup]),
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
			hostsHandler([source, staging, readOnly, backup]),
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

	it("sends transferVolumeQuadlets: false when the companion option is unchecked", async () => {
		server.use(
			quadletsHandler(() => [web]),
			hostsHandler([source, staging, readOnly, backup]),
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
			targetHostId: "m-2",
			mode: "move",
			stop: true,
			force: false,
			transferVolumeQuadlets: false,
		});
	});

	it("sends transferVolumeQuadlets: true in copy mode when the option stays checked", async () => {
		server.use(
			quadletsHandler(() => [web]),
			hostsHandler([source, staging, readOnly, backup]),
			previewHandler({
				volumes: [quadletMoveCompanionVolumeFixture("absent"), hostPathVolume],
			}),
			moveHandler(),
		);
		const user = await openMoveDialogWithPreview();

		await user.click(
			screen.getByRole("radio", { name: "Copy (keep this host's quadlet)" }),
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
			targetHostId: "m-2",
			mode: "copy",
			transferVolumeQuadlets: true,
		});
	});

	it("disables confirm when a companion differs on the target", async () => {
		server.use(
			quadletsHandler(() => [web]),
			hostsHandler([source, staging, readOnly, backup]),
			previewHandler({
				volumes: [quadletMoveCompanionVolumeFixture("differs"), hostPathVolume],
			}),
			moveHandler(),
		);
		const user = await openMoveDialogWithPreview();

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

	it("hides the migrate and overwrite options when nothing is migratable", async () => {
		server.use(
			quadletsHandler(() => [web]),
			hostsHandler([source, staging, readOnly, backup]),
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

	it("shows the hot-export caveat when the source is running and migration is checked", async () => {
		server.use(
			quadletsHandler(() => [web]),
			hostsHandler([source, staging, readOnly, backup]),
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

	it("runs a checked migration over the NDJSON transport, narrating in place", async () => {
		server.use(
			countingQuadletsHandler(() => [web]),
			hostsHandler([source, staging, readOnly, backup]),
			previewHandler(),
			moveStreamHandler(
				[
					{
						operationId: "op-1",
						type: "volume",
						hostId: "m-1",
						ref: "pgdata",
						podmanVolume: "pgdata",
						phase: "start",
					},
					{
						operationId: "op-1",
						type: "volume",
						hostId: "m-1",
						ref: "pgdata",
						podmanVolume: "pgdata",
						phase: "done",
						bytes: 2097152,
					},
					{
						operationId: "op-1",
						type: "volume",
						hostId: "m-1",
						ref: "ghost",
						podmanVolume: "ghost",
						phase: "skip",
						reason: "source volume does not exist",
					},
					{
						operationId: "op-1",
						type: "complete",
						hostId: "m-1",
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
		await waitFor(() => expect(moveRequests.count).toBe(1), {
			timeout: FIND_TIMEOUT,
		});
		expect(moveRequests.body).toEqual({
			targetHostId: "m-2",
			mode: "move",
			stop: true,
			force: false,
			migrateVolumeData: true,
		});

		// Per-volume progress reaches the report module while the stream
		// runs; the terminal event's report then takes the body over instead
		// of closing, the source list refetching behind the dialog.
		expect(
			await screen.findByTestId(
				"move-migration-volume-pgdata",
				{},
				{ timeout: FIND_TIMEOUT },
			),
		).toBeInTheDocument();
		expect(
			await screen.findByTestId(
				"move-migration-volume-ghost",
				{},
				{ timeout: FIND_TIMEOUT },
			),
		).toBeInTheDocument();
		expect(
			await screen.findByTestId(
				"move-migration-success",
				{},
				{ timeout: FIND_TIMEOUT },
			),
		).toBeInTheDocument();
		expect(
			screen.queryByTestId("move-migration-progress"),
		).not.toBeInTheDocument();
		await waitFor(() => expect(quadletFetches).toBeGreaterThan(before), {
			timeout: FIND_TIMEOUT,
		});
		expect(screen.queryByTestId("confirm-move-button")).not.toBeInTheDocument();

		await user.click(screen.getByTestId("close-move-report-button"));
		await waitFor(
			() => expect(screen.queryByTestId("modal")).not.toBeInTheDocument(),
			{ timeout: FIND_TIMEOUT },
		);
	});

	it("sends overwriteVolumes only with a checked migration onto an existing target volume", async () => {
		server.use(
			quadletsHandler(() => [web]),
			hostsHandler([source, staging, readOnly, backup]),
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
					hostId: "m-1",
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
			targetHostId: "m-2",
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
			hostsHandler([source, staging, readOnly, backup]),
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
			targetHostId: "m-2",
			mode: "move",
			stop: true,
			force: false,
		});
	});

	it("keeps a migrating partial result open", async () => {
		server.use(
			quadletsHandler(() => [web]),
			hostsHandler([source, staging, readOnly, backup]),
			previewHandler(),
			moveStreamHandler([
				{
					operationId: "op-1",
					type: "complete",
					hostId: "m-1",
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

		// The partial contract holds on the stream transport too: the report
		// slot takes over and the dialog stays open.
		expect(
			await screen.findByTestId("move-partial-message"),
		).toBeInTheDocument();
		expect(screen.queryByTestId("confirm-move-button")).not.toBeInTheDocument();
	});

	it("maps VOLUME_MIGRATION_TARGET_NOT_EMPTY to the overwrite hint and a retryable confirm", async () => {
		server.use(
			quadletsHandler(() => [web]),
			hostsHandler([source, staging, readOnly, backup]),
			previewHandler(),
			moveStreamHandler([
				{
					operationId: "op-1",
					type: "volume",
					hostId: "m-1",
					ref: "pgdata",
					podmanVolume: "pgdata",
					phase: "start",
				},
				{
					operationId: "op-1",
					type: "error",
					hostId: "m-1",
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

	it("offers the transfer-secrets option default-off only for an admin caller with a transferable secret (issue 222)", async () => {
		server.use(
			quadletsHandler(() => [web]),
			hostsHandler([source, staging, readOnly, backup]),
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

	it("hides the transfer-secrets option when nothing is transferable (issue 222)", async () => {
		server.use(
			quadletsHandler(() => [web]),
			hostsHandler([source, staging, readOnly, backup]),
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

		expect(
			screen.queryByTestId("move-transfer-secrets-option"),
		).not.toBeInTheDocument();
	});

	it("hides the transfer-secrets option for a non-admin caller (issue 222)", async () => {
		server.use(
			quadletsHandler(() => [web]),
			hostsHandler([source, staging, readOnly, backup]),
			previewHandler(secretPreview),
			moveHandler(),
		);
		renderWithQuery(
			<QuadletList
				host={hostFixture({ permission: "write", isOwner: false })}
			/>,
			{ router: true },
		);
		const user = userEvent.setup();
		await user.click(
			await screen.findByRole("button", { name: "Move or copy web" }),
		);

		// The preview has resolved (the warning is up); the option stays
		// hidden for a caller without the reveal-secrets capability.
		await screen.findByTestId("move-secret-warning");
		expect(
			screen.queryByTestId("move-transfer-secrets-option"),
		).not.toBeInTheDocument();
	});

	it("sends no transferSecrets when the option is offered but stays unchecked (issue 222)", async () => {
		server.use(
			quadletsHandler(() => [web]),
			hostsHandler([source, staging, readOnly, backup]),
			previewHandler(secretPreview),
			moveHandler(),
		);
		const user = await openMoveDialogReadyToConfirm();

		await user.click(screen.getByTestId("confirm-move-button"));

		await waitFor(() => expect(moveRequests.count).toBe(1));
		expect(moveRequests.body).toEqual({
			targetHostId: "m-2",
			mode: "move",
			stop: true,
			force: false,
		});
	});

	it("sends transferSecrets: true when the option is checked — no transport switch (issue 222)", async () => {
		server.use(
			quadletsHandler(() => [web]),
			hostsHandler([source, staging, readOnly, backup]),
			previewHandler(secretPreview),
			moveHandler(),
		);
		const user = await openMoveDialogReadyToConfirm();

		await user.click(screen.getByTestId("move-transfer-secrets-option"));
		await user.click(screen.getByTestId("confirm-move-button"));

		// The value transfer rides the plain request — no NDJSON, no progress UI.
		await waitFor(() => expect(moveRequests.count).toBe(1));
		expect(moveRequests.body).toEqual({
			targetHostId: "m-2",
			mode: "move",
			stop: true,
			force: false,
			transferSecrets: true,
		});
	});

	it("stays open on a plain transfer's secretTransfer outcomes (issue 222)", async () => {
		server.use(
			quadletsHandler(() => [web]),
			hostsHandler([source, staging, readOnly, backup]),
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
		expect(
			await screen.findByTestId("move-secret-success"),
		).toBeInTheDocument();
		expect(screen.queryByTestId("confirm-move-button")).not.toBeInTheDocument();

		await user.click(screen.getByTestId("close-move-report-button"));
		await waitFor(() =>
			expect(screen.queryByTestId("modal")).not.toBeInTheDocument(),
		);
	});

	it("sends transferSecrets and migrateVolumeData on one stream request (issue 222)", async () => {
		server.use(
			quadletsHandler(() => [web]),
			hostsHandler([source, staging, readOnly, backup]),
			previewHandler(secretPreview),
			moveStreamHandler([
				{
					operationId: "op-1",
					type: "complete",
					hostId: "m-1",
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

		// Both flags ride one request; the terminal event's envelope narrates
		// in place — no progress UI of its own for the secrets.
		await waitFor(() => expect(moveRequests.count).toBe(1));
		expect(moveRequests.body).toEqual({
			targetHostId: "m-2",
			mode: "move",
			stop: true,
			force: false,
			transferSecrets: true,
			migrateVolumeData: true,
		});
		expect(
			await screen.findByTestId("move-migration-success"),
		).toBeInTheDocument();
	});

	it("maps SECRET_TRANSFER_FAILED on the stream error to an actionable message naming the secret (issue 222)", async () => {
		server.use(
			quadletsHandler(() => [web]),
			hostsHandler([source, staging, readOnly, backup]),
			previewHandler(secretPreview),
			moveStreamHandler([
				{
					operationId: "op-1",
					type: "error",
					hostId: "m-1",
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
