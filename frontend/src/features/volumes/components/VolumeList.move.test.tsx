import { VolumeList } from "@features/volumes/components/VolumeList";
import type {
	VolumeMovePreview,
	VolumeMoveResult,
	VolumeSummary,
} from "@features/volumes/services/volumesApi";
import type { Machine } from "@shared/types/api";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { beforeEach, describe, expect, it } from "vitest";
import { errorJson } from "@/test/errors";
import {
	machineFixture,
	volumeFixture,
	volumeMovePreviewFixture,
	volumeMoveResultFixture,
} from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";
import { liveNdjsonResponse, ndjsonResponse } from "@/test/stream";

const volume = volumeFixture();

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
let volumeFetches = 0;

function resetCounters() {
	moveRequests = { count: 0, body: {} };
	previewTargets = [];
	volumeFetches = 0;
}

function volumesHandler(volumes: () => VolumeSummary[]) {
	return http.get("*/api/machines/m-1/volumes", () => {
		volumeFetches++;
		return HttpResponse.json(volumes());
	});
}

function machinesHandler(machines: Machine[]) {
	return http.get("*/api/machines", () => HttpResponse.json(machines));
}

function previewHandler(overrides: Partial<VolumeMovePreview> = {}) {
	return http.get(
		"*/api/machines/m-1/volumes/app-data/move-preview",
		({ request }) => {
			previewTargets.push(
				new URL(request.url).searchParams.get("targetMachineId") ?? "",
			);
			return HttpResponse.json(volumeMovePreviewFixture(overrides));
		},
	);
}

// The transfer endpoint always answers NDJSON: progress events, then one
// terminal event. The events travel as raw framed lines — they are the
// volume move domain's own event vocabulary, not the shared stream kit's.
function moveStreamHandler(events: Record<string, unknown>[], live = false) {
	const chunks = events.map((event) => `${JSON.stringify(event)}\n`);
	return http.post(
		"*/api/machines/m-1/volumes/app-data/move",
		async ({ request }) => {
			moveRequests.count++;
			moveRequests.body = (await request.json()) as Record<string, unknown>;
			return live ? liveNdjsonResponse(chunks, 50) : ndjsonResponse([], chunks);
		},
	);
}

function completeEvent(result: VolumeMoveResult) {
	return {
		operationId: "op-1",
		type: "complete",
		machineId: "m-1",
		result,
	};
}

function errorEvent(code: string, message: string) {
	return {
		operationId: "op-1",
		type: "error",
		machineId: "m-1",
		error: { code, message },
	};
}

async function openMoveDialog(
	previewOverrides: Partial<VolumeMovePreview> = {},
) {
	server.use(
		volumesHandler(() => [volume]),
		machinesHandler([source, staging, readOnly, backup]),
		previewHandler(previewOverrides),
		moveStreamHandler([completeEvent(volumeMoveResultFixture())]),
	);
	renderWithQuery(<VolumeList machine={source} />, {
		chakra: true,
		router: true,
	});
	const user = userEvent.setup();
	await user.click(
		await screen.findByRole("button", { name: "Move or copy app-data" }),
	);
	expect(await screen.findByTestId("modal")).toBeInTheDocument();
	return user;
}

// Ready-to-confirm state: the preview resolved (the default fixture is
// collision-free and idle), so confirm unlocks once it arrives.
async function openMoveDialogReadyToConfirm(
	previewOverrides: Partial<VolumeMovePreview> = {},
) {
	const user = await openMoveDialog(previewOverrides);
	await waitFor(() =>
		expect(screen.getByTestId("confirm-move-button")).toBeEnabled(),
	);
	return user;
}

