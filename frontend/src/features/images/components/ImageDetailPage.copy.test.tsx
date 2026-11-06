import { ImageDetailPage } from "@features/images/components/ImageDetailPage";
import type { ImageCopyPreview } from "@features/images/services/imagesApi";
import type { Host } from "@shared/types/api";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it } from "vitest";
import {
	hostFixture,
	imageCopyCompleteEventFixture,
	imageCopyPreviewFixture,
	imageCopyResultFixture,
	imageDetailFixture,
} from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";
import { ndjsonResponse } from "@/test/stream";

// The image detail page's Copy to host action (spec 477, ticket #481): the
// header-actions slot, the manage-images gate's tri-state wiring, and the
// shared copy dialog opened with the image fixed. The gate's classification
// is pinned once in the hook's suite and the dialog's own behavior once in
// the TransferDialog preset's suite, so this suite keeps adapter wiring
// only — allowed renders the control, denied the shared hint, pending
// neither, a click opens the dialog on this image, and the confirm sends
// the row's request shape and refreshes the page's overview.

const image = imageDetailFixture();
const encodedImageId = encodeURIComponent(image.id);

const source = hostFixture();
const staging = hostFixture({ id: "m-2", name: "staging-01" });
const readOnly = hostFixture({
	id: "m-3",
	name: "laptop-01",
	permission: "read",
	isOwner: false,
});

let previewTargets: string[] = [];
let detailFetches = 0;
let copyRequests: { count: number; body: Record<string, unknown> } = {
	count: 0,
	body: {},
};

function resetCounters() {
	previewTargets = [];
	detailFetches = 0;
	copyRequests = { count: 0, body: {} };
}

// The page reads the host's per-caller facts for the manage gate.
function mockHost(permission: "read" | "write" | "admin" = "admin") {
	server.use(
		http.get("*/api/hosts/m-1", () =>
			HttpResponse.json(
				hostFixture({ permission, isOwner: permission === "admin" }),
			),
		),
	);
}

function mockImageDetail() {
	server.use(
		http.get("*/api/hosts/m-1/images/:imageId", () => {
			detailFetches++;
			return HttpResponse.json(image);
		}),
	);
}

// The dialog's transfer state fetches the hosts list itself to filter the
// targets by the caller's capability.
function mockHosts(hosts: Host[]) {
	server.use(http.get("*/api/hosts", () => HttpResponse.json(hosts)));
}

function mockPreview(overrides: Partial<ImageCopyPreview> = {}) {
	server.use(
		http.get(
			`*/api/hosts/m-1/images/${encodedImageId}/copy-preview`,
			({ request }) => {
				previewTargets.push(
					new URL(request.url).searchParams.get("targetHostId") ?? "",
				);
				return HttpResponse.json(imageCopyPreviewFixture(overrides));
			},
		),
	);
}

// The copy endpoint always answers NDJSON: progress events, then one
// terminal event — the row's handler shape, so the page's request is
// asserted against the same contract.
function copyStreamHandler(events: Record<string, unknown>[]) {
	const chunks = events.map((event) => `${JSON.stringify(event)}\n`);
	return http.post(
		`*/api/hosts/m-1/images/${encodedImageId}/copy`,
		async ({ request }) => {
			copyRequests.count++;
			copyRequests.body = (await request.json()) as Record<string, unknown>;
			return ndjsonResponse([], chunks);
		},
	);
}

function renderDetailPage() {
	return renderWithQuery(
		<Routes>
			<Route
				path="/hosts/:hostId/images/:imageId"
				element={<ImageDetailPage />}
			/>
		</Routes>,
		{
			chakra: true,
			router: {
				initialEntries: [`/hosts/m-1/images/${encodedImageId}`],
			},
		},
	);
}

