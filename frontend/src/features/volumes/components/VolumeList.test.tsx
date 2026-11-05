import { VolumeList } from "@features/volumes/components/VolumeList";
import type { VolumeSummary } from "@features/volumes/services/volumesApi";
import { downloadFile } from "@shared/utils/download";
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { useLocation } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";
import { hostFixture, volumeFixture } from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

// The export button's download is mocked: the stopPropagation test only
// needs to verify the row click does not fire, not the download itself
// (the export flow has its own suite in VolumeList.export.test.tsx).
vi.mock("@shared/utils/download", () => ({
	downloadFile: vi.fn().mockResolvedValue(undefined),
}));
const mockedDownload = vi.mocked(downloadFile);

function mockVolumes(volumes: VolumeSummary[]) {
	server.use(
		http.get("*/api/hosts/m-1/volumes", () => HttpResponse.json(volumes)),
	);
}

function LocationProbe() {
	const location = useLocation();
	return <div data-testid="location">{location.pathname}</div>;
}

function renderVolumeList(
	permission: "read" | "write" | "admin" = "admin" as const,
) {
	return renderWithQuery(
		<>
			<LocationProbe />
			<VolumeList host={hostFixture({ permission })} />
		</>,
		{ chakra: true, router: { initialEntries: ["/hosts/m-1"] } },
	);
}

describe("VolumeList", () => {
	it("renders volume rows with name, type badge, driver, created date and size", async () => {
		mockVolumes([
			volumeFixture({ name: "app-data", anonymous: false, size: 1500000 }),
			volumeFixture({
				name: "a1b2c3d4",
				anonymous: true,
				createdAt: "2026-08-25T10:00:00Z",
			}),
		]);
		renderVolumeList();

		const rows = await screen.findAllByTestId("volume-row");
		expect(rows).toHaveLength(2);

		const named = within(rows[0]);
		expect(named.getByText("app-data")).toBeInTheDocument();
		expect(named.getByTestId("volume-type-badge")).toHaveTextContent(/named/i);
		expect(named.getByText("local")).toBeInTheDocument();
		expect(named.getByText(/1\.5 MB/)).toBeInTheDocument();

		const anonymous = within(rows[1]);
		expect(anonymous.getByText("a1b2c3d4")).toBeInTheDocument();
		expect(anonymous.getByTestId("volume-type-badge")).toHaveTextContent(
			/anonymous/i,
		);
	});

	it("renders a placeholder cell when the API reports no size", async () => {
		mockVolumes([volumeFixture({ size: undefined })]);
		renderVolumeList();

		const row = await screen.findByTestId("volume-row");
		expect(within(row).getAllByRole("cell")).toHaveLength(6);
		expect(within(row).getByText("—")).toBeInTheDocument();
	});

	it("shows a loading state before volumes arrive, with the toolbar still above it", async () => {
		server.use(
			http.get("*/api/hosts/m-1/volumes", async () => {
				await new Promise((resolve) => setTimeout(resolve, 50));
				return HttpResponse.json([volumeFixture()]);
			}),
		);
		renderVolumeList();

		expect(screen.getByTestId("volumes-loading")).toBeInTheDocument();
		expect(screen.getByTestId("create-volume-button")).toBeInTheDocument();
		expect(screen.getByTestId("prune-volumes-button")).toBeInTheDocument();
	});

	it("navigates to the volume's detail page when a row is clicked", async () => {
		mockVolumes([volumeFixture()]);
		renderVolumeList();

		const user = userEvent.setup();
		await user.click(await screen.findByTestId("volume-row"));

		expect(screen.getByTestId("location").textContent).toBe(
			"/hosts/m-1/volumes/app-data",
		);
	});

	it("keeps the row's action buttons from triggering the navigation", async () => {
		mockVolumes([volumeFixture()]);
		// The hosts endpoint for the move/copy dialog's target list.
		server.use(
			http.get("*/api/hosts", () => HttpResponse.json([hostFixture()])),
		);
		renderVolumeList();

		const user = userEvent.setup();
		const row = await screen.findByTestId("volume-row");

		// Export — fires the download, not the row navigation.
		await user.click(
			within(row).getByRole("button", { name: "Export app-data" }),
		);
		expect(mockedDownload).toHaveBeenCalledTimes(1);
		expect(screen.getByTestId("location").textContent).toBe("/hosts/m-1");

		// Populate — opens the populate dialog, not the detail page.
		await user.click(
			within(row).getByRole("button", { name: "Populate app-data" }),
		);
		expect(
			await screen.findByTestId("confirm-populate-button"),
		).toBeInTheDocument();
		expect(screen.getByTestId("location").textContent).toBe("/hosts/m-1");
		await user.click(screen.getByRole("button", { name: "Cancel" }));

		// Move/Copy — opens the transfer dialog, not the detail page.
		await user.click(within(row).getByTestId("move-volume-button"));
		expect(await screen.findByText(/move or copy/i)).toBeInTheDocument();
		expect(screen.getByTestId("location").textContent).toBe("/hosts/m-1");
		await user.click(screen.getByRole("button", { name: "Cancel" }));

		// Delete — opens the confirm dialog, not the detail page.
		await user.click(
			within(row).getByRole("button", { name: "Delete app-data" }),
		);
		expect(
			await screen.findByTestId("confirm-delete-button"),
		).toBeInTheDocument();
		expect(screen.getByTestId("location").textContent).toBe("/hosts/m-1");
	});

	it("sorts rows when a column header is clicked", async () => {
		mockVolumes([
			volumeFixture({ name: "zebra-data" }),
			volumeFixture({ name: "apple-data" }),
		]);
		renderVolumeList();
		await screen.findAllByTestId("volume-row");
		const user = userEvent.setup();
		await user.click(screen.getByTestId("sort-volume-0"));
		const rows = screen.getAllByTestId("volume-row");
		expect(within(rows[0]).getByText("apple-data")).toBeInTheDocument();
		expect(within(rows[1]).getByText("zebra-data")).toBeInTheDocument();
	});
});
