import { NetworkList } from "@features/networks/components/NetworkList";
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { Route, Routes, useParams } from "react-router-dom";
import { describe, expect, it } from "vitest";
import {
	containerFixture,
	hostFixture,
	networkFixture,
} from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

// Row-click navigation (spec 301): the row navigates to the network's
// detail page; the connect button stops propagation so it opens the
// connect dialog without navigating.

// The navigation target: a stub standing in for the network detail page,
// rendering the route params so the test pins the exact URL navigated to.
function NetworkDetailStub() {
	const { hostId, networkName } = useParams<{
		hostId: string;
		networkName: string;
	}>();
	return (
		<div data-testid="network-detail-page">
			{hostId}/{networkName}
		</div>
	);
}

function renderNetworkList() {
	renderWithQuery(
		<Routes>
			<Route
				path="/hosts/:hostId"
				element={<NetworkList host={hostFixture()} />}
			/>
			<Route
				path="/hosts/:hostId/networks/:networkName"
				element={<NetworkDetailStub />}
			/>
		</Routes>,
		{ router: { initialEntries: ["/hosts/m-1"] } },
	);
}

function mockLists() {
	return [
		http.get("*/api/hosts/m-1/networks", () =>
			HttpResponse.json([networkFixture()]),
		),
		http.get("*/api/hosts/m-1/containers", () =>
			HttpResponse.json({
				containers: [containerFixture()],
				total: 1,
				timestamp: "2026-08-26T00:00:00Z",
			}),
		),
	];
}

describe("NetworkList navigation", () => {
	it("navigates to the network's detail page when the row is clicked", async () => {
		server.use(...mockLists());
		renderNetworkList();
		const user = userEvent.setup();

		await user.click(await screen.findByTestId("network-row"));

		const stub = await screen.findByTestId("network-detail-page");
		expect(stub).toHaveTextContent("m-1/app-net");
		expect(screen.queryByTestId("network-row")).not.toBeInTheDocument();
	});

	it("does not navigate when the row's connect button is clicked", async () => {
		server.use(...mockLists());
		renderNetworkList();
		const user = userEvent.setup();

		const row = await screen.findByTestId("network-row");
		await user.click(
			within(row).getByRole("button", { name: "Connect container app-net" }),
		);

		// the connect dialog opens, not the detail page
		expect(await screen.findByTestId("modal")).toBeInTheDocument();
		expect(
			screen.getByText("Connect container to app-net"),
		).toBeInTheDocument();
		expect(screen.queryByTestId("network-detail-page")).not.toBeInTheDocument();
	});
});
