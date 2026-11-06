import { VolumeDetailPage } from "@features/volumes/components/VolumeDetailPage";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { hostFixture, volumeFixture } from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

// MSW cannot stream FormData bodies in this jsdom environment (the handler
// hangs on any body read), so the transport is mocked at the API-function
// seam — the sanctioned exception the list's populate suite uses too; the
// multipart payload shape is pinned by volumesApi.populate.test.
const populateVolumeMock = vi.fn().mockResolvedValue(undefined);
vi.mock("@features/volumes/services/volumesApi", async (importOriginal) => {
	const actual =
		await importOriginal<
			typeof import("@features/volumes/services/volumesApi")
		>();
	return {
		...actual,
		populateVolume: (...args: unknown[]) => populateVolumeMock(...args),
	};
});

// The volume detail page's Populate action (spec 477, ticket #482): the
// header-actions slot, the manage-volumes capability gate's tri-state, the
// shared upload dialog with the volume fixed, and the page's own refresh
// after a successful upload. The dialog's body behavior is pinned by the
// list's populate suite; this suite keeps adapter wiring only.

// The host's next detail report, mutable so a successful populate's refetch
// answers with the post-upload state — the freshness contract is the
// refetch, not the mutation's own answer.
let reported = volumeFixture({ name: "app-data", size: 1024 });

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
			HttpResponse.json(reported),
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

function file(name: string, contents = "x"): File {
	return new File([contents], name, { type: "text/plain" });
}

describe("VolumeDetailPage populate", () => {
	beforeEach(() => {
		reported = volumeFixture({ name: "app-data", size: 1024 });
		populateVolumeMock.mockClear();
		populateVolumeMock.mockResolvedValue(undefined);
	});

	it("renders Populate for a manage-volumes caller and opens the upload modal with the volume fixed", async () => {
		mockHost("write");
		mockVolumeDetail();
		renderDetailPage();
		const user = userEvent.setup();

		await user.click(await screen.findByTestId("populate-volume-button"));

		const modal = await screen.findByTestId("modal");
		expect(modal).toHaveTextContent("Populate app-data");
		expect(screen.getByTestId("populate-files-input")).toBeInTheDocument();
		expect(populateVolumeMock).not.toHaveBeenCalled();
	});

	it("renders the shared write hint instead of Populate for a read-level caller", async () => {
		mockHost("read");
		mockVolumeDetail();
		renderDetailPage();

		const hint = await screen.findByTestId("volumes-permission-hint");
		expect(hint).toHaveAttribute("data-requirement", "write");
		expect(
			screen.queryByTestId("populate-volume-button"),
		).not.toBeInTheDocument();
	});

	it("renders neither Populate nor the hint while the gate is pending", async () => {
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
		expect(
			screen.queryByTestId("populate-volume-button"),
		).not.toBeInTheDocument();
		expect(
			screen.queryByTestId("volumes-permission-hint"),
		).not.toBeInTheDocument();

		release();
		await screen.findByTestId("populate-volume-button");
	});

	it("refreshes the page's own data after a successful populate", async () => {
		mockHost("write");
		mockVolumeDetail();
		populateVolumeMock.mockImplementation(async () => {
			// The host's next detail report answers with the post-upload
			// state: the volume's reported size grew.
			reported = volumeFixture({ name: "app-data", size: 4000 });
		});
		renderDetailPage();
		const user = userEvent.setup();

		expect(await screen.findByText("1 KB")).toBeInTheDocument();
		await user.click(screen.getByTestId("populate-volume-button"));
		const input = (
			await screen.findByTestId("populate-files-input")
		).querySelector<HTMLInputElement>("input[type=file]")!;
		await user.upload(input, file("etc/app.conf"));
		await user.click(screen.getByTestId("confirm-populate-button"));

		// The mutation's own invalidation refetches the detail query, whose
		// key extends the volumes list key — no reload, no manual refetch.
		expect(await screen.findByText("4 KB")).toBeInTheDocument();
		await waitFor(() =>
			expect(screen.queryByTestId("modal")).not.toBeInTheDocument(),
		);
	});
});
