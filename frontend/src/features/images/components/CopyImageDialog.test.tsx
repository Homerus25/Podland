import { CopyImageDialog } from "@features/images/components/CopyImageDialog";
import { useImages } from "@features/images/hooks/useImages";
import type { ImageCopyPreview } from "@features/images/services/imagesApi";
import type { Host } from "@shared/types/api";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { beforeEach, describe, expect, it } from "vitest";
import {
	hostFixture,
	imageCopyCompleteEventFixture,
	imageCopyErrorEventFixture,
	imageCopyEventFixture,
	imageCopyPreviewFixture,
	imageCopyResultFixture,
	imageFixture,
} from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";
import { liveNdjsonResponse, ndjsonResponse } from "@/test/stream";

// The single-image copy dialog's domain assertions (issue 376), mirroring the
// volumes move suite: the already-present fact as information, the
// target-scoped preview refetch, the copy-only body, the live progress and
// stay-open report, the both-hosts refetch, and the too-large hint. The
// skeleton behavior (the hidden radio, the Copy confirm) is pinned once in
// the TransferDialog preset's suite.
//
// The dialog is rendered directly here, not through a list: spec 486 (ticket
// #488) retired the images list's row button, so the single-image copy is
// reached from the image detail page — whose own suite pins the header
// action's gate and wiring — and the batch copy from the toolbar. This suite
// keeps the dialog's domain, which neither of those re-pins.

const image = imageFixture();
const encodedImageId = encodeURIComponent(image.id);

const source = hostFixture();
const staging = hostFixture({ id: "m-2", name: "staging-01" });
const backup = hostFixture({ id: "m-4", name: "backup-01" });
const readOnly = hostFixture({
	id: "m-3",
	name: "laptop-01",
	permission: "read",
	isOwner: false,
});

let copyRequests: { count: number; body: Record<string, unknown> } = {
	count: 0,
	body: {},
};
let previewTargets: string[] = [];
let sourceImageFetches = 0;
let targetImageFetches = 0;

function resetCounters() {
	copyRequests = { count: 0, body: {} };
	previewTargets = [];
	sourceImageFetches = 0;
	targetImageFetches = 0;
}

function imagesHandler(hostId: "m-1" | "m-2") {
	return http.get(`*/api/hosts/${hostId}/images`, () => {
		if (hostId === "m-1") sourceImageFetches++;
		else targetImageFetches++;
		return HttpResponse.json(hostId === "m-1" ? [image] : []);
	});
}

function hostsHandler(hosts: Host[]) {
	return http.get("*/api/hosts", () => HttpResponse.json(hosts));
}

function previewHandler(overrides: Partial<ImageCopyPreview> = {}) {
	return http.get(
		`*/api/hosts/m-1/images/${encodedImageId}/copy-preview`,
		({ request }) => {
			previewTargets.push(
				new URL(request.url).searchParams.get("targetHostId") ?? "",
			);
			return HttpResponse.json(imageCopyPreviewFixture(overrides));
		},
	);
}

// The copy endpoint always answers NDJSON: progress events, then one
// terminal event. The events travel as raw framed lines built by the
// shared image-copy event fixtures — they are the image copy domain's
// own event vocabulary, not the shared stream kit's.
function copyStreamHandler(events: Record<string, unknown>[], live = false) {
	const chunks = events.map((event) => `${JSON.stringify(event)}\n`);
	return http.post(
		`*/api/hosts/m-1/images/${encodedImageId}/copy`,
		async ({ request }) => {
			copyRequests.count++;
			copyRequests.body = (await request.json()) as Record<string, unknown>;
			return live ? liveNdjsonResponse(chunks, 50) : ndjsonResponse([], chunks);
		},
	);
}

// Neither host's image list is rendered by the dialog; the probes mount
// both queries so the both-hosts refetch is observable through MSW.
function ImagesProbe() {
	useImages("m-1");
	useImages("m-2");
	return null;
}

function renderCopyDialog() {
	renderWithQuery(
		<>
			<CopyImageDialog host={source} image={image} onClose={() => {}} />
			<ImagesProbe />
		</>,
		{ chakra: true, router: true },
	);
}

// The confirm unlocks only once the hosts list and the target-scoped preview
// have both resolved; under a loaded worker that can outlast waitFor's
// one-second default, so the gate gets a wider window.
async function waitForConfirmEnabled() {
	await waitFor(
		() => expect(screen.getByTestId("confirm-move-button")).toBeEnabled(),
		{ timeout: 3000 },
	);
}

// Ready-to-confirm state: the preview resolved (the default fixture says
// the target does not hold the image), so confirm unlocks once it arrives.
async function renderCopyDialogReadyToConfirm(
	previewOverrides: Partial<ImageCopyPreview> = {},
) {
	server.use(
		imagesHandler("m-1"),
		imagesHandler("m-2"),
		hostsHandler([source, staging, readOnly, backup]),
		previewHandler(previewOverrides),
		copyStreamHandler([
			imageCopyCompleteEventFixture(imageCopyResultFixture()),
		]),
	);
	renderCopyDialog();
	const user = userEvent.setup();
	await waitForConfirmEnabled();
	return user;
}

