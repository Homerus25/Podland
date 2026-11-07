import { ImageList } from "@features/images/components/ImageList";
import type {
	ImageCopyPreview,
	ImageCopyResult,
	ImageSummary,
} from "@features/images/services/imagesApi";
import type { Host } from "@shared/types/api";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http, type JsonBodyType } from "msw";
import { beforeEach, describe, expect, it } from "vitest";
import { errorJson } from "@/test/errors";
import {
	hostFixture,
	imageCopyCompleteEventFixture,
	imageCopyPreviewFixture,
	imageCopyResultFixture,
	imageFixture,
} from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";
import { ndjsonResponse } from "@/test/stream";

// The Images "Copy selected to host" adapter (spec 486, ticket #488) at the
// MSW/HTTP seam. The batch contract itself — one target for the whole
// selection, per-item previews and warnings, sequential execution, the
// per-item report, the succeeded keys — is pinned once in the shared
// MultiTransferDialog suite. This suite keeps adapter smokes only: the
// toolbar button's presence and gate, the row button's removal, the dialog
// opening with the selected images, and the selection clearing only the
// successes.

const alpha = imageFixture({
	id: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	repoTags: ["docker.io/library/nginx:1.27"],
});
const beta = imageFixture({
	id: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	repoTags: ["docker.io/library/redis:7"],
});

const source = hostFixture();
const staging = hostFixture({ id: "m-2", name: "staging-01" });
const backup = hostFixture({ id: "m-4", name: "backup-01" });
const readOnly = hostFixture({
	id: "m-3",
	name: "laptop-01",
	permission: "read",
	isOwner: false,
});

let copyRequests: { imageId: string; body: Record<string, unknown> }[] = [];
let previewRequests: string[] = [];

function resetCounters() {
	copyRequests = [];
	previewRequests = [];
}

function hostsHandler(hosts: Host[]) {
	return http.get("*/api/hosts", () => HttpResponse.json(hosts));
}

function imagesHandler(images: () => ImageSummary[]) {
	return http.get("*/api/hosts/m-1/images", () => HttpResponse.json(images()));
}

function previewHandler(overrides: Partial<ImageCopyPreview> = {}) {
	return http.get(
		"*/api/hosts/m-1/images/:imageId/copy-preview",
		({ params }) => {
			previewRequests.push(String(params.imageId));
			return HttpResponse.json(imageCopyPreviewFixture(overrides));
		},
	);
}

// The per-item copy endpoint: the existing single-image NDJSON route,
// unchanged — progress events, then one terminal event.
function copyHandler(
	result: (imageId: string) => ImageCopyResult | HttpResponse<JsonBodyType>,
) {
	return http.post(
		"*/api/hosts/m-1/images/:imageId/copy",
		async ({ request, params }) => {
			const imageId = String(params.imageId);
			copyRequests.push({
				imageId,
				body: (await request.json()) as Record<string, unknown>,
			});
			const answer = result(imageId);
			if (answer instanceof HttpResponse) return answer;
			return ndjsonResponse(
				[],
				[`${JSON.stringify(imageCopyCompleteEventFixture(answer))}\n`],
			);
		},
	);
}

function renderImageList(permission: "read" | "write" | "admin" = "admin") {
	return renderWithQuery(
		<ImageList
			host={hostFixture({ permission, isOwner: permission === "admin" })}
		/>,
		// The row navigates on click, so the list renders inside a router.
		{ chakra: true, router: true },
	);
}

// The batch confirm unlocks only once the hosts list and every item's
// target-scoped preview have resolved; under a loaded worker that can
// outlast waitFor's one-second default, so the gate gets a wider window.
async function waitForBatchConfirmEnabled() {
	await waitFor(
		() =>
			expect(screen.getByTestId("confirm-multi-transfer-button")).toBeEnabled(),
		{ timeout: 3000 },
	);
}

// The row checkbox's accessible name is the row's stable key (the shared
// selection contract derives it from `getKey`), which for an image is its
// opaque id — the tag is the display label, not the identity.
async function selectImages(...ids: string[]) {
	const user = userEvent.setup();
	for (const id of ids) {
		await user.click(screen.getByRole("checkbox", { name: `Select ${id}` }));
	}
	return user;
}

