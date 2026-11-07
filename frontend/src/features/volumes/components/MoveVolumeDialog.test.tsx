import { MoveVolumeDialog } from "@features/volumes/components/MoveVolumeDialog";
import { useVolumes } from "@features/volumes/hooks/useVolumes";
import type {
	VolumeMovePreview,
	VolumeMoveResult,
} from "@features/volumes/services/volumesApi";
import type { Host } from "@shared/types/api";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { beforeEach, describe, expect, it } from "vitest";
import {
	hostFixture,
	volumeFixture,
	volumeMovePreviewFixture,
	volumeMoveResultFixture,
} from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";
import { liveNdjsonResponse, ndjsonResponse } from "@/test/stream";

// The single-item volume transfer dialog's volume-domain behavior (issue
// 238): the force flag the move body carries and the copy body omits, the
// in-use warning and its checkbox, the NDJSON run with its in-place report
// and both-hosts refetch, the partial report's copy, and the two
// execution-time refusal hints. The dialog skeleton — the target select, the
// mode radio, the preview's pending/error/collision states, the
// referencing-quadlet warning, the stay-open report's footer swap — is
// pinned once in the shared TransferDialog suite, and the target list's
// capability filtering in its `useTransferState` suite, so neither is
// re-pinned here.
//
// The dialog is driven at its own seam: the volumes list no longer mounts it
// (spec 486, ticket #490 retired the row's Move/Copy button in favour of the
// toolbar's batch transfer), so the volume detail page is its only caller and
// the dialog itself is the surface under test.

const volume = volumeFixture();

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
let volumeFetches = 0;

function resetCounters() {
	moveRequests = { count: 0, body: {} };
	volumeFetches = 0;
}

function volumesHandler() {
	return http.get("*/api/hosts/m-1/volumes", () => {
		volumeFetches++;
		return HttpResponse.json([volume]);
	});
}

function hostsHandler(hosts: Host[]) {
	return http.get("*/api/hosts", () => HttpResponse.json(hosts));
}

function previewHandler(overrides: Partial<VolumeMovePreview> = {}) {
	return http.get("*/api/hosts/m-1/volumes/app-data/move-preview", () =>
		HttpResponse.json(volumeMovePreviewFixture(overrides)),
	);
}

// The transfer endpoint always answers NDJSON: progress events, then one
// terminal event. The events travel as raw framed lines — they are the
// volume move domain's own event vocabulary, not the shared stream kit's.
function moveStreamHandler(events: Record<string, unknown>[], live = false) {
	const chunks = events.map((event) => `${JSON.stringify(event)}\n`);
	return http.post(
		"*/api/hosts/m-1/volumes/app-data/move",
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
		hostId: "m-1",
		result,
	};
}

function errorEvent(code: string, message: string) {
	return {
		operationId: "op-1",
		type: "error",
		hostId: "m-1",
		error: { code, message },
	};
}

// The source list's observer: a completed transfer invalidates both hosts'
// volume lists, and the refetch is what tells a mounted list. The probe keeps
// that contract observable without mounting the whole list.
function VolumesProbe({ hostId }: { hostId: string }) {
	useVolumes(hostId);
	return null;
}

function renderDialog() {
	return renderWithQuery(
		<>
			<VolumesProbe hostId={source.id} />
			<MoveVolumeDialog
				host={source}
				volume={volume}
				onClose={() => undefined}
			/>
		</>,
		{ chakra: true },
	);
}

async function openMoveDialog(
	previewOverrides: Partial<VolumeMovePreview> = {},
) {
	server.use(
		volumesHandler(),
		hostsHandler([source, staging, readOnly, backup]),
		previewHandler(previewOverrides),
		moveStreamHandler([completeEvent(volumeMoveResultFixture())]),
	);
	renderDialog();
	expect(await screen.findByTestId("modal")).toBeInTheDocument();
	return userEvent.setup();
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

describe("MoveVolumeDialog", () => {
	beforeEach(resetCounters);

	it("sends the move body with force, and the copy body without it", async () => {
		const user = await openMoveDialogReadyToConfirm();

		await user.click(screen.getByTestId("confirm-move-button"));
		await waitFor(() => expect(moveRequests.count).toBe(1));
		expect(moveRequests.body).toEqual({
			targetHostId: "m-2",
			mode: "move",
			force: false,
		});
	});

	it("warns an in-use source in move mode with the force checkbox, and hides both in copy mode", async () => {
		const user = await openMoveDialogReadyToConfirm({ inUse: true });

		const warning = screen.getByTestId("move-inuse-warning");
		expect(warning).toHaveTextContent("container on this host");
		const forceOption = screen.getByTestId("move-force-option");
		expect(forceOption).not.toBeChecked();

		// Copy mode: no removal, so no in-use warning and no force.
		await user.click(
			screen.getByRole("radio", { name: "Copy (keep this host's volume)" }),
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
			targetHostId: "m-2",
			mode: "move",
			force: true,
		});
	});

	it("runs the transfer over the NDJSON transport, narrates in place, and refetches the source list", async () => {
		server.use(
			volumesHandler(),
			hostsHandler([source, staging, readOnly, backup]),
			previewHandler(),
			moveStreamHandler(
				[
					{
						operationId: "op-1",
						type: "volume",
						hostId: "m-1",
						volume: "app-data",
						phase: "start",
					},
					{
						operationId: "op-1",
						type: "volume",
						hostId: "m-1",
						volume: "app-data",
						phase: "done",
						bytes: 2097152,
					},
					completeEvent(volumeMoveResultFixture()),
				],
				true,
			),
		);
		renderDialog();
		const user = userEvent.setup();
		await waitFor(() =>
			expect(screen.getByTestId("confirm-move-button")).toBeEnabled(),
		);
		const before = volumeFetches;

		await user.click(screen.getByTestId("confirm-move-button"));

		await waitFor(() => expect(moveRequests.count).toBe(1));
		expect(moveRequests.body).toEqual({
			targetHostId: "m-2",
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
	});

	it("keeps the dialog open on a partial result, naming the both-hosts state", async () => {
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
		expect(message).toHaveTextContent("both hosts");
		expect(message).toHaveTextContent("failed to remove volume");
		expect(screen.getByTestId("modal")).toBeInTheDocument();
		expect(screen.queryByTestId("confirm-move-button")).not.toBeInTheDocument();
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
			targetHostId: "m-2",
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
					"Volume app-data already exists on the target host",
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
