import { VolumeList } from "@features/volumes/components/VolumeList";
import type { VolumeSummary } from "@features/volumes/services/volumesApi";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import { errorResponse } from "@/test/errors";
import { volumeFixture } from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

function mockVolumes(volumes: VolumeSummary[]) {
	server.use(
		http.get("*/api/machines/m-1/volumes", () => HttpResponse.json(volumes)),
	);
}

function renderVolumeList(
	permission: "read" | "write" | "admin" = "admin" as const,
) {
	return renderWithQuery(
		<VolumeList machineId="m-1" permission={permission} />,
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
			http.get("*/api/machines/m-1/volumes", async () => {
				await new Promise((resolve) => setTimeout(resolve, 50));
				return HttpResponse.json([volumeFixture()]);
			}),
		);
		renderVolumeList();

		expect(screen.getByTestId("volumes-loading")).toBeInTheDocument();
		expect(screen.getByTestId("create-volume-button")).toBeInTheDocument();
		expect(screen.getByTestId("prune-volumes-button")).toBeInTheDocument();
	});

	it("opens the inspect drawer when a row is clicked", async () => {
		mockVolumes([volumeFixture()]);
		server.use(
			http.get("*/api/machines/m-1/volumes/app-data", () =>
				HttpResponse.json(volumeFixture({ labels: { team: "platform" } })),
			),
		);
		renderVolumeList();

		const user = userEvent.setup();
		await user.click(await screen.findByTestId("volume-row"));

		const content = await screen.findByTestId("volume-drawer-content");
		expect(content).toHaveTextContent("app-data");
		expect(screen.getByTestId("volume-drawer-labels")).toHaveTextContent(
			"platform",
		);
	});

	it("removes a deleted row from the list after a stale 404 inspect", async () => {
		mockVolumes([volumeFixture()]);
		let listCount = 0;
		server.use(
			http.get("*/api/machines/m-1/volumes", () => {
				listCount += 1;
				return HttpResponse.json(listCount > 1 ? [] : [volumeFixture()]);
			}),
			http.get("*/api/machines/m-1/volumes/app-data", () =>
				errorResponse(404, "VOLUME_NOT_FOUND", "Volume not found", "r-1"),
			),
		);
		renderVolumeList();

		const user = userEvent.setup();
		await user.click(await screen.findByTestId("volume-row"));

		expect(
			await screen.findByTestId("volume-drawer-not-found"),
		).toHaveTextContent(/no longer exists/i);
		await waitFor(() =>
			expect(screen.queryByTestId("volume-row")).not.toBeInTheDocument(),
		);
		expect(screen.getByTestId("volumes-empty")).toBeInTheDocument();
	});
});
