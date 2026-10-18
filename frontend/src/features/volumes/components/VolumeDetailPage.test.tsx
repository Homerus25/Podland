import { VolumeDetailPage } from "@features/volumes/components/VolumeDetailPage";
import { useVolumes } from "@features/volumes/hooks/useVolumes";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import type { ReactElement } from "react";
import { Route, Routes } from "react-router-dom";
import { describe, expect, it } from "vitest";
import { errorResponse } from "@/test/errors";
import { volumeFixture } from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

function mockVolumeDetail(
	responder: () => ReturnType<typeof HttpResponse.json>,
) {
	server.use(http.get("*/api/machines/m-1/volumes/app-data", responder));
}

// The page reads its route params (machineId, volumeName) and the shared
// layout reads machineId for the back button, so every render goes through
// a matching route inside the harness's MemoryRouter.
function renderDetailPage(ui: ReactElement = <VolumeDetailPage />) {
	return renderWithQuery(
		<Routes>
			<Route path="/machines/:machineId/volumes/:volumeName" element={ui} />
		</Routes>,
		{ router: { initialEntries: ["/machines/m-1/volumes/app-data"] } },
	);
}

describe("VolumeDetailPage", () => {
	it("renders the volume's full configuration as overview rows", async () => {
		mockVolumeDetail(() =>
			HttpResponse.json(
				volumeFixture({
					labels: { team: "platform", env: "prod" },
					options: { type: "tmpfs", device: "tmpfs" },
					size: 1500000,
				}),
			),
		);
		renderDetailPage();

		expect(screen.getByTestId("detail-page-title")).toHaveTextContent(
			"app-data",
		);
		const overview = await screen.findByTestId("volume-detail-overview");
		expect(overview).toHaveTextContent("app-data");
		expect(overview).toHaveTextContent("local");
		expect(screen.getByTestId("volume-detail-type-badge")).toHaveTextContent(
			"Named",
		);
		expect(
			screen.getByTestId("volume-detail-created"),
		).not.toBeEmptyDOMElement();
		expect(overview).toHaveTextContent("1.5 MB");
		expect(overview).toHaveTextContent(
			"/var/lib/containers/storage/volumes/app-data/_data",
		);

		const labels = screen.getByTestId("volume-detail-labels");
		expect(labels).toHaveTextContent("team");
		expect(labels).toHaveTextContent("platform");
		expect(labels).toHaveTextContent("env");
		expect(labels).toHaveTextContent("prod");

		const options = screen.getByTestId("volume-detail-options");
		expect(options).toHaveTextContent("type");
		expect(options).toHaveTextContent("tmpfs");
		expect(options).toHaveTextContent("device");
	});

	it("renders the anonymous badge and an em-dash when the volume has no size", async () => {
		mockVolumeDetail(() =>
			HttpResponse.json(volumeFixture({ anonymous: true, size: undefined })),
		);
		renderDetailPage();

		const overview = await screen.findByTestId("volume-detail-overview");
		expect(screen.getByTestId("volume-detail-type-badge")).toHaveTextContent(
			"Anonymous",
		);
		expect(overview).toHaveTextContent("—");
	});

	it("shows None placeholders when labels and options are empty", async () => {
		mockVolumeDetail(() =>
			HttpResponse.json(volumeFixture({ labels: {}, options: {} })),
		);
		renderDetailPage();

		await screen.findByTestId("volume-detail-overview");
		expect(screen.getByTestId("volume-detail-labels")).toHaveTextContent(
			/none/i,
		);
		expect(screen.getByTestId("volume-detail-options")).toHaveTextContent(
			/none/i,
		);
	});

	it("renders None placeholders when the response omits labels and options", async () => {
		// The wire shape for a volume without labels/options: the page must
		// render its None placeholders, not throw during render.
		mockVolumeDetail(() =>
			HttpResponse.json({
				name: "app-data",
				driver: "local",
				createdAt: "2026-08-20T10:00:00Z",
				anonymous: false,
				mountpoint: "/var/lib/containers/storage/volumes/app-data/_data",
			}),
		);
		renderDetailPage();

		await screen.findByTestId("volume-detail-overview");
		expect(screen.getByTestId("volume-detail-labels")).toHaveTextContent(
			/none/i,
		);
		expect(screen.getByTestId("volume-detail-options")).toHaveTextContent(
			/none/i,
		);
	});

	it("links the back button to the machine's volumes tab", async () => {
		mockVolumeDetail(() => HttpResponse.json(volumeFixture()));
		renderDetailPage();

		await screen.findByTestId("volume-detail-overview");
		expect(screen.getByTestId("detail-back-button")).toHaveAttribute(
			"href",
			"/machines/m-1?tab=volumes",
		);
	});

	it("reveals the inspect DTO as pretty-printed JSON, fetched lazily on first click", async () => {
		const volume = volumeFixture({ labels: { team: "platform" } });
		let requests = 0;
		server.use(
			http.get("*/api/machines/m-1/volumes/app-data", () => {
				requests++;
				return HttpResponse.json(volume);
			}),
		);
		const user = userEvent.setup();
		renderDetailPage();

		// The overview query fires on mount; the inspect query stays silent
		// until the first reveal click.
		await screen.findByTestId("volume-detail-overview");
		expect(requests).toBe(1);
		expect(screen.queryByTestId("inspect-json")).not.toBeInTheDocument();
		expect(screen.getByTestId("toggle-inspect-button")).toHaveTextContent(
			"Show inspect JSON",
		);

		await user.click(screen.getByTestId("toggle-inspect-button"));

		expect((await screen.findByTestId("inspect-json")).textContent).toBe(
			JSON.stringify(volume, null, 2),
		);
		expect(requests).toBe(2);
		expect(screen.getByTestId("toggle-inspect-button")).toHaveTextContent(
			"Hide inspect JSON",
		);
	});

	it("renders the error state through the standard error treatment", async () => {
		mockVolumeDetail(() =>
			errorResponse(
				403,
				"ACCESS_DENIED",
				"You do not have access to this machine",
				"r-2",
			),
		);
		renderDetailPage();

		const alert = await screen.findByTestId("detail-page-error");
		expect(alert).toHaveTextContent("ACCESS_DENIED");
		expect(alert).toHaveTextContent("You do not have access to this machine");
		expect(
			screen.queryByTestId("volume-detail-overview"),
		).not.toBeInTheDocument();
		expect(
			screen.queryByTestId("detail-page-not-found"),
		).not.toBeInTheDocument();
	});

	it("on 404 shows the not-found state and refetches the volume list", async () => {
		mockVolumeDetail(() =>
			errorResponse(404, "VOLUME_NOT_FOUND", "Volume not found", "r-1"),
		);
		const listRequests: string[] = [];
		server.use(
			http.get("*/api/machines/m-1/volumes", ({ request }) => {
				listRequests.push(new URL(request.url).pathname);
				return HttpResponse.json([]);
			}),
		);
		// A live list observer makes the invalidate-and-refetch observable
		// through the public interface: the stale row dies via a real refetch.
		function VolumeListProbe() {
			useVolumes("m-1");
			return null;
		}
		renderDetailPage(
			<>
				<VolumeListProbe />
				<VolumeDetailPage />
			</>,
		);

		expect(
			await screen.findByTestId("detail-page-not-found"),
		).toHaveTextContent(/no longer exists/i);
		await waitFor(() => expect(listRequests).toHaveLength(2));
	});
});
