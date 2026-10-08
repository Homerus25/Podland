import { ContainerList } from "@features/containers/components/ContainerList";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http, type JsonBodyType } from "msw";
import { describe, expect, it } from "vitest";
import { errorJson } from "@/test/errors";
import {
	containerFixture,
	machineFixture,
	networkFixture,
} from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

// c-1 is already attached to app-net; the dialog must offer only the
// networks it is not on.
const attached = containerFixture({
	id: "c-1",
	names: ["/web"],
	networks: ["app-net"],
});

let networksGets = 0;
let containersGets = 0;
let connectRequests: { count: number; url: string; body: unknown } = {
	count: 0,
	url: "",
	body: null,
};

function resetCounters() {
	networksGets = 0;
	containersGets = 0;
	connectRequests = { count: 0, url: "", body: null };
}

function mockLists() {
	return [
		http.get("*/api/machines/m-1/containers", () => {
			containersGets++;
			return HttpResponse.json({
				containers: [attached],
				total: 1,
				timestamp: "2026-08-26T00:00:00Z",
			});
		}),
		http.get("*/api/machines/m-1/networks", () => {
			networksGets++;
			return HttpResponse.json([
				networkFixture(),
				networkFixture({ name: "other-net", subnets: ["10.89.0.0/16"] }),
			]);
		}),
	];
}

function connectHandler(status = 200, body?: JsonBodyType) {
	return http.post(
		"*/api/machines/m-1/networks/other-net/connect",
		async ({ request }) => {
			connectRequests.count++;
			connectRequests.url = request.url;
			connectRequests.body = await request.json();
			if (status !== 200) return HttpResponse.json(body, { status });
			return new HttpResponse(null, { status: 200 });
		},
	);
}

async function openConnectDialog() {
	renderWithQuery(<ContainerList machine={machineFixture()} />, {
		router: true,
	});
	const row = await screen.findByTestId("container-row");
	const user = userEvent.setup();
	await user.click(
		within(row).getByRole("button", { name: "Connect to network web" }),
	);
	expect(await screen.findByTestId("modal")).toBeInTheDocument();
	return user;
}

async function pickNetwork(user: ReturnType<typeof userEvent.setup>) {
	await user.click(screen.getByTestId("searchable-select-input"));
	await user.click(screen.getByTestId("searchable-select-option-other-net"));
}

describe("ContainerList connect to network", () => {
	it("shows a Connect to network button per row at write level and opens the dialog", async () => {
		resetCounters();
		server.use(...mockLists(), connectHandler());
		renderWithQuery(<ContainerList machine={machineFixture()} />, {
			router: true,
		});

		const row = await screen.findByTestId("container-row");
		const user = userEvent.setup();
		await user.click(
			within(row).getByRole("button", { name: "Connect to network web" }),
		);

		expect(await screen.findByTestId("modal")).toBeInTheDocument();
		expect(screen.getByTestId("connect-network-submit")).toBeInTheDocument();
	});

	it("hides the button below manage-networks while Logs stays visible", async () => {
		resetCounters();
		server.use(...mockLists(), connectHandler());
		renderWithQuery(
			<ContainerList
				machine={machineFixture({ permission: "read", isOwner: false })}
			/>,
			{ router: true },
		);

		const row = await screen.findByTestId("container-row");
		expect(
			within(row).queryByRole("button", { name: /connect to network/i }),
		).not.toBeInTheDocument();
		expect(
			within(row).getByRole("button", { name: /logs/i }),
		).toBeInTheDocument();
	});

	it("offers the machine's networks excluding the container's current ones", async () => {
		resetCounters();
		server.use(...mockLists(), connectHandler());
		const user = await openConnectDialog();

		await user.click(screen.getByTestId("searchable-select-input"));
		expect(
			await screen.findByTestId("searchable-select-option-other-net"),
		).toBeInTheDocument();
		expect(
			screen.queryByTestId("searchable-select-option-app-net"),
		).not.toBeInTheDocument();
	});

	it("keeps submit disabled until a network is chosen, then sends {container} to the connect endpoint", async () => {
		resetCounters();
		server.use(...mockLists(), connectHandler());
		const user = await openConnectDialog();

		expect(screen.getByTestId("connect-network-submit")).toBeDisabled();
		await pickNetwork(user);
		await waitFor(() =>
			expect(screen.getByTestId("connect-network-submit")).toBeEnabled(),
		);
		await user.click(screen.getByTestId("connect-network-submit"));

		await waitFor(() => expect(connectRequests.count).toBe(1));
		expect(
			connectRequests.url.endsWith(
				"/api/machines/m-1/networks/other-net/connect",
			),
		).toBe(true);
		expect(connectRequests.body).toEqual({ container: "c-1" });
	});

	it("refetches both the networks and the containers lists and closes on success", async () => {
		resetCounters();
		server.use(...mockLists(), connectHandler());
		const user = await openConnectDialog();

		// The dialog's networks query has landed before the mutation fires.
		await waitFor(() => expect(networksGets).toBe(1));
		expect(containersGets).toBe(1);
		await pickNetwork(user);
		await user.click(screen.getByTestId("connect-network-submit"));

		// Both lists refetch: the containers DTO carries each container's
		// networks, so the attach is visible from both tabs.
		await waitFor(() => expect(networksGets).toBe(2));
		await waitFor(() => expect(containersGets).toBe(2));
		await waitFor(() =>
			expect(screen.queryByTestId("modal")).not.toBeInTheDocument(),
		);
	});

	it("renders the error envelope and stays open", async () => {
		resetCounters();
		server.use(
			...mockLists(),
			connectHandler(
				404,
				errorJson("NETWORK_NOT_FOUND", "network not found", "req-12"),
			),
		);
		const user = await openConnectDialog();

		await pickNetwork(user);
		await user.click(screen.getByTestId("connect-network-submit"));

		const error = await screen.findByTestId("mutation-dialog-error");
		expect(error).toHaveTextContent("NETWORK_NOT_FOUND");
		expect(error).toHaveTextContent("network not found");
		expect(screen.getByTestId("modal")).toBeInTheDocument();
	});
});
