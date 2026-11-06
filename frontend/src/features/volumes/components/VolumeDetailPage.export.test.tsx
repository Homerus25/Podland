import { VolumeDetailPage } from "@features/volumes/components/VolumeDetailPage";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { hostFixture, volumeFixture } from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

// The volume detail page's Export action (spec 477, ticket #482): the
// header-actions slot, ungated because an export is a read, the busy state,
// and the error toast. The download transport is mocked at its own seam —
// the sanctioned exception for export suites — because the archive is a
// browser download, not an API response the page renders.
vi.mock("@shared/utils/download", () => ({
	downloadFile: vi.fn().mockResolvedValue(undefined),
}));

import { downloadFile } from "@shared/utils/download";

const mockedDownload = vi.mocked(downloadFile);

const volume = volumeFixture({ name: "app-data", size: 1024 });

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
		http.get("*/api/hosts/m-1/volumes/app-data", () =>
			HttpResponse.json(volume),
		),
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

beforeEach(() => {
	mockedDownload.mockClear();
	mockedDownload.mockResolvedValue(undefined);
});

describe("VolumeDetailPage export", () => {
	it("renders the Export action for a read-level caller and downloads the archive", async () => {
		mockHost("read");
		mockVolumeDetail();
		renderDetailPage();
		const user = userEvent.setup();

		await user.click(await screen.findByTestId("export-volume-button"));

		await waitFor(() => expect(mockedDownload).toHaveBeenCalledTimes(1));
		expect(mockedDownload).toHaveBeenCalledWith(
			"/api/hosts/m-1/volumes/app-data/export",
			"app-data.tar",
		);
	});

	// The header action row's one owner renders an ungated action whatever
	// the gate says, so a pending host query must not take Export down with
	// the Move/Copy and Populate actions it is still deciding about.
	it("renders the Export action while the host query is pending", async () => {
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
		mockVolumeDetail();
		renderDetailPage();

		expect(await screen.findByTestId("export-volume-button")).toHaveTextContent(
			"Export",
		);
		expect(screen.queryByTestId("move-volume-button")).not.toBeInTheDocument();
		expect(
			screen.queryByTestId("volumes-permission-hint"),
		).not.toBeInTheDocument();

		release();
		await screen.findByTestId("move-volume-button");
	});

	it("keeps the existing Remove action beside Export without duplicating it", async () => {
		mockHost("write");
		mockVolumeDetail();
		renderDetailPage();

		expect(await screen.findByTestId("export-volume-button")).toHaveTextContent(
			"Export",
		);
		expect(screen.getAllByTestId("remove-volume-button")).toHaveLength(1);
	});

	it("shows a busy state on the Export action while the download runs", async () => {
		mockHost("read");
		mockVolumeDetail();
		let resolveDownload: (() => void) | undefined;
		mockedDownload.mockImplementation(
			() =>
				new Promise<void>((resolve) => {
					resolveDownload = resolve;
				}),
		);
		renderDetailPage();
		const user = userEvent.setup();

		const button = await screen.findByTestId("export-volume-button");
		await user.click(button);
		await waitFor(() => expect(button).toHaveAttribute("data-loading"));

		resolveDownload?.();
		await waitFor(() => expect(button).not.toHaveAttribute("data-loading"));
	});

	it("toasts the standard error treatment when the export fails", async () => {
		mockHost("read");
		mockVolumeDetail();
		mockedDownload.mockRejectedValue({
			code: "VOLUME_NOT_FOUND",
			message: "no such volume",
		});
		renderDetailPage();
		const user = userEvent.setup();

		await user.click(await screen.findByTestId("export-volume-button"));

		expect(await screen.findByText(/failed to export/i)).toBeInTheDocument();
		// The action is usable again once the failure has surfaced.
		await waitFor(() =>
			expect(screen.getByTestId("export-volume-button")).toBeEnabled(),
		);
	});
});
