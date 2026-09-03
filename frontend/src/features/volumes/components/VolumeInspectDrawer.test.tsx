import { VolumeInspectDrawer } from "@features/volumes/components/VolumeInspectDrawer";
import type { VolumeDetail } from "@features/volumes/services/volumesApi";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import { describe, expect, it, vi } from "vitest";
import { server } from "@/test/server";

function volumeFixture(overrides: Partial<VolumeDetail> = {}): VolumeDetail {
	return {
		name: "app-data",
		driver: "local",
		createdAt: "2026-08-20T10:00:00Z",
		anonymous: false,
		labels: { team: "platform", env: "prod" },
		options: { type: "tmpfs", device: "tmpfs" },
		mountpoint: "/var/lib/containers/storage/volumes/app-data/_data",
		size: 1500000,
		...overrides,
	};
}

function mockInspect(
	name: string,
	responder: () => ReturnType<typeof HttpResponse.json>,
) {
	server.use(http.get(`*/api/machines/m-1/volumes/${name}`, responder));
}

function renderDrawer(
	queryClient = new QueryClient({
		defaultOptions: { queries: { retry: false } },
	}),
) {
	return render(
		<QueryClientProvider client={queryClient}>
			<VolumeInspectDrawer
				machineId="m-1"
				volumeName="app-data"
				isOpen={true}
				onClose={() => {}}
			/>
		</QueryClientProvider>,
	);
}

describe("VolumeInspectDrawer", () => {
	it("renders the volume's full configuration from the inspect endpoint", async () => {
		mockInspect("app-data", () => HttpResponse.json(volumeFixture()));
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

	it("on 404 shows a stale-volume message and invalidates the list", async () => {
		mockInspect("app-data", () =>
			HttpResponse.json(
				{
					requestId: "r-1",
					error: { code: "VOLUME_NOT_FOUND", message: "Volume not found" },
				},
				{ status: 404 },
			),
		);
		const queryClient = new QueryClient({
			defaultOptions: { queries: { retry: false } },
		});
		const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");
		renderDrawer(queryClient);

		expect(
			await screen.findByTestId("volume-drawer-not-found"),
		).toHaveTextContent(/no longer exists/i);
		await waitFor(() =>
			expect(invalidateSpy).toHaveBeenCalledWith({
				queryKey: ["machines", "m-1", "volumes"],
			}),
		);
	});

	it("shows the standard error treatment on server errors", async () => {
		mockInspect("app-data", () =>
			HttpResponse.json(
				{
					requestId: "r-2",
					error: { code: "INTERNAL_ERROR", message: "ssh failed" },
				},
				{ status: 500 },
			),
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
		render(
			<QueryClientProvider
				client={
					new QueryClient({ defaultOptions: { queries: { retry: false } } })
				}
			>
				<VolumeInspectDrawer
					machineId="m-1"
					volumeName="app-data"
					isOpen={false}
					onClose={() => {}}
				/>
			</QueryClientProvider>,
		);

		expect(
			screen.queryByTestId("volume-drawer-content"),
		).not.toBeInTheDocument();
	});
});
