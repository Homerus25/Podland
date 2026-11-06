import { VolumeDetailPage } from "@features/volumes/components/VolumeDetailPage";
import type { VolumeMoveResult } from "@features/volumes/services/volumesApi";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it } from "vitest";
import { errorResponse } from "@/test/errors";
import {
	hostFixture,
	volumeFixture,
	volumeMovePreviewFixture,
	volumeMoveResultFixture,
} from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";
import { ndjsonResponse } from "@/test/stream";

// The volume detail page's Move/Copy action (spec 477, ticket #482): the
// header-actions slot, the manage-volumes capability gate's tri-state, the
// shared transfer dialog with the volume fixed, and the page's own refresh
// after a completed transfer. The gate's classification is pinned once in
// the hook's suite and the transfer skeleton once in the shared
// TransferDialog suite; this suite keeps adapter wiring only.

const source = hostFixture();
const staging = hostFixture({ id: "m-2", name: "staging-01" });

// The host's next detail report, mutable so a completed transfer's refetch
// answers with the post-transfer state — the freshness contract is the
// refetch, not the mutation's own answer.
let reported = volumeFixture({ name: "app-data", size: 1024 });
let moved = false;
let detailRequests = 0;

function resetCounters() {
	reported = volumeFixture({ name: "app-data", size: 1024 });
	moved = false;
	detailRequests = 0;
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

function mockVolumeDetail() {
	server.use(
		http.get("*/api/hosts/m-1/volumes/app-data", () => {
			detailRequests += 1;
			// Once the transfer has landed, the source host no longer holds
			// the volume: the refetch is what tells the page.
			if (moved) {
				return errorResponse(
					404,
					"VOLUME_NOT_FOUND",
					"Volume not found",
					"r-1",
				);
			}
			return HttpResponse.json(reported);
		}),
	);
}

// The transfer dialog's own endpoints: the capability-filtered host list,
// the target-scoped preview, and the NDJSON move stream.
function mockTransferEndpoints(
	onMove: () => VolumeMoveResult = () => volumeMoveResultFixture(),
) {
	server.use(
		http.get("*/api/hosts", () => HttpResponse.json([source, staging])),
		http.get("*/api/hosts/m-1/volumes/app-data/move-preview", () =>
			HttpResponse.json(volumeMovePreviewFixture()),
		),
		http.post("*/api/hosts/m-1/volumes/app-data/move", () => {
			const result = onMove();
			return ndjsonResponse(
				[],
				[
					`${JSON.stringify({
						operationId: "op-1",
						type: "complete",
						hostId: "m-1",
						result,
					})}\n`,
				],
			);
		}),
	);
}

function renderDetailPage() {
	return renderWithQuery(
		<Routes>
			<Route
				path="/hosts/:hostId/volumes/:volumeName"
				element={<VolumeDetailPage />}
			/>
		</Routes>,
		{
			router: { initialEntries: ["/hosts/m-1/volumes/app-data"] },
			chakra: true,
		},
	);
}

describe("VolumeDetailPage move/copy", () => {
	beforeEach(resetCounters);

	it("renders Move/Copy for a manage-volumes caller and opens the transfer dialog with the volume fixed", async () => {
		mockHost("write");
		mockVolumeDetail();
		mockTransferEndpoints();
		renderDetailPage();
		const user = userEvent.setup();

		await user.click(await screen.findByTestId("move-volume-button"));

		const modal = await screen.findByTestId("modal");
		expect(modal).toHaveTextContent("Move or copy app-data");
		expect(screen.getByTestId("move-target-select")).toBeInTheDocument();
	});

	it("renders the shared write hint instead of Move/Copy for a read-level caller", async () => {
		mockHost("read");
		mockVolumeDetail();
		renderDetailPage();

		const hint = await screen.findByTestId("volumes-permission-hint");
		expect(hint).toHaveAttribute("data-requirement", "write");
		expect(screen.queryByTestId("move-volume-button")).not.toBeInTheDocument();
	});

	it("renders neither Move/Copy nor the hint while the gate is pending", async () => {
		mockVolumeDetail();
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

		await screen.findByTestId("volume-detail-overview");
		expect(screen.queryByTestId("move-volume-button")).not.toBeInTheDocument();
		expect(
			screen.queryByTestId("volumes-permission-hint"),
		).not.toBeInTheDocument();

		release();
		await screen.findByTestId("move-volume-button");
	});

	it("hides the actions when the volume no longer exists", async () => {
		mockHost("admin");
		server.use(
			http.get("*/api/hosts/m-1/volumes/app-data", () =>
				errorResponse(404, "VOLUME_NOT_FOUND", "Volume not found", "r-1"),
			),
			// The page's stale-row effect refetches the list on a 404.
			http.get("*/api/hosts/m-1/volumes", () => HttpResponse.json([])),
		);
		renderDetailPage();

		expect(
			await screen.findByTestId("detail-page-not-found"),
		).toBeInTheDocument();
		expect(screen.queryByTestId("move-volume-button")).not.toBeInTheDocument();
		expect(
			screen.queryByTestId("export-volume-button"),
		).not.toBeInTheDocument();
	});

	it("refreshes the page's own data after a completed transfer", async () => {
		mockHost("write");
		mockVolumeDetail();
		mockTransferEndpoints(() => {
			moved = true;
			return volumeMoveResultFixture();
		});
		// The page's stale-row effect refetches the list once the refetched
		// detail 404s.
		server.use(
			http.get("*/api/hosts/m-1/volumes", () => HttpResponse.json([])),
		);
		renderDetailPage();
		const user = userEvent.setup();

		await screen.findByTestId("volume-detail-overview");
		expect(detailRequests).toBe(1);
		await user.click(screen.getByTestId("move-volume-button"));
		await waitFor(() =>
			expect(screen.getByTestId("confirm-move-button")).toBeEnabled(),
		);
		await user.click(screen.getByTestId("confirm-move-button"));

		// The transfer's own invalidation refetches the detail query, whose
		// key extends the volumes list key — no reload, no manual refetch —
		// and the page renders the host's new answer.
		expect(
			await screen.findByTestId("detail-page-not-found"),
		).toBeInTheDocument();
		expect(detailRequests).toBeGreaterThanOrEqual(2);
	});
});
