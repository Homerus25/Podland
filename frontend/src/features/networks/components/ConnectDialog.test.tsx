import { ConnectDialog } from "@features/networks/components/ConnectDialog";
import { QueryClient } from "@tanstack/react-query";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http, type JsonBodyType } from "msw";
import { useState } from "react";
import { describe, expect, it } from "vitest";
import { errorJson } from "@/test/errors";
import { containerFixture, networkFixture } from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

// c-1 is already attached to app-net, c-3 is stopped; c-2 and c-4 are
// eligible to connect to it.
const attached = containerFixture({
	id: "c-1",
	names: ["/web"],
	networks: ["app-net"],
});
const eligible = containerFixture({ id: "c-2", names: ["/db"], networks: [] });
const eligible2 = containerFixture({
	id: "c-4",
	names: ["/queue"],
	networks: [],
});
const stopped = containerFixture({
	id: "c-3",
	names: ["/cache"],
	state: "exited",
	status: "Exited (0) 5 minutes ago",
});

// Networks successfully connected during the test, per container id, so
// the refetched containers list reflects the attaches and the dialog's
// eligible options shrink accordingly.
let extraNetworks: Record<string, string[]> = {};
let networksGets = 0;
let containersGets = 0;
let connectRequests: { url: string; networkName: string; body: unknown }[] = [];

function resetCounters() {
	extraNetworks = {};
	networksGets = 0;
	containersGets = 0;
	connectRequests = [];
}

function mockLists() {
	return [
		http.get("*/api/machines/m-1/networks", () => {
			networksGets++;
			return HttpResponse.json([
				networkFixture(),
				networkFixture({ name: "other-net", subnets: ["10.89.0.0/16"] }),
				networkFixture({ name: "third-net", subnets: ["10.90.0.0/16"] }),
			]);
		}),
		http.get("*/api/machines/m-1/containers", () => {
			containersGets++;
			const containers = [attached, eligible, eligible2, stopped].map(
				(container) => ({
					...container,
					networks: [
						...(container.networks ?? []),
						...(extraNetworks[container.id] ?? []),
					],
				}),
			);
			return HttpResponse.json({
				containers,
				total: containers.length,
				timestamp: "2026-08-26T00:00:00Z",
			});
		}),
	];
}

// A rule keyed "networkName/containerId": the pair answers with the error
// for its first `times` attempts (default: always), then connects.
type ConnectRule = { status: number; body: JsonBodyType; times?: number };

function connectHandler(rules: Record<string, ConnectRule> = {}) {
	const attempts: Record<string, number> = {};
	return http.post(
		"*/api/machines/m-1/networks/:networkName/connect",
		async ({ request, params }) => {
			const networkName = params.networkName as string;
			const body = (await request.json()) as { container: string };
			connectRequests.push({ url: request.url, networkName, body });
			const key = `${networkName}/${body.container}`;
			const rule = rules[key];
			if (rule) {
				attempts[key] = (attempts[key] ?? 0) + 1;
				if (rule.times === undefined || attempts[key] <= rule.times) {
					return HttpResponse.json(rule.body, { status: rule.status });
				}
			}
			extraNetworks[body.container] = [
				...(extraNetworks[body.container] ?? []),
				networkName,
			];
			return new HttpResponse(null, { status: 200 });
		},
	);
}

// The Networks tab's flow: the network is fixed, the dialog picks the
// containers.
function renderNetworkFixed(onClose: () => void = () => {}) {
	return renderWithQuery(
		<ConnectDialog
			machineId="m-1"
			network={networkFixture()}
			onClose={onClose}
		/>,
	);
}

// The Containers tab's flow: the container is fixed, the dialog picks the
// networks.
function renderContainerFixed(onClose: () => void = () => {}) {
	return renderWithQuery(
		<ConnectDialog machineId="m-1" container={attached} onClose={onClose} />,
	);
}