describe("ImageList copy selected", () => {
	beforeEach(resetCounters);

	it("renders Copy selected to host permanently for a manage-images caller, disabled on an empty selection", async () => {
		server.use(imagesHandler(() => [alpha, beta]));
		renderImageList();
		await screen.findByText("docker.io/library/nginx:1.27");

		// Permanently rendered — the button disables itself rather than
		// appearing and disappearing with the selection.
		const button = screen.getByTestId("copy-selected-images-button");
		expect(button).toBeDisabled();

		await selectImages(alpha.id);
		expect(screen.getByTestId("copy-selected-images-button")).toBeEnabled();
	});

	it("is absent below manage-images, where the permission hint replaces the toolbar", async () => {
		server.use(imagesHandler(() => [alpha, beta]));
		renderImageList("read");
		await screen.findByText("docker.io/library/nginx:1.27");

		expect(
			screen.queryByTestId("copy-selected-images-button"),
		).not.toBeInTheDocument();
		expect(screen.getByTestId("images-permission-hint")).toBeInTheDocument();
	});

	it("renders no row Copy to host button any more, keeping Download", async () => {
		server.use(imagesHandler(() => [alpha, beta]));
		renderImageList();

		const rows = await screen.findAllByTestId("image-row");
		expect(rows).toHaveLength(2);
		for (const row of rows) {
			expect(
				within(row).queryByTestId("copy-image-button"),
			).not.toBeInTheDocument();
			expect(
				within(row).queryByRole("button", { name: /copy .* to another host/i }),
			).not.toBeInTheDocument();
			expect(
				within(row).getByRole("button", { name: /^download /i }),
			).toBeInTheDocument();
		}
	});

	it("opens one copy-only dialog naming the selected images, sending nothing yet", async () => {
		server.use(
			imagesHandler(() => [alpha, beta]),
			hostsHandler([source, staging, readOnly, backup]),
			previewHandler(),
			copyHandler((imageId) => imageCopyResultFixture({ id: imageId })),
		);
		renderImageList();
		await screen.findByText("docker.io/library/nginx:1.27");
		const user = await selectImages(alpha.id, beta.id);

		await user.click(screen.getByTestId("copy-selected-images-button"));

		const modal = await screen.findByTestId("modal");
		expect(modal).toHaveTextContent("Copy 2 images");
		const blocks = screen.getAllByTestId("multi-transfer-item");
		expect(blocks).toHaveLength(2);
		expect(
			within(blocks[0]).getByText("docker.io/library/nginx:1.27"),
		).toBeInTheDocument();
		expect(
			within(blocks[1]).getByText("docker.io/library/redis:7"),
		).toBeInTheDocument();
		// One target for the whole batch, and no mode radio: an image copy
		// is copy-only.
		expect(screen.getAllByTestId("move-target-select")).toHaveLength(1);
		expect(screen.queryByTestId("move-mode-radio")).not.toBeInTheDocument();
		expect(copyRequests).toHaveLength(0);
	});

	it("renders the already-present note under the image it applies to", async () => {
		server.use(
			imagesHandler(() => [alpha, beta]),
			hostsHandler([source, staging, readOnly, backup]),
			// The target already holds the first image only.
			http.get("*/api/hosts/m-1/images/:imageId/copy-preview", ({ params }) =>
				HttpResponse.json(
					imageCopyPreviewFixture({
						present: String(params.imageId) === alpha.id,
					}),
				),
			),
			copyHandler((imageId) => imageCopyResultFixture({ id: imageId })),
		);
		renderImageList();
		await screen.findByText("docker.io/library/nginx:1.27");
		const user = await selectImages(alpha.id, beta.id);

		await user.click(screen.getByTestId("copy-selected-images-button"));

		const blocks = await screen.findAllByTestId("multi-transfer-item");
		expect(
			within(blocks[0]).getByTestId("copy-already-present"),
		).toHaveTextContent("staging-01");
		expect(
			within(blocks[1]).queryByTestId("copy-already-present"),
		).not.toBeInTheDocument();
	});

	it("clears only the successes from the selection after a run", async () => {
		server.use(
			imagesHandler(() => [alpha, beta]),
			hostsHandler([source, staging, readOnly, backup]),
			previewHandler(),
			// The second image's copy is refused by the server: a failed item,
			// not a failed batch.
			copyHandler((imageId) =>
				imageId === beta.id
					? HttpResponse.json(
							errorJson(
								"IMAGE_TRANSFER_TOO_LARGE",
								"Image exceeds the transfer bound",
							),
							{ status: 413 },
						)
					: imageCopyResultFixture({ id: imageId }),
			),
		);
		renderImageList();
		await screen.findByText("docker.io/library/nginx:1.27");
		const user = await selectImages(alpha.id, beta.id);

		await user.click(screen.getByTestId("copy-selected-images-button"));
		await screen.findByTestId("modal");
		await waitForBatchConfirmEnabled();

		await user.click(screen.getByTestId("confirm-multi-transfer-button"));

		// The report is the run's completion signal; what it says is the
		// shared dialog's contract, pinned in its own suite.
		await screen.findByTestId("multi-transfer-report");

		// The succeeded image leaves the selection; the failed one stays
		// selected for the retry.
		await user.click(screen.getByTestId("close-multi-transfer-report-button"));
		await waitFor(() =>
			expect(screen.queryByTestId("modal")).not.toBeInTheDocument(),
		);
		expect(
			screen.getByRole("checkbox", { name: `Select ${alpha.id}` }),
		).not.toBeChecked();
		expect(
			screen.getByRole("checkbox", { name: `Select ${beta.id}` }),
		).toBeChecked();
	});
});
