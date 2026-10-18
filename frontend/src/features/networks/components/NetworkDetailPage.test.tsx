import { NetworkDetailPage } from "@features/networks/components/NetworkDetailPage";
import type { NetworkDetail } from "@features/networks/services/networksApi";
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { Route, Routes, useParams } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import { errorResponse } from "@/test/errors";
import { networkDetailFixture, rawInspectFixture } from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

function mockNetworkDetail(detail: NetworkDetail) {
	server.use(
		http.get("*/api/machines/m-1/networks/app-net", () =>
			HttpResponse.json(detail),
		),
	);
}

// The cross-link target: a stub standing in for the container detail page,
// rendering the route params so the test pins the exact URL navigated to.
function ContainerDetailStub() {
	const { machineId, containerId } = useParams<{
		machineId: string;
		containerId: string;
	}>();
	return (
		<div data-testid="container-detail-page">
			{machineId}/{containerId}
		</div>
	);
}

function renderNetworkDetailPage() {
	return renderWithQuery(
		<Routes>
			<Route
				path="/machines/:machineId/networks/:networkName"
				element={<NetworkDetailPage />}
			/>
			<Route
				path="/machines/:machineId/containers/:containerId"
				element={<ContainerDetailStub />}
			/>
		</Routes>,
		{ router: { initialEntries: ["/machines/m-1/networks/app-net"] } },
	);
}

afterEach(() => {
	vi.useRealTimers();
});

describe("NetworkDetailPage", () => {
	it("shows the loading state while the network detail is pending", async () => {
		server.use(
			http.get("*/api/machines/m-1/networks/app-net", async () => {
				await new Promise((resolve) => setTimeout(resolve, 50));
				return HttpResponse.json(networkDetailFixture());
			}),
		);
		renderNetworkDetailPage();

		expect(screen.getByTestId("detail-page-loading")).toBeInTheDocument();
		expect(
			screen.queryByTestId("detail-page-overview"),
		).not.toBeInTheDocument();
	});

	it("renders the overview with the network's name, id, driver, and creation time", async () => {
		vi.useFakeTimers({ shouldAdvanceTime: true });
		vi.setSystemTime(new Date("2026-08-23T12:00:00Z"));
		mockNetworkDetail(networkDetailFixture());
		renderNetworkDetailPage();

		const overview = await screen.findByTestId("detail-page-overview");
		expect(within(overview).getByText("Name")).toBeInTheDocument();
		expect(within(overview).getByText("app-net")).toBeInTheDocument();
		expect(within(overview).getByText("Id")).toBeInTheDocument();
		expect(within(overview).getByText("9f8e7d6c5b4a")).toBeInTheDocument();
		expect(within(overview).getByText("Driver")).toBeInTheDocument();
		expect(within(overview).getByText("bridge")).toBeInTheDocument();
		expect(within(overview).getByText("Created")).toBeInTheDocument();
		expect(within(overview).getByText("3 days ago")).toBeInTheDocument();
	});

	it("links each attached container to its container detail page", async () => {
		mockNetworkDetail(
			networkDetailFixture({
				containers: [
					{ id: "c-1", name: "my-container" },
					{ id: "c-2", name: "sidecar" },
				],
			}),
		);
		renderNetworkDetailPage();

		const first = await screen.findByTestId("network-container-link-c-1");
		expect(first).toHaveTextContent("my-container");
		expect(first).toHaveAttribute("href", "/machines/m-1/containers/c-1");
		const second = screen.getByTestId("network-container-link-c-2");
		expect(second).toHaveTextContent("sidecar");
		expect(second).toHaveAttribute("href", "/machines/m-1/containers/c-2");
	});

	it("navigates to the container's detail page when its link is clicked", async () => {
		mockNetworkDetail(networkDetailFixture());
		renderNetworkDetailPage();
		const user = userEvent.setup();

		await user.click(await screen.findByTestId("network-container-link-c-1"));

		const stub = await screen.findByTestId("container-detail-page");
		expect(stub).toHaveTextContent("m-1/c-1");
	});

	it("shows None when the network has no attached containers", async () => {
		mockNetworkDetail(networkDetailFixture({ containers: [] }));
		renderNetworkDetailPage();

		const overview = await screen.findByTestId("detail-page-overview");
		expect(within(overview).getByText("None")).toBeInTheDocument();
		expect(
			screen.queryByTestId(/network-container-link-/),
		).not.toBeInTheDocument();
	});

	it("returns to the networks tab from the back button", async () => {
		mockNetworkDetail(networkDetailFixture());
		renderNetworkDetailPage();

		await screen.findByTestId("detail-page-overview");
		expect(screen.getByTestId("detail-back-button")).toHaveAttribute(
			"href",
			"/machines/m-1?tab=networks",
		);
	});

	it("fetches the inspect JSON lazily on first reveal and pretty-prints it", async () => {
		let inspectRequests = 0;
		mockNetworkDetail(networkDetailFixture());
		server.use(
			http.get("*/api/machines/m-1/networks/app-net/inspect", () => {
				inspectRequests++;
				return HttpResponse.json(rawInspectFixture());
			}),
		);
		renderNetworkDetailPage();
		const user = userEvent.setup();

		// The page mounts with the overview rendered and no inspect request:
		// the query is enabled on first reveal, not on mount.
		expect(
			await screen.findByTestId("detail-page-overview"),
		).toBeInTheDocument();
		expect(inspectRequests).toBe(0);
		expect(screen.queryByTestId("inspect-json")).not.toBeInTheDocument();

		await user.click(screen.getByTestId("toggle-inspect-button"));

		expect((await screen.findByTestId("inspect-json")).textContent).toBe(
			JSON.stringify(rawInspectFixture(), null, 2),
		);
		expect(inspectRequests).toBe(1);
	});

	it("renders the error state when the detail endpoint fails", async () => {
		server.use(
			http.get("*/api/machines/m-1/networks/app-net", () =>
				errorResponse(403, "ACCESS_DENIED", "Access denied", "req-1"),
			),
		);
		renderNetworkDetailPage();

		const alert = await screen.findByTestId("detail-page-error");
		expect(alert).toHaveTextContent("ACCESS_DENIED");
		expect(alert).toHaveTextContent("Access denied");
		expect(
			screen.queryByTestId("detail-page-overview"),
		).not.toBeInTheDocument();
	});

	it("renders the not-found state when the network no longer exists (404)", async () => {
		server.use(
			http.get("*/api/machines/m-1/networks/app-net", () =>
				errorResponse(404, "NETWORK_NOT_FOUND", "Network not found", "req-1"),
			),
		);
		renderNetworkDetailPage();

		expect(
			await screen.findByTestId("detail-page-not-found"),
		).toHaveTextContent("This item no longer exists on the machine.");
		expect(screen.queryByTestId("detail-page-error")).not.toBeInTheDocument();
		expect(
			screen.queryByTestId("detail-page-overview"),
		).not.toBeInTheDocument();
	});
});