describe("CopyImageDialog", () => {
	beforeEach(resetCounters);

	it("renders the preview's already-present fact as information, never a block", async () => {
		await renderCopyDialogReadyToConfirm({ present: true });

		const present = screen.getByTestId("copy-already-present");
		expect(present).toHaveTextContent("staging-01");
		expect(present).toHaveTextContent("already holds this image");
		expect(screen.getByTestId("confirm-move-button")).toBeEnabled();
		expect(copyRequests.count).toBe(0);
	});

	it("refetches the preview when the target changes", async () => {
		const user = await renderCopyDialogReadyToConfirm();

		await user.selectOptions(screen.getByTestId("move-target-select"), "m-4");

		await waitFor(() => expect(previewTargets).toEqual(["m-2", "m-4"]));
	});

	it("sends the copy body — the target only, no mode, no force", async () => {
		const user = await renderCopyDialogReadyToConfirm();

		await user.click(screen.getByTestId("confirm-move-button"));

		await waitFor(() => expect(copyRequests.count).toBe(1));
		expect(copyRequests.body).toEqual({ targetHostId: "m-2" });
	});

	it("streams with live progress, narrates the stay-open report, and refetches both hosts' image lists", async () => {
		server.use(
			imagesHandler("m-1"),
			imagesHandler("m-2"),
			hostsHandler([source, staging, readOnly, backup]),
			previewHandler(),
			copyStreamHandler(
				[
					imageCopyEventFixture({ phase: "start" }),
					imageCopyEventFixture({ phase: "bytes", bytes: 1048576 }),
					imageCopyEventFixture({ phase: "done", bytes: 2097152 }),
					imageCopyCompleteEventFixture(imageCopyResultFixture()),
				],
				true,
			),
		);
		renderCopyDialog();
		const user = userEvent.setup();
		await waitForConfirmEnabled();
		const beforeSource = sourceImageFetches;
		const beforeTarget = targetImageFetches;

		await user.click(screen.getByTestId("confirm-move-button"));

		await waitFor(() => expect(copyRequests.count).toBe(1));
		expect(copyRequests.body).toEqual({ targetHostId: "m-2" });

		// Progress rendered while the stream ran, then the terminal report
		// narrated in place instead of closing — the byte count is the
		// answer's point.
		expect(await screen.findByTestId("move-progress")).toBeInTheDocument();
		const report = await screen.findByTestId("move-success-report");
		expect(report).toHaveTextContent("Copied");
		expect(report).toHaveTextContent("docker.io/library/nginx:1.27");
		expect(report).toHaveTextContent("staging-01");
		expect(report).toHaveTextContent("2.1 MB");
		const names = within(report).getByTestId("copy-loaded-names");
		expect(names).toHaveTextContent("docker.io/library/nginx:1.27");
		expect(screen.queryByTestId("confirm-move-button")).not.toBeInTheDocument();

		// The transfer touched two hosts: both image lists refetch from
		// the server's answer, not from closure assumptions.
		await waitFor(() => {
			expect(sourceImageFetches).toBeGreaterThan(beforeSource);
			expect(targetImageFetches).toBeGreaterThan(beforeTarget);
		});
	});

	it("narrates the too-large refusal as a hint in the error region", async () => {
		const user = await renderCopyDialogReadyToConfirm();
		server.use(
			copyStreamHandler([
				imageCopyErrorEventFixture(
					"IMAGE_TRANSFER_TOO_LARGE",
					"Image docker.io/library/nginx:1.27 exceeds the transfer bound",
				),
			]),
		);

		await user.click(screen.getByTestId("confirm-move-button"));

		expect(
			await screen.findByText("IMAGE_TRANSFER_TOO_LARGE"),
		).toBeInTheDocument();
		const hint = screen.getByTestId("copy-too-large-hint");
		expect(hint).toHaveTextContent("transfer bound");
		expect(screen.getByTestId("modal")).toBeInTheDocument();
	});

	it("narrates a completed copy without a loaded-names block when the load reported none", async () => {
		server.use(
			hostsHandler([source, staging, readOnly, backup]),
			previewHandler(),
			copyStreamHandler([
				imageCopyCompleteEventFixture(imageCopyResultFixture({ names: [] })),
			]),
		);
		renderCopyDialog();
		const user = userEvent.setup();
		await waitForConfirmEnabled();

		await user.click(screen.getByTestId("confirm-move-button"));

		const report = await screen.findByTestId("move-success-report");
		expect(report).toHaveTextContent("Copied");
		expect(
			within(report).queryByTestId("copy-loaded-names"),
		).not.toBeInTheDocument();
	});
});
