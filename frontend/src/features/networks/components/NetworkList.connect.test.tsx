import { NetworkList } from "@features/networks/components/NetworkList";
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

// c-1 is already attached to app-net and c-3 is stopped; only c-2 is
// eligible to connect.
const attached = containerFixture({
	id: "c-1",
	names: ["/web"],
	networks: ["app-net"],
});
const eligible = containerFixture({ id: "c-2", names: ["/db"], networks: [] });
const stopped = containerFixture({
	id: "c-3",
	names: ["/cache"],
	state: "exited",
	status: "Exited (0) 5 minutes ago",
});

let networksGets = 0;
let containersGets = 0;
let connectRequests: { count: number; body: unknown } = {
	count: 0,
	body: null,
};

function resetCounters() {
	networksGets = 0;
	containersGets = 0;
	connectRequests = { count: 0, body: null };
}

function mockLists() {
	return [
		http.get("*/api/machines/m-1/networks", () => {
			networksGets++;
			return HttpResponse.json([networkFixture()]);
		}),
		http.get("*/api/machines/m-1/containers", () => {
			containersGets++;
			return HttpResponse.json({
				containers: [attached, eligible, stopped],
				total: 3,
				timestamp: "2026-08-26T00:00:00Z",
			});
		}),
	];
}

function connectHandler(status = 200, body?: JsonBodyType) {
	return http.post(
		"*/api/machines/m-1/networks/app-net/connect",
		async ({ request }) => {
			connectRequests.count++;
			connectRequests.body = await request.json();
			if (status !== 200) return HttpResponse.json(body, { status });
			return new HttpResponse(null, { status: 200 });
		},
	);
}

async function openConnectDialog() {
	renderWithQuery(<NetworkList machine={machineFixture()} />);
	const row = await screen.findByTestId("network-row");
	const user = userEvent.setup();
	await user.click(
		within(row).getByRole("button", { name: "Connect container app-net" }),
	);
	expect(await screen.findByTestId("modal")).toBeInTheDocument();
	return user;
}

describe("NetworkList connect", () => {
	it("opens the dialog from the row button with submit disabled until a container is chosen", async () => {
		resetCounters();
		server.use(...mockLists(), connectHandler());
		const user = await openConnectDialog();

		expect(screen.getByTestId("connect-container-submit")).toBeDisabled();
		await user.click(screen.getByTestId("searchable-select-input"));
		await user.click(screen.getByTestId("searchable-select-option-c-2"));
		await waitFor(() =>
			expect(screen.getByTestId("connect-container-submit")).toBeEnabled(),
		);
	});

	it("excludes already-attached and stopped containers", async () => {
		resetCounters();
		server.use(...mockLists(), connectHandler());
		const user = await openConnectDialog();

		await user.click(screen.getByTestId("searchable-select-input"));
		expect(
			await screen.findByTestId("searchable-select-option-c-2"),
		).toBeInTheDocument();
		expect(
			screen.queryByTestId("searchable-select-option-c-1"),
		).not.toBeInTheDocument();
		expect(
			screen.queryByTestId("searchable-select-option-c-3"),
		).not.toBeInTheDocument();
	});

	it("sends {container} to the connect endpoint and refetches both lists on success", async () => {
		resetCounters();
		server.use(...mockLists(), connectHandler());
		const user = await openConnectDialog();

		await user.click(screen.getByTestId("searchable-select-input"));
		await user.click(screen.getByTestId("searchable-select-option-c-2"));
		await user.click(screen.getByTestId("connect-container-submit"));

		await waitFor(() => expect(connectRequests.count).toBe(1));
		expect(connectRequests.body).toEqual({ container: "c-2" });
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
				errorJson("CONTAINER_NOT_FOUND", "container not found", "req-11"),
			),
		);
		const user = await openConnectDialog();

		await user.click(screen.getByTestId("searchable-select-input"));
		await user.click(screen.getByTestId("searchable-select-option-c-2"));
		await user.click(screen.getByTestId("connect-container-submit"));

		const error = await screen.findByTestId("mutation-dialog-error");
		expect(error).toHaveTextContent("CONTAINER_NOT_FOUND");
		expect(error).toHaveTextContent("container not found");
		expect(screen.getByTestId("modal")).toBeInTheDocument();
	});
});