describe("VolumeList move/copy", () => {
	beforeEach(resetCounters);

	it("hides the Move/Copy action below the manage-volumes capability", async () => {
		server.use(volumesHandler(() => [volume]));
		renderWithQuery(
			<VolumeList
				machine={machineFixture({ permission: "read", isOwner: false })}
			/>,
			{ chakra: true, router: true },
		);

		expect(await screen.findByTestId("volume-row")).toBeInTheDocument();
		expect(
			screen.queryByRole("button", { name: "Move or copy app-data" }),
		).not.toBeInTheDocument();
	});

	it("offers one dialog listing only manageable machines, source excluded", async () => {
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
		const user = await openMoveDialogReadyToConfirm();

		await user.selectOptions(screen.getByTestId("move-target-select"), "m-4");

		await waitFor(() => expect(previewTargets).toEqual(["m-2", "m-4"]));
	});

	it("sends the move body with force, and the copy body without it", async () => {
		const user = await openMoveDialogReadyToConfirm();

		await user.click(screen.getByTestId("confirm-move-button"));
		await waitFor(() => expect(moveRequests.count).toBe(1));
		expect(moveRequests.body).toEqual({
			targetMachineId: "m-2",
			mode: "move",
			force: false,
		});

		// Copy never removes anything: force is move-mode vocabulary.
		await user.click(screen.getByTestId("close-move-report-button"));
		await waitFor(() =>
			expect(screen.queryByTestId("modal")).not.toBeInTheDocument(),
		);
		await user.click(
			screen.getByRole("button", { name: "Move or copy app-data" }),
		);
		expect(await screen.findByTestId("modal")).toBeInTheDocument();
		await user.click(
			screen.getByRole("radio", { name: "Copy (keep this machine's volume)" }),
		);
		await waitFor(() =>
			expect(screen.getByTestId("confirm-move-button")).toBeEnabled(),
		);
		await user.click(screen.getByTestId("confirm-move-button"));
		await waitFor(() => expect(moveRequests.count).toBe(2));
		expect(moveRequests.body).toEqual({
			targetMachineId: "m-2",
			mode: "copy",
		});
	});

	it("disables confirm on a collision with a clear message", async () => {
		await openMoveDialog({ collision: true });

		expect(
			await screen.findByTestId("move-collision-warning"),
		).toHaveTextContent("already exists on staging-01");
		expect(screen.getByTestId("confirm-move-button")).toBeDisabled();
		expect(moveRequests.count).toBe(0);
	});

	it("keeps confirm disabled while the preview is pending", async () => {
		server.use(
			volumesHandler(() => [volume]),
			machinesHandler([source, staging, readOnly, backup]),
			http.get(
				"*/api/machines/m-1/volumes/app-data/move-preview",
				() => new Promise<Response>(() => {}),
			),
		);
		renderWithQuery(<VolumeList machine={source} />, {
			chakra: true,
			router: true,
		});
		const user = userEvent.setup();
		await user.click(
			await screen.findByRole("button", { name: "Move or copy app-data" }),
		);

		expect(
			await screen.findByTestId("move-preview-pending"),
		).toBeInTheDocument();
		expect(screen.getByTestId("confirm-move-button")).toBeDisabled();
	});

	it("shows a failed preview and keeps confirm disabled", async () => {
		server.use(
			volumesHandler(() => [volume]),
			machinesHandler([source, staging, readOnly, backup]),
			http.get("*/api/machines/m-1/volumes/app-data/move-preview", () =>
				HttpResponse.json(
					errorJson("ACCESS_DENIED", "Access denied", "req-71"),
					{ status: 403 },
				),
			),
		);
		renderWithQuery(<VolumeList machine={source} />, {
			chakra: true,
			router: true,
		});
		const user = userEvent.setup();
		await user.click(
			await screen.findByRole("button", { name: "Move or copy app-data" }),
		);

		expect(await screen.findByText("ACCESS_DENIED")).toBeInTheDocument();
		expect(screen.getByTestId("confirm-move-button")).toBeDisabled();
	});

	it("warns an in-use source in move mode with the force checkbox, and hides both in copy mode", async () => {
		const user = await openMoveDialogReadyToConfirm({ inUse: true });

		const warning = screen.getByTestId("move-inuse-warning");
		expect(warning).toHaveTextContent("container on this machine");
		const forceOption = screen.getByTestId("move-force-option");
		expect(forceOption).not.toBeChecked();

		// Copy mode: no removal, so no in-use warning and no force.
		await user.click(
			screen.getByRole("radio", { name: "Copy (keep this machine's volume)" }),
		);
		expect(screen.queryByTestId("move-inuse-warning")).not.toBeInTheDocument();
		expect(screen.queryByTestId("move-force-option")).not.toBeInTheDocument();
	});

	it("sends force=true when the checkbox is checked", async () => {
		const user = await openMoveDialogReadyToConfirm({ inUse: true });

		await user.click(screen.getByTestId("move-force-option"));
		await user.click(screen.getByTestId("confirm-move-button"));

		await waitFor(() => expect(moveRequests.count).toBe(1));
		expect(moveRequests.body).toEqual({
			targetMachineId: "m-2",
			mode: "move",
			force: true,
		});
	});

	it("lists referencing quadlets as a warning, with the dangling note in move mode only", async () => {
		const user = await openMoveDialogReadyToConfirm({
			referencingQuadlets: [
				{ name: "web.container" },
				{ name: "db.container" },
			],
		});

		const warning = screen.getByTestId("move-referencing-warning");
		expect(warning).toHaveTextContent("web.container");
		expect(warning).toHaveTextContent("db.container");
		expect(warning).toHaveTextContent("keep pointing at prod-web-01");
		expect(warning).toHaveTextContent("references dangle");

		await user.click(
			screen.getByRole("radio", { name: "Copy (keep this machine's volume)" }),
		);
		expect(
			screen.getByTestId("move-referencing-warning"),
		).not.toHaveTextContent("references dangle");
	});

	it("runs the transfer over the NDJSON transport, narrates in place, and refetches the source list", async () => {
		server.use(
			volumesHandler(() => [volume]),
			machinesHandler([source, staging, readOnly, backup]),
			previewHandler(),
			moveStreamHandler(
				[
					{
						operationId: "op-1",
						type: "volume",
						machineId: "m-1",
						volume: "app-data",
						phase: "start",
					},
					{
						operationId: "op-1",
						type: "volume",
						machineId: "m-1",
						volume: "app-data",
						phase: "done",
						bytes: 2097152,
					},
					completeEvent(volumeMoveResultFixture()),
				],
				true,
			),
		);
		renderWithQuery(<VolumeList machine={source} />, {
			chakra: true,
			router: true,
		});
		const user = userEvent.setup();
		await user.click(
			await screen.findByRole("button", { name: "Move or copy app-data" }),
		);
		await waitFor(() =>
			expect(screen.getByTestId("confirm-move-button")).toBeEnabled(),
		);
		const before = volumeFetches;

		await user.click(screen.getByTestId("confirm-move-button"));

		await waitFor(() => expect(moveRequests.count).toBe(1));
		expect(moveRequests.body).toEqual({
			targetMachineId: "m-2",
			mode: "move",
			force: false,
		});

		// Progress rendered while the stream ran, then the terminal report
		// narrated in place instead of closing — the byte count is the
		// answer's point.
		expect(await screen.findByTestId("move-progress")).toBeInTheDocument();
		const report = await screen.findByTestId("move-success-report");
		expect(report).toHaveTextContent("Moved");
		expect(report).toHaveTextContent("app-data");
		expect(report).toHaveTextContent("staging-01");
		expect(report).toHaveTextContent("2.1 MB");
		expect(screen.queryByTestId("confirm-move-button")).not.toBeInTheDocument();
		await waitFor(() => expect(volumeFetches).toBeGreaterThan(before));

		await user.click(screen.getByTestId("close-move-report-button"));
		await waitFor(() =>
			expect(screen.queryByTestId("modal")).not.toBeInTheDocument(),
		);
	});

	it("keeps the dialog open on a partial result, naming the both-machines state", async () => {
		const user = await openMoveDialogReadyToConfirm();
		server.use(
			moveStreamHandler([
				completeEvent(
					volumeMoveResultFixture({
						status: "partial",
						error: "failed to remove volume: volume is in use",
					}),
				),
			]),
		);

		await user.click(screen.getByTestId("confirm-move-button"));

		const message = await screen.findByTestId("move-partial-message");
		expect(message).toHaveTextContent("both machines");
		expect(message).toHaveTextContent("failed to remove volume");
		expect(screen.getByTestId("modal")).toBeInTheDocument();
		expect(screen.queryByTestId("confirm-move-button")).not.toBeInTheDocument();

		await user.click(screen.getByTestId("close-move-report-button"));
		await waitFor(() =>
			expect(screen.queryByTestId("modal")).not.toBeInTheDocument(),
		);
	});

	it("maps a VOLUME_IN_USE refusal to the force hint, surfaces the checkbox, and retries with force", async () => {
		const user = await openMoveDialogReadyToConfirm();
		server.use(
			moveStreamHandler([
				errorEvent("VOLUME_IN_USE", "Volume is in use by a container"),
			]),
		);

		await user.click(screen.getByTestId("confirm-move-button"));

		expect(await screen.findByText("VOLUME_IN_USE")).toBeInTheDocument();
		expect(
			screen.getByText("Volume is in use by a container"),
		).toBeInTheDocument();
		expect(screen.getByTestId("move-inuse-hint")).toHaveTextContent("Force");
		// The stale preview said not-in-use, so the refusal itself must
		// surface the force checkbox the hint names.
		const forceOption = screen.getByTestId("move-force-option");
		expect(forceOption).not.toBeChecked();
		expect(screen.getByTestId("confirm-move-button")).toBeEnabled();
		expect(screen.getByTestId("modal")).toBeInTheDocument();

		// The instructed way out works without reopening the dialog.
		await user.click(forceOption);
		await user.click(screen.getByTestId("confirm-move-button"));
		await waitFor(() => expect(moveRequests.count).toBe(2));
		expect(moveRequests.body).toEqual({
			targetMachineId: "m-2",
			mode: "move",
			force: true,
		});
	});

	it("maps a VOLUME_ALREADY_EXISTS refusal to the collision hint", async () => {
		const user = await openMoveDialogReadyToConfirm();
		server.use(
			moveStreamHandler([
				errorEvent(
					"VOLUME_ALREADY_EXISTS",
					"Volume app-data already exists on the target machine",
				),
			]),
		);

		await user.click(screen.getByTestId("confirm-move-button"));

		expect(
			await screen.findByText("VOLUME_ALREADY_EXISTS"),
		).toBeInTheDocument();
		const hint = screen.getByTestId("move-collision-hint");
		expect(hint).toHaveTextContent("staging-01");
		expect(hint).toHaveTextContent("remove it there first");
		expect(screen.getByTestId("confirm-move-button")).toBeEnabled();
	});
});
