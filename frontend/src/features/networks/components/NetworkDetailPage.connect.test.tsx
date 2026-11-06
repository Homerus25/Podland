import { NetworkDetailPage } from "@features/networks/components/NetworkDetailPage";
import type { NetworkContainerRef } from "@features/networks/services/networksApi";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it } from "vitest";
import {
	containerFixture,
	hostFixture,
	networkDetailFixture,
	networkFixture,
} from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

// The network detail page's Connect container action (spec 477, ticket
// #480): the header-actions slot beside Remove, the manage-networks gate,
// and the shared ConnectDialog with the network fixed. The dialog's own
// behavior (option filtering, submit policy, error summary) is pinned once
// in ConnectDialog.test.tsx and the gate's classification once in the
// capability-gate hook's suite; this suite keeps adapter wiring, the
// request shape, and the refresh-on-success assertion.

// The network's attached containers, mutated by the connect handler so the
// refetched detail DTO reflects the attach — the refresh is asserted
// through the rendered overview, never through an invalidation spy.
let attached: NetworkContainerRef[] = [];
let connectRequests: { url: string; body: unknown }[] = [];

function resetState() {
	attached = [{ id: "c-1", name: "my-container" }];
	connectRequests = [];
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

function mockNetworkDetail() {
	server.use(
		http.get("*/api/hosts/m-1/networks/app-net", () =>
			HttpResponse.json(networkDetailFixture({ containers: attached })),
		),
	);
}

// The dialog fetches its own options: the containers list (c-1 is already
// on app-net, c-2 is eligible) and the networks list.
function mockDialogOptions() {
	server.use(
		http.get("*/api/hosts/m-1/containers", () =>
			HttpResponse.json({
				containers: [
					containerFixture({
						id: "c-1",
						names: ["/my-container"],
						networks: ["app-net"],
					}),
					containerFixture({ id: "c-2", names: ["/db"], networks: [] }),
				],
				total: 2,
				timestamp: "2026-08-26T00:00:00Z",
			}),
		),
		http.get("*/api/hosts/m-1/networks", () =>
			HttpResponse.json([networkFixture()]),
		),
	);
}

function mockConnect() {
	server.use(
		http.post(
			"*/api/hosts/m-1/networks/:networkName/connect",
			async ({ request }) => {
				const body = (await request.json()) as { container: string };
				connectRequests.push({ url: request.url, body });
				attached = [...attached, { id: body.container, name: "db" }];
				return new HttpResponse(null, { status: 200 });
			},
		),
	);
}

function renderDetailPage() {
	return renderWithQuery(
		<Routes>
			<Route
				path="/hosts/:hostId/networks/:networkName"
				element={<NetworkDetailPage />}
			/>
		</Routes>,
		{ router: { initialEntries: ["/hosts/m-1/networks/app-net"] } },
	);
}

describe("NetworkDetailPage connect", () => {
	beforeEach(resetState);

	it("renders the Connect container action for a write-level caller", async () => {
		mockHost("write");
		mockNetworkDetail();
		renderDetailPage();

		expect(
			await screen.findByTestId("connect-network-button"),
		).toHaveTextContent("Connect container");
		expect(
			screen.queryByTestId("networks-permission-hint"),
		).not.toBeInTheDocument();
	});

	it("renders the shared write hint instead of the Connect action for a read-level caller", async () => {
		mockHost("read");
		mockNetworkDetail();
		renderDetailPage();

		const hint = await screen.findByTestId("networks-permission-hint");
		expect(hint).toHaveAttribute("data-requirement", "write");
		expect(
			screen.queryByTestId("connect-network-button"),
		).not.toBeInTheDocument();
	});

	it("renders neither the Connect action nor the hint while the gate is pending", async () => {
		mockNetworkDetail();
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

		await screen.findByTestId("detail-page-overview");
		expect(
			screen.queryByTestId("connect-network-button"),
		).not.toBeInTheDocument();
		expect(
			screen.queryByTestId("networks-permission-hint"),
		).not.toBeInTheDocument();

		release();
		await screen.findByTestId("connect-network-button");
	});

	it("opens the shared connect dialog with the network fixed", async () => {
		mockHost();
		mockNetworkDetail();
		mockDialogOptions();
		renderDetailPage();
		const user = userEvent.setup();

		await user.click(await screen.findByTestId("connect-network-button"));

		expect(await screen.findByTestId("modal")).toBeInTheDocument();
		expect(
			screen.getByText("Connect container to app-net"),
		).toBeInTheDocument();
		expect(screen.getByTestId("connect-container-submit")).toBeDisabled();
	});

	it("sends the row's connect request and refreshes the attached containers on success", async () => {
		mockHost();
		mockNetworkDetail();
		mockDialogOptions();
		mockConnect();
		renderDetailPage();
		const user = userEvent.setup();

		// The overview starts with the one attached container.
		expect(
			await screen.findByTestId("network-container-link-c-1"),
		).toBeInTheDocument();
		expect(
			screen.queryByTestId("network-container-link-c-2"),
		).not.toBeInTheDocument();

		await user.click(screen.getByTestId("connect-network-button"));
		await user.click(await screen.findByTestId("searchable-select-input"));
		await user.click(await screen.findByTestId("searchable-select-option-c-2"));
		await user.click(screen.getByTestId("connect-container-submit"));

		// The request shape is the row's: one POST per pick to the network's
		// connect route with the container id in the body.
		await waitFor(() => expect(connectRequests).toHaveLength(1));
		expect(
			connectRequests[0].url.endsWith(
				"/api/hosts/m-1/networks/app-net/connect",
			),
		).toBe(true);
		expect(connectRequests[0].body).toEqual({ container: "c-2" });

		// The dialog closes on success and the page's attached-containers
		// section refetches: the new container appears without a reload.
		await waitFor(() =>
			expect(screen.queryByTestId("modal")).not.toBeInTheDocument(),
		);
		expect(
			await screen.findByTestId("network-container-link-c-2"),
		).toHaveTextContent("db");
	});
});
