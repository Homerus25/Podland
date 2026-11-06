import { NetworkDetailPage } from "@features/networks/components/NetworkDetailPage";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http, type JsonBodyType } from "msw";
import { Route, Routes, useSearchParams } from "react-router-dom";
import { beforeEach, describe, expect, it } from "vitest";
import { errorJson, errorResponse } from "@/test/errors";
import { hostFixture, networkDetailFixture } from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

// The network detail page's Remove action (spec 461, ticket #465): the
// header-actions slot, the capability-gate wiring, the force-refusal
// confirm, and the navigate-away on success. The gate's classification is
// pinned once in the hook's suite and the dialog skeleton once in the
// shared MutationDialog suite; this suite keeps domain assertions only.

let deleteRequests: {
	count: number;
	pathname: string;
	force: string | null;
} = { count: 0, pathname: "", force: null };

function resetCounters() {
	deleteRequests = { count: 0, pathname: "", force: null };
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
			HttpResponse.json(networkDetailFixture()),
		),
	);
}

function mockDelete(
	responder: () => { status: number; body?: JsonBodyType } = () => ({
		status: 204,
	}),
) {
	server.use(
		http.delete("*/api/hosts/m-1/networks/*", ({ request }) => {
			deleteRequests.count += 1;
			const url = new URL(request.url);
			deleteRequests.pathname = url.pathname;
			deleteRequests.force = url.searchParams.get("force");
			const response = responder();
			if (response.status !== 204) {
				return HttpResponse.json(response.body, { status: response.status });
			}
			return new HttpResponse(null, { status: 204 });
		}),
	);
}

// The navigation target: a stub standing in for the host page, rendering
// the tab search parameter so the test pins the exact URL navigated to.
function HostTabStub() {
	const [params] = useSearchParams();
	return <div data-testid="host-tab-page">{params.get("tab")}</div>;
}

function renderDetailPage() {
	return renderWithQuery(
		<Routes>
			<Route
				path="/hosts/:hostId/networks/:networkName"
				element={<NetworkDetailPage />}
			/>
			<Route path="/hosts/:hostId" element={<HostTabStub />} />
		</Routes>,
		{ router: { initialEntries: ["/hosts/m-1/networks/app-net"] } },
	);
}

describe("NetworkDetailPage remove", () => {
	beforeEach(resetCounters);

	it("renders the Remove action for a write-level caller", async () => {
		mockHost("write");
		mockNetworkDetail();
		renderDetailPage();

		expect(
			await screen.findByTestId("remove-network-button"),
		).toHaveTextContent("Remove");
		expect(
			screen.queryByTestId("networks-permission-hint"),
		).not.toBeInTheDocument();
	});

	it("renders the shared write hint instead of the Remove action for a read-level caller", async () => {
		mockHost("read");
		mockNetworkDetail();
		renderDetailPage();

		const hint = await screen.findByTestId("networks-permission-hint");
		expect(hint).toHaveAttribute("data-requirement", "write");
		expect(hint).toHaveTextContent(
			"You have read-only access to this host. Ask an owner or admin for write access to manage its networks.",
		);
		expect(
			screen.queryByTestId("remove-network-button"),
		).not.toBeInTheDocument();
	});

	it("renders neither the Remove action nor the hint while the gate is pending", async () => {
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
			screen.queryByTestId("remove-network-button"),
		).not.toBeInTheDocument();
		expect(
			screen.queryByTestId("networks-permission-hint"),
		).not.toBeInTheDocument();

		release();
		await screen.findByTestId("remove-network-button");
	});

	it("hides the Remove action when the network no longer exists", async () => {
		mockHost();
		server.use(
			http.get("*/api/hosts/m-1/networks/app-net", () =>
				errorResponse(404, "NETWORK_NOT_FOUND", "Network not found", "r-1"),
			),
		);
		renderDetailPage();

		expect(
			await screen.findByTestId("detail-page-not-found"),
		).toBeInTheDocument();
		expect(
			screen.queryByTestId("remove-network-button"),
		).not.toBeInTheDocument();
		expect(
			screen.queryByTestId("networks-permission-hint"),
		).not.toBeInTheDocument();
	});

	it("opens the confirm naming the network and sends nothing before confirming", async () => {
		mockHost();
		mockNetworkDetail();
		mockDelete();
		renderDetailPage();
		const user = userEvent.setup();

		await user.click(await screen.findByTestId("remove-network-button"));

		expect(await screen.findByTestId("modal")).toBeInTheDocument();
		expect(screen.getByText(/Remove network app-net/)).toBeInTheDocument();
		expect(deleteRequests.count).toBe(0);
	});

	it("sends the plain delete on confirm and lands on the networks tab", async () => {
		mockHost();
		mockNetworkDetail();
		mockDelete();
		renderDetailPage();
		const user = userEvent.setup();

		await user.click(await screen.findByTestId("remove-network-button"));
		await user.click(await screen.findByTestId("confirm-delete-button"));

		await waitFor(() => expect(deleteRequests.count).toBe(1));
		expect(deleteRequests.pathname).toBe("/api/hosts/m-1/networks/app-net");
		expect(deleteRequests.force).toBeNull();
		expect(await screen.findByTestId("host-tab-page")).toHaveTextContent(
			"networks",
		);
	});

	it("swaps to the in-use hint on a refusal and force-deletes on the retry", async () => {
		mockHost();
		mockNetworkDetail();
		let refused = true;
		mockDelete(() => {
			if (!refused) return { status: 204 };
			refused = false;
			return {
				status: 409,
				body: errorJson(
					"NETWORK_IN_USE",
					"network is in use by container abc",
					"req-7",
				),
			};
		});
		renderDetailPage();
		const user = userEvent.setup();

		await user.click(await screen.findByTestId("remove-network-button"));
		await user.click(await screen.findByTestId("confirm-delete-button"));

		expect(
			await screen.findByTestId("network-in-use-warning"),
		).toBeInTheDocument();
		expect(screen.getByText(/attached to this network/i)).toBeInTheDocument();
		expect(
			screen.queryByTestId("confirm-delete-button"),
		).not.toBeInTheDocument();

		await user.click(screen.getByTestId("force-remove-button"));

		await waitFor(() => expect(deleteRequests.count).toBe(2));
		expect(deleteRequests.force).toBe("true");
		expect(await screen.findByTestId("host-tab-page")).toHaveTextContent(
			"networks",
		);
	});
});
