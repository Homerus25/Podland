import { NetworkList } from "@features/networks/components/NetworkList";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import { errorResponse } from "@/test/errors";
import {
	containerFixture,
	machineFixture,
	networkFixture,
	rawInspectFixture,
} from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

function networksHandler() {
	return http.get("*/api/machines/m-1/networks", () =>
		HttpResponse.json([networkFixture()]),
	);
}

function inspectHandler() {
	return http.get("*/api/machines/m-1/networks/app-net/inspect", () =>
		HttpResponse.json(rawInspectFixture()),
	);
}

function renderNetworkList() {
	renderWithQuery(<NetworkList machine={machineFixture()} />);
}

describe("NetworkList inspect", () => {
	it("opens the inspect dialog titled with the network's name and shows its JSON", async () => {
		const requestedPaths: string[] = [];
		server.use(
			networksHandler(),
			http.get("*/api/machines/m-1/networks/app-net/inspect", ({ request }) => {
				requestedPaths.push(new URL(request.url).pathname);
				return HttpResponse.json(rawInspectFixture());
			}),
		);
		renderNetworkList();
		const user = userEvent.setup();

		await user.click(await screen.findByTestId("network-row"));

		const modal = await screen.findByTestId("modal");
		expect(within(modal).getByText("app-net")).toBeInTheDocument();
		expect(screen.getByTestId("inspect-dialog-json").textContent).toBe(
			JSON.stringify(rawInspectFixture(), null, 2),
		);
		expect(requestedPaths).toEqual([
			"/api/machines/m-1/networks/app-net/inspect",
		]);
	});

	it("closing the dialog returns to the list", async () => {
		server.use(networksHandler(), inspectHandler());
		renderNetworkList();
		const user = userEvent.setup();

		await user.click(await screen.findByTestId("network-row"));
		await screen.findByTestId("inspect-dialog-json");
		await user.click(screen.getByRole("button", { name: "Close dialog" }));

		await waitFor(() =>
			expect(screen.queryByTestId("modal")).not.toBeInTheDocument(),
		);
		expect(screen.getByTestId("network-row")).toBeInTheDocument();
	});

	it("renders the error region inside the dialog when the endpoint 404s", async () => {
		server.use(
			networksHandler(),
			http.get("*/api/machines/m-1/networks/app-net/inspect", () =>
				errorResponse(404, "NETWORK_NOT_FOUND", "Network not found"),
			),
		);
		renderNetworkList();
		const user = userEvent.setup();

		await user.click(await screen.findByTestId("network-row"));

		const alert = await screen.findByTestId("inspect-dialog-error");
		expect(alert).toHaveTextContent("NETWORK_NOT_FOUND");
		expect(alert).toHaveTextContent("Network not found");
		expect(screen.queryByTestId("inspect-dialog-json")).not.toBeInTheDocument();
	});

	it("does not open the inspect dialog from the row's connect button", async () => {
		let inspectRequests = 0;
		server.use(
			networksHandler(),
			http.get("*/api/machines/m-1/containers", () =>
				HttpResponse.json({
					containers: [containerFixture()],
					total: 1,
					timestamp: "2026-08-26T00:00:00Z",
				}),
			),
			http.get("*/api/machines/m-1/networks/*/inspect", () => {
				inspectRequests++;
				return HttpResponse.json(rawInspectFixture());
			}),
		);
		renderNetworkList();
		const user = userEvent.setup();

		const row = await screen.findByTestId("network-row");
		await user.click(
			within(row).getByRole("button", { name: "Connect container app-net" }),
		);

		// the connect dialog opens, not the inspect one
		expect(
			await screen.findByText("Connect container to app-net"),
		).toBeInTheDocument();
		expect(inspectRequests).toBe(0);
		expect(
			screen.queryByTestId("inspect-dialog-loading"),
		).not.toBeInTheDocument();
		expect(screen.queryByTestId("inspect-dialog-json")).not.toBeInTheDocument();
		expect(
			screen.queryByTestId("inspect-dialog-error"),
		).not.toBeInTheDocument();
	});
});