describe("ImageDetailPage copy to host", () => {
	beforeEach(resetCounters);

	it("renders the Copy to host action for a manage-capable caller", async () => {
		mockHost("write");
		mockImageDetail();
		renderDetailPage();

		expect(await screen.findByTestId("copy-image-button")).toHaveTextContent(
			"Copy to host",
		);
		expect(
			screen.queryByTestId("copy-image-permission-hint"),
		).not.toBeInTheDocument();
	});

	it("renders the shared write hint instead of the action for a read-level caller", async () => {
		mockHost("read");
		mockImageDetail();
		renderDetailPage();

		// The hint is the Remove cascade's: Copy to host answers the same
		// capability, so it carries none of its own (the one-hint rule).
		const hint = await screen.findByTestId("images-permission-hint");
		expect(hint).toHaveAttribute("data-requirement", "write");
		expect(hint).toHaveTextContent(
			"You have read-only access to this host. Ask an owner or admin for write access to manage its images.",
		);
		expect(
			within(screen.getByTestId("detail-page-header-actions")).getAllByTestId(
				/permission-hint$/,
			),
		).toHaveLength(1);
		expect(
			screen.queryByTestId("copy-image-permission-hint"),
		).not.toBeInTheDocument();
		expect(screen.queryByTestId("copy-image-button")).not.toBeInTheDocument();
	});

	it("renders neither the action nor the hint while the gate is pending", async () => {
		mockImageDetail();
		let release: () => void = () => {};
		const gate = new Promise<void>((resolve) => {
			release = resolve;
		});
		server.use(
			http.get("*/api/hosts/m-1", async () => {
				await gate;
				return HttpResponse.json(hostFixture({ permission: "write" }));
			}),
		);
		renderDetailPage();

		await screen.findByTestId("detail-page-overview");
		expect(screen.queryByTestId("copy-image-button")).not.toBeInTheDocument();
		expect(
			screen.queryByTestId("images-permission-hint"),
		).not.toBeInTheDocument();

		release();
		await screen.findByTestId("copy-image-button");
	});

	it("opens the shared copy dialog with this image fixed", async () => {
		mockHost("write");
		mockImageDetail();
		mockHosts([source, staging, readOnly]);
		mockPreview();
		renderDetailPage();
		const user = userEvent.setup();

		await user.click(await screen.findByTestId("copy-image-button"));

		expect(await screen.findByTestId("modal")).toBeInTheDocument();
		// The dialog's target list is the capability-filtered hosts list with
		// the source excluded — the shared transfer state's own filtering.
		const select = screen.getByTestId("move-target-select");
		const names = within(select)
			.getAllByRole("option")
			.map((option) => option.textContent);
		expect(names).toEqual(["staging-01"]);
		// The preview is scoped to this page's image, not to a row's.
		await waitFor(() => expect(previewTargets).toEqual(["m-2"]));
	});

	// The spec's per-action contract: the request shape matches the row's
	// and the page refreshes on success. The copy stream invalidates the
	// source host's images prefix, and the detail key extends the list key,
	// so the page's own overview refetches — the state the header and
	// overview render from stays truthful without a reload.
	it("sends the row's copy body and refreshes the page's overview on success", async () => {
		mockHost("write");
		mockImageDetail();
		mockHosts([source, staging, readOnly]);
		mockPreview();
		server.use(
			copyStreamHandler([
				imageCopyCompleteEventFixture(imageCopyResultFixture()),
			]),
		);
		renderDetailPage();
		const user = userEvent.setup();

		await user.click(await screen.findByTestId("copy-image-button"));
		await waitFor(() =>
			expect(screen.getByTestId("confirm-move-button")).toBeEnabled(),
		);
		const before = detailFetches;

		await user.click(screen.getByTestId("confirm-move-button"));

		await waitFor(() => expect(copyRequests.count).toBe(1));
		// The row's body: the target only, no mode, no force.
		expect(copyRequests.body).toEqual({ targetHostId: "m-2" });
		await waitFor(() => expect(detailFetches).toBeGreaterThan(before));
	});
});
