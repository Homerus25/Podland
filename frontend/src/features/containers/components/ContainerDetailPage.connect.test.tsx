import { ContainerDetailPage } from "@features/containers/components/ContainerDetailPage";
import type { ContainerDetail } from "@features/containers/services/containersApi";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it } from "vitest";
import { errorResponse } from "@/test/errors";
import {
	containerDetailFixture,
	containerFixture,
	hostFixture,
	networkFixture,
} from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

// The container the page's overview rides. A successful connect rewrites its
// networks inside the POST, so the refetched detail — not the mutation's own
// answer — is what updates the rendered Networks row.
let reported: ContainerDetail;
let connectRequests: { pathname: string; body: unknown }[];

function mockDetail() {
	server.use(
		http.get("*/api/hosts/m-1/containers/c-1", () =>
			HttpResponse.json(reported),
		),
	);
}

// The host query feeds the page's capability gates.
function mockHost(permission: "read" | "write" | "admin" = "write") {
	server.use(
		http.get("*/api/hosts/m-1", () =>
			HttpResponse.json(
				hostFixture({ permission, isOwner: permission === "admin" }),
			),
		),
	);
}

// The dialog fetches both lists itself on open: the networks it offers and
// the containers list it always reads.
function mockLists() {
	server.use(
		http.get("*/api/hosts/m-1/networks", () =>
			HttpResponse.json([
				networkFixture(),
				networkFixture({ name: "other-net", subnets: ["10.89.0.0/16"] }),
			]),
		),
		http.get("*/api/hosts/m-1/containers", () =>
			HttpResponse.json({
				containers: [
					containerFixture({
						id: "c-1",
						names: ["/web"],
						networks: reported.networks,
					}),
				],
				total: 1,
				timestamp: "2026-08-26T00:00:00Z",
			}),
		),
	);
}

// One connect request per picked network, carrying the fixed container.
function connectHandler() {
	return http.post(
		"*/api/hosts/m-1/networks/:networkName/connect",
		async ({ request, params }) => {
			const networkName = params.networkName as string;
			connectRequests.push({
				pathname: new URL(request.url).pathname,
				body: await request.json(),
			});
			reported = {
				...reported,
				networks: [...(reported.networks ?? []), networkName],
			};
			return new HttpResponse(null, { status: 200 });
		},
	);
}

function renderDetailPage() {
	return renderWithQuery(
		<Routes>
			<Route
				path="/hosts/:hostId/containers/:containerId"
				element={<ContainerDetailPage />}
			/>
		</Routes>,
		{ router: { initialEntries: ["/hosts/m-1/containers/c-1"] } },
	);
}

describe("ContainerDetailPage connect to network", () => {
	beforeEach(() => {
		reported = containerDetailFixture();
		connectRequests = [];
		mockHost("write");
		mockDetail();
		mockLists();
	});

	it("opens the shared connect dialog with the container fixed", async () => {
		const user = userEvent.setup();
		renderDetailPage();

		await user.click(await screen.findByTestId("connect-container-button"));

		expect(await screen.findByTestId("modal")).toBeInTheDocument();
		expect(screen.getByText("Connect web to a network")).toBeInTheDocument();
	});

	it("does not offer the networks the container is already attached to", async () => {
		const user = userEvent.setup();
		renderDetailPage();

		await user.click(await screen.findByTestId("connect-container-button"));
		await user.click(await screen.findByTestId("searchable-select-input"));

		expect(
			await screen.findByTestId("searchable-select-option-other-net"),
		).toBeInTheDocument();
		expect(
			screen.queryByTestId("searchable-select-option-app-net"),
		).not.toBeInTheDocument();
	});

	it("sends one connect request per picked network with the container fixed", async () => {
		server.use(connectHandler());
		const user = userEvent.setup();
		renderDetailPage();

		await user.click(await screen.findByTestId("connect-container-button"));
		await user.click(await screen.findByTestId("searchable-select-input"));
		await user.click(
			await screen.findByTestId("searchable-select-option-other-net"),
		);
		await user.click(screen.getByTestId("connect-network-submit"));

		await waitFor(() => expect(connectRequests).toHaveLength(1));
		expect(connectRequests[0]).toEqual({
			pathname: "/api/hosts/m-1/networks/other-net/connect",
			body: { container: "c-1" },
		});
	});

	it("refreshes the page's Networks row after a successful connect", async () => {
		server.use(connectHandler());
		const user = userEvent.setup();
		renderDetailPage();

		expect(
			await screen.findByTestId("container-detail-networks"),
		).toHaveTextContent("app-net");
		await user.click(screen.getByTestId("connect-container-button"));
		await user.click(await screen.findByTestId("searchable-select-input"));
		await user.click(
			await screen.findByTestId("searchable-select-option-other-net"),
		);
		await user.click(screen.getByTestId("connect-network-submit"));

		await waitFor(() =>
			expect(screen.getByTestId("container-detail-networks")).toHaveTextContent(
				"other-net",
			),
		);
	});

	// The gate smokes: connect rides manage-networks, a different capability
	// from the lifecycle actions beside it.
	it("renders the write-permission hint instead of Connect for a caller without manage-networks", async () => {
		mockHost("read");
		renderDetailPage();

		const hint = await screen.findByTestId("container-connect-permission-hint");
		expect(hint).toHaveAttribute("data-requirement", "write");
		expect(
			screen.queryByTestId("connect-container-button"),
		).not.toBeInTheDocument();
	});

	it("renders neither Connect nor the hint while the host query is pending", async () => {
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

		await screen.findByTestId("container-detail-name");
		expect(
			screen.queryByTestId("connect-container-button"),
		).not.toBeInTheDocument();
		expect(
			screen.queryByTestId("container-connect-permission-hint"),
		).not.toBeInTheDocument();

		release();
		await screen.findByTestId("connect-container-button");
	});

	// The hint self-gates on the overview DTO like every control does: a
	// denied manage-networks caller on a dead container's page reads no
	// hint for an action that could never fire — the Remove cascade's
	// not-found suppression, applied to the hint-only entry.
	it("renders no connect hint once the overview 404s", async () => {
		mockHost("read");
		server.use(
			http.get("*/api/hosts/m-1/containers/c-1", () =>
				errorResponse(404, "CONTAINER_NOT_FOUND", "Container not found"),
			),
		);
		renderDetailPage();

		expect(
			await screen.findByText("This item no longer exists on the host."),
		).toBeInTheDocument();
		expect(
			screen.queryByTestId("container-connect-permission-hint"),
		).not.toBeInTheDocument();
		expect(
			screen.queryByTestId("containers-permission-hint"),
		).not.toBeInTheDocument();
	});
});
