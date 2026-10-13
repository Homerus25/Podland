import { ConnectDialog } from "@features/networks/components/ConnectDialog";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http, type JsonBodyType } from "msw";
import { describe, expect, it } from "vitest";
import { errorJson } from "@/test/errors";
import { containerFixture, networkFixture } from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

// c-1 is already attached to app-net and c-3 is stopped; only c-2 is
// eligible to connect to it.
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
		http.get("*/api/machines/m-1/networks", () => {
			networksGets++;
			return HttpResponse.json([
				networkFixture(),
				networkFixture({ name: "other-net", subnets: ["10.89.0.0/16"] }),
			]);
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
		"*/api/machines/m-1/networks/:networkName/connect",
		async ({ request }) => {
			connectRequests.count++;
			connectRequests.url = request.url;
			connectRequests.body = await request.json();
			if (status !== 200) return HttpResponse.json(body, { status });
			return new HttpResponse(null, { status: 200 });
		},
	);
}

// The Networks tab's flow: the network is fixed, the dialog picks the
// container.
function renderNetworkFixed() {
	return renderWithQuery(
		<ConnectDialog
			machineId="m-1"
			network={networkFixture()}
			onClose={() => {}}
		/>,
	);
}

// The Containers tab's flow: the container is fixed, the dialog picks the
// network.
function renderContainerFixed() {
	return renderWithQuery(
		<ConnectDialog machineId="m-1" container={attached} onClose={() => {}} />,
	);
}

async function pickContainer(user: ReturnType<typeof userEvent.setup>) {
	await user.click(screen.getByTestId("searchable-select-input"));
	await user.click(screen.getByTestId("searchable-select-option-c-2"));
}

async function pickNetwork(user: ReturnType<typeof userEvent.setup>) {
	await user.click(screen.getByTestId("searchable-select-input"));
	await user.click(screen.getByTestId("searchable-select-option-other-net"));
}

describe("ConnectDialog with the network fixed", () => {
	it("offers running containers only, excluding the ones already attached", async () => {
		resetCounters();
		server.use(...mockLists(), connectHandler());
		renderNetworkFixed();
		const user = userEvent.setup();

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

	it("keeps submit disabled until a container is picked, then sends {container} to the connect endpoint", async () => {
		resetCounters();
		server.use(...mockLists(), connectHandler());
		renderNetworkFixed();
		const user = userEvent.setup();

		expect(screen.getByTestId("connect-container-submit")).toBeDisabled();
		await pickContainer(user);
		await waitFor(() =>
			expect(screen.getByTestId("connect-container-submit")).toBeEnabled(),
		);
		await user.click(screen.getByTestId("connect-container-submit"));

		await waitFor(() => expect(connectRequests.count).toBe(1));
		expect(
			connectRequests.url.endsWith(
				"/api/machines/m-1/networks/app-net/connect",
			),
		).toBe(true);
		expect(connectRequests.body).toEqual({ container: "c-2" });
	});

	it("refetches both lists on success", async () => {
		resetCounters();
		server.use(...mockLists(), connectHandler());
		renderNetworkFixed();
		const user = userEvent.setup();

		// The dialog's own queries have landed before the mutation fires.
		await waitFor(() => expect(containersGets).toBe(1));
		await waitFor(() => expect(networksGets).toBe(1));
		await pickContainer(user);
		await user.click(screen.getByTestId("connect-container-submit"));

		// Both lists refetch: the containers DTO carries each container's
		// networks, so the attach is visible from both tabs.
		await waitFor(() => expect(containersGets).toBe(2));
		await waitFor(() => expect(networksGets).toBe(2));
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
		renderNetworkFixed();
		const user = userEvent.setup();

		await pickContainer(user);
		await user.click(screen.getByTestId("connect-container-submit"));

		const error = await screen.findByTestId("mutation-dialog-error");
		expect(error).toHaveTextContent("CONTAINER_NOT_FOUND");
		expect(error).toHaveTextContent("container not found");
		expect(screen.getByTestId("modal")).toBeInTheDocument();
	});
});

describe("ConnectDialog with the container fixed", () => {
	it("offers the machine's networks excluding the container's current ones", async () => {
		resetCounters();
		server.use(...mockLists(), connectHandler());
		renderContainerFixed();
		const user = userEvent.setup();

		await user.click(screen.getByTestId("searchable-select-input"));
		expect(
			await screen.findByTestId("searchable-select-option-other-net"),
		).toBeInTheDocument();
		expect(
			screen.queryByTestId("searchable-select-option-app-net"),
		).not.toBeInTheDocument();
	});

	it("keeps submit disabled until a network is picked, then sends {container} to the connect endpoint", async () => {
		resetCounters();
		server.use(...mockLists(), connectHandler());
		renderContainerFixed();
		const user = userEvent.setup();

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

	it("refetches both lists on success", async () => {
		resetCounters();
		server.use(...mockLists(), connectHandler());
		renderContainerFixed();
		const user = userEvent.setup();

		// The dialog's own queries have landed before the mutation fires.
		await waitFor(() => expect(containersGets).toBe(1));
		await waitFor(() => expect(networksGets).toBe(1));
		await pickNetwork(user);
		await user.click(screen.getByTestId("connect-network-submit"));

		// Both lists refetch: the containers DTO carries each container's
		// networks, so the attach is visible from both tabs.
		await waitFor(() => expect(containersGets).toBe(2));
		await waitFor(() => expect(networksGets).toBe(2));
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
		renderContainerFixed();
		const user = userEvent.setup();

		await pickNetwork(user);
		await user.click(screen.getByTestId("connect-network-submit"));

		const error = await screen.findByTestId("mutation-dialog-error");
		expect(error).toHaveTextContent("NETWORK_NOT_FOUND");
		expect(error).toHaveTextContent("network not found");
		expect(screen.getByTestId("modal")).toBeInTheDocument();
	});
});