// A harness that unmounts the dialog on close, the way both list surfaces
// render it conditionally — so a success actually removes the modal.
function NetworkFixedHarness() {
	const [open, setOpen] = useState(true);
	return open ? (
		<ConnectDialog
			machineId="m-1"
			network={networkFixture()}
			onClose={() => setOpen(false)}
		/>
	) : null;
}

function ContainerFixedHarness() {
	const [open, setOpen] = useState(true);
	return open ? (
		<ConnectDialog
			machineId="m-1"
			container={attached}
			onClose={() => setOpen(false)}
		/>
	) : null;
}

// Multi-select picking: the list stays open between picks, so one input
// click serves the whole batch.
async function pickOptions(
	user: ReturnType<typeof userEvent.setup>,
	values: string[],
) {
	await user.click(screen.getByTestId("searchable-select-input"));
	for (const value of values) {
		await user.click(screen.getByTestId(`searchable-select-option-${value}`));
	}
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
			screen.getByTestId("searchable-select-option-c-4"),
		).toBeInTheDocument();
		expect(
			screen.queryByTestId("searchable-select-option-c-1"),
		).not.toBeInTheDocument();
		expect(
			screen.queryByTestId("searchable-select-option-c-3"),
		).not.toBeInTheDocument();
	});

	it("keeps submit disabled until at least one container is picked", async () => {
		resetCounters();
		server.use(...mockLists(), connectHandler());
		renderNetworkFixed();
		const user = userEvent.setup();

		expect(screen.getByTestId("connect-container-submit")).toBeDisabled();
		await pickOptions(user, ["c-2"]);
		await waitFor(() =>
			expect(screen.getByTestId("connect-container-submit")).toBeEnabled(),
		);
		// Removing the only chip empties the selection and disables submit.
		await user.click(screen.getByTestId("searchable-select-remove-c-2"));
		await waitFor(() =>
			expect(screen.getByTestId("connect-container-submit")).toBeDisabled(),
		);
	});

	it("fires one connect request per picked container", async () => {
		resetCounters();
		server.use(...mockLists(), connectHandler());
		renderNetworkFixed();
		const user = userEvent.setup();

		await pickOptions(user, ["c-2", "c-4"]);
		// Both picks render as chips and leave the option list.
		expect(
			screen.getByTestId("searchable-select-chip-c-2"),
		).toBeInTheDocument();
		expect(
			screen.getByTestId("searchable-select-chip-c-4"),
		).toBeInTheDocument();
		expect(
			screen.queryByTestId("searchable-select-option-c-2"),
		).not.toBeInTheDocument();
		expect(
			screen.queryByTestId("searchable-select-option-c-4"),
		).not.toBeInTheDocument();

		await user.click(screen.getByTestId("connect-container-submit"));

		await waitFor(() => expect(connectRequests).toHaveLength(2));
		expect(connectRequests.map((request) => request.body)).toEqual(
			expect.arrayContaining([{ container: "c-2" }, { container: "c-4" }]),
		);
		for (const request of connectRequests) {
			expect(request.networkName).toBe("app-net");
			expect(
				request.url.endsWith("/api/machines/m-1/networks/app-net/connect"),
			).toBe(true);
		}
	});

	it("closes and refetches both lists when every request succeeds", async () => {
		resetCounters();
		server.use(...mockLists(), connectHandler());
		renderWithQuery(<NetworkFixedHarness />);
		const user = userEvent.setup();

		// The dialog's own queries have landed before the mutation fires.
		await waitFor(() => expect(containersGets).toBe(1));
		await waitFor(() => expect(networksGets).toBe(1));
		await pickOptions(user, ["c-2", "c-4"]);
		await user.click(screen.getByTestId("connect-container-submit"));

		// All requests succeeded, so the dialog closes through onClose.
		await waitFor(() =>
			expect(screen.queryByTestId("modal")).not.toBeInTheDocument(),
		);
		// Both lists refetch: the containers DTO carries each container's
		// networks, so the attaches are visible from both tabs.
		await waitFor(() => expect(containersGets).toBe(2));
		await waitFor(() => expect(networksGets).toBe(2));
	});

	it("stays open with an error summary naming the failed items when a request fails; succeeded connections remain", async () => {
		resetCounters();
		server.use(
			...mockLists(),
			connectHandler({
				"app-net/c-2": {
					status: 404,
					body: errorJson(
						"CONTAINER_NOT_FOUND",
						"container not found",
						"req-11",
					),
				},
			}),
		);
		renderNetworkFixed();
		const user = userEvent.setup();

		await pickOptions(user, ["c-2", "c-4"]);
		await user.click(screen.getByTestId("connect-container-submit"));

		// Both requests fired — the succeeded connection is not rolled back.
		await waitFor(() => expect(connectRequests).toHaveLength(2));
		const error = await screen.findByTestId("mutation-dialog-error");
		expect(error).toHaveTextContent("1 connection failed");
		expect(error).toHaveTextContent("db");
		expect(error).toHaveTextContent("container not found");
		expect(screen.getByTestId("modal")).toBeInTheDocument();
	});

	it("refreshes the eligible options after a partial failure: succeeded items leave, failed items remain for retry", async () => {
		resetCounters();
		server.use(
			...mockLists(),
			connectHandler({
				"app-net/c-2": {
					status: 404,
					body: errorJson(
						"CONTAINER_NOT_FOUND",
						"container not found",
						"req-11",
					),
				},
			}),
		);
		renderNetworkFixed();
		const user = userEvent.setup();

		await pickOptions(user, ["c-2", "c-4"]);
		await user.click(screen.getByTestId("connect-container-submit"));
		await screen.findByTestId("mutation-dialog-error");

		// Both lists refetch after the settle.
		await waitFor(() => expect(containersGets).toBe(2));
		await waitFor(() => expect(networksGets).toBe(2));

		// The succeeded container is now attached: its chip is gone and it
		// never reappears as an option. The failed one stays selected.
		expect(
			screen.queryByTestId("searchable-select-chip-c-4"),
		).not.toBeInTheDocument();
		expect(
			screen.getByTestId("searchable-select-chip-c-2"),
		).toBeInTheDocument();
		await user.click(screen.getByTestId("searchable-select-input"));
		await user.click(screen.getByTestId("searchable-select-remove-c-2"));
		expect(
			await screen.findByTestId("searchable-select-option-c-2"),
		).toBeInTheDocument();
		expect(
			screen.queryByTestId("searchable-select-option-c-4"),
		).not.toBeInTheDocument();
	});

	it("retries only the failed items after a partial failure", async () => {
		resetCounters();
		server.use(
			...mockLists(),
			connectHandler({
				"app-net/c-2": {
					status: 404,
					body: errorJson(
						"CONTAINER_NOT_FOUND",
						"container not found",
						"req-11",
					),
					times: 1,
				},
			}),
		);
		renderWithQuery(<NetworkFixedHarness />);
		const user = userEvent.setup();

		await pickOptions(user, ["c-2", "c-4"]);
		await user.click(screen.getByTestId("connect-container-submit"));
		await screen.findByTestId("mutation-dialog-error");
		await waitFor(() => expect(connectRequests).toHaveLength(2));

		// The selection pruned to the failed item; the retry fires only it.
		await user.click(screen.getByTestId("connect-container-submit"));
		await waitFor(() => expect(connectRequests).toHaveLength(3));
		expect(connectRequests[2].body).toEqual({ container: "c-2" });
		// The retry succeeded, so the dialog closes.
		await waitFor(() =>
			expect(screen.queryByTestId("modal")).not.toBeInTheDocument(),
		);
	});

	it("issues fresh list requests every time the dialog opens, inside the five-minute cache window", async () => {
		resetCounters();
		server.use(...mockLists(), connectHandler());
		// The app-wide stale time is five minutes (main.tsx); mirroring it
		// pins the refetch inside the cache window.
		const queryClient = new QueryClient({
			defaultOptions: { queries: { retry: false, staleTime: 5 * 60 * 1000 } },
		});
		const first = renderWithQuery(
			<ConnectDialog
				machineId="m-1"
				network={networkFixture()}
				onClose={() => {}}
			/>,
			{ queryClient },
		);
		await waitFor(() => expect(containersGets).toBe(1));
		await waitFor(() => expect(networksGets).toBe(1));
		first.unmount();

		renderWithQuery(
			<ConnectDialog
				machineId="m-1"
				network={networkFixture()}
				onClose={() => {}}
			/>,
			{ queryClient },
		);
		await waitFor(() => expect(containersGets).toBe(2));
		await waitFor(() => expect(networksGets).toBe(2));
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
			screen.getByTestId("searchable-select-option-third-net"),
		).toBeInTheDocument();
		expect(
			screen.queryByTestId("searchable-select-option-app-net"),
		).not.toBeInTheDocument();
	});

	it("keeps submit disabled until at least one network is picked, then fires one request per picked network", async () => {
		resetCounters();
		server.use(...mockLists(), connectHandler());
		renderContainerFixed();
		const user = userEvent.setup();

		expect(screen.getByTestId("connect-network-submit")).toBeDisabled();
		await pickOptions(user, ["other-net", "third-net"]);
		await waitFor(() =>
			expect(screen.getByTestId("connect-network-submit")).toBeEnabled(),
		);
		await user.click(screen.getByTestId("connect-network-submit"));

		await waitFor(() => expect(connectRequests).toHaveLength(2));
		expect(connectRequests.map((request) => request.body)).toEqual([
			{ container: "c-1" },
			{ container: "c-1" },
		]);
		expect(
			connectRequests.map((request) => request.networkName).sort(),
		).toEqual(["other-net", "third-net"]);
		for (const request of connectRequests) {
			expect(
				request.url.endsWith(
					`/api/machines/m-1/networks/${request.networkName}/connect`,
				),
			).toBe(true);
		}
	});

	it("closes and refetches both lists when every request succeeds", async () => {
		resetCounters();
		server.use(...mockLists(), connectHandler());
		renderWithQuery(<ContainerFixedHarness />);
		const user = userEvent.setup();

		// The dialog's own queries have landed before the mutation fires.
		await waitFor(() => expect(containersGets).toBe(1));
		await waitFor(() => expect(networksGets).toBe(1));
		await pickOptions(user, ["other-net", "third-net"]);
		await user.click(screen.getByTestId("connect-network-submit"));

		// All requests succeeded, so the dialog closes through onClose.
		await waitFor(() =>
			expect(screen.queryByTestId("modal")).not.toBeInTheDocument(),
		);
		// Both lists refetch: the containers DTO carries each container's
		// networks, so the attaches are visible from both tabs.
		await waitFor(() => expect(containersGets).toBe(2));
		await waitFor(() => expect(networksGets).toBe(2));
	});

	it("stays open with an error summary naming the failed networks when a request fails", async () => {
		resetCounters();
		server.use(
			...mockLists(),
			connectHandler({
				"other-net/c-1": {
					status: 404,
					body: errorJson("NETWORK_NOT_FOUND", "network not found", "req-12"),
				},
			}),
		);
		renderContainerFixed();
		const user = userEvent.setup();

		await pickOptions(user, ["other-net", "third-net"]);
		await user.click(screen.getByTestId("connect-network-submit"));

		// Both requests fired — the succeeded connection is not rolled back.
		await waitFor(() => expect(connectRequests).toHaveLength(2));
		const error = await screen.findByTestId("mutation-dialog-error");
		expect(error).toHaveTextContent("1 connection failed");
		expect(error).toHaveTextContent("other-net");
		expect(error).toHaveTextContent("network not found");
		expect(screen.getByTestId("modal")).toBeInTheDocument();
	});
});
