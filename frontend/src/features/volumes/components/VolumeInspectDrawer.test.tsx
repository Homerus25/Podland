import { VolumeInspectDrawer } from "@features/volumes/components/VolumeInspectDrawer";
import { useVolumes } from "@features/volumes/hooks/useVolumes";
import { screen, waitFor } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import { errorResponse } from "@/test/errors";
import { volumeFixture } from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

function mockInspect(
	name: string,
	responder: () => ReturnType<typeof HttpResponse.json>,
) {
	server.use(http.get(`*/api/machines/m-1/volumes/${name}`, responder));
}

function renderDrawer() {
	return renderWithQuery(
		<VolumeInspectDrawer
			machineId="m-1"
			volumeName="app-data"
			isOpen={true}
			onClose={() => {}}
		/>,
	);
}

describe("VolumeInspectDrawer", () => {
	it("renders the volume's full configuration from the inspect endpoint", async () => {
		mockInspect("app-data", () =>
			HttpResponse.json(
				volumeFixture({
					labels: { team: "platform", env: "prod" },
					options: { type: "tmpfs", device: "tmpfs" },
					size: 1500000,
				}),
			),
		);
		renderDrawer();

		const content = await screen.findByTestId("volume-drawer-content");
		expect(content).toHaveTextContent("app-data");
		expect(content).toHaveTextContent("local");
		expect(content).toHaveTextContent("Named");
		expect(content).toHaveTextContent(
			"/var/lib/containers/storage/volumes/app-data/_data",
		);
		expect(
			screen.getByTestId("volume-drawer-created"),
		).not.toBeEmptyDOMElement();
		expect(content).toHaveTextContent("1.5 MB");

		const labels = screen.getByTestId("volume-drawer-labels");
		expect(labels).toHaveTextContent("team");
		expect(labels).toHaveTextContent("platform");
		expect(labels).toHaveTextContent("env");
		expect(labels).toHaveTextContent("prod");

		const options = screen.getByTestId("volume-drawer-options");
		expect(options).toHaveTextContent("type");
		expect(options).toHaveTextContent("tmpfs");
		expect(options).toHaveTextContent("device");
	});

	it("shows a placeholder when labels or options are empty", async () => {
		mockInspect("app-data", () =>
			HttpResponse.json(volumeFixture({ labels: {}, options: {} })),
		);
		renderDrawer();

		await screen.findByTestId("volume-drawer-content");
		expect(screen.getByTestId("volume-drawer-labels")).toHaveTextContent(
			/none/i,
		);
		expect(screen.getByTestId("volume-drawer-options")).toHaveTextContent(
			/none/i,
		);
	});

	it("shows a distinct loading state while fetching", () => {
		mockInspect(
			"app-data",
			() =>
				new Promise(() => {
					/* never resolves */
				}) as unknown as ReturnType<typeof HttpResponse.json>,
		);
		renderDrawer();

		expect(screen.getByTestId("volume-drawer-loading")).toBeInTheDocument();
		expect(
			screen.queryByTestId("volume-drawer-content"),
		).not.toBeInTheDocument();
	});

	it("on 404 shows a stale-volume message and refetches the volume list", async () => {
		mockInspect("app-data", () =>
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
		renderWithQuery(
			<>
				<VolumeListProbe />
				<VolumeInspectDrawer
					machineId="m-1"
					volumeName="app-data"
					isOpen={true}
					onClose={() => {}}
				/>
			</>,
		);

		expect(
			await screen.findByTestId("volume-drawer-not-found"),
		).toHaveTextContent(/no longer exists/i);
		await waitFor(() => expect(listRequests).toHaveLength(2));
	});

	it("shows the standard error treatment on server errors", async () => {
		mockInspect("app-data", () =>
			errorResponse(500, "INTERNAL_ERROR", "ssh failed", "r-2"),
		);
		renderDrawer();

		await waitFor(() =>
			expect(screen.getByTestId("volume-drawer-error")).toBeInTheDocument(),
		);
		expect(
			screen.queryByTestId("volume-drawer-loading"),
		).not.toBeInTheDocument();
		expect(
			screen.queryByTestId("volume-drawer-not-found"),
		).not.toBeInTheDocument();
	});

	it("does not render anything when closed", () => {
		renderWithQuery(
			<VolumeInspectDrawer
				machineId="m-1"
				volumeName="app-data"
				isOpen={false}
				onClose={() => {}}
			/>,
		);

		expect(
			screen.queryByTestId("volume-drawer-content"),
		).not.toBeInTheDocument();
	});
});
