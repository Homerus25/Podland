import { useContainers } from "@features/containers/hooks/useContainers";
import { NetworkList } from "@features/networks/components/NetworkList";
import type { NetworkSummary } from "@features/networks/services/networksApi";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http, type JsonBodyType } from "msw";
import { beforeEach, describe, expect, it } from "vitest";
import { errorJson } from "@/test/errors";
import {
	containerFixture,
	hostFixture,
	networkFixture,
} from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

const remaining = networkFixture({ name: "survivor" });

// The networks list's next response body, swapped by a successful delete so
// the refetch proves the row disappears.
let listRequests: { count: number; nextNetworks: () => NetworkSummary[] } = {
	count: 0,
	nextNetworks: () => [networkFixture({ name: "victim" }), remaining],
};
let deleteRequests: {
	count: number;
	pathname: string;
	search: string;
} = { count: 0, pathname: "", search: "" };
// The containers list is the second half of the delete's invalidation: the
// containers DTO carries each container's networks, so a removed network
// changes those rows too.
let containerListRequests = 0;

function resetCounters() {
	listRequests = {
		count: 0,
		nextNetworks: () => [networkFixture({ name: "victim" }), remaining],
	};
	deleteRequests = { count: 0, pathname: "", search: "" };
	containerListRequests = 0;
}

function installHandlers({
	deleteResponse = () => ({ status: 204 }),
	removeOnDelete = false,
}: {
	deleteResponse?: () => { status: number; body?: JsonBodyType };
	removeOnDelete?: boolean;
} = {}) {
	server.use(
		http.get("*/api/hosts/m-1/networks", () => {
			listRequests.count += 1;
			return HttpResponse.json(listRequests.nextNetworks());
		}),
		http.get("*/api/hosts/m-1/containers", () => {
			containerListRequests += 1;
			return HttpResponse.json({
				containers: [containerFixture()],
				total: 1,
				timestamp: "2026-08-26T00:00:00Z",
			});
		}),
		http.delete("*/api/hosts/m-1/networks/*", ({ request }) => {
			deleteRequests.count += 1;
			const url = new URL(request.url);
			deleteRequests.pathname = url.pathname;
			deleteRequests.search = url.search;
			const response = deleteResponse();
			if (response.status !== 204) {
				return HttpResponse.json(response.body, { status: response.status });
			}
			if (removeOnDelete) {
				listRequests.nextNetworks = () => [remaining];
			}
			return new HttpResponse(null, { status: 204 });
		}),
	);
}

function renderNetworkList(permission: "read" | "write" | "admin" = "admin") {
	renderWithQuery(
		<>
			<NetworkList
				host={hostFixture({ permission, isOwner: permission === "admin" })}
			/>
			<ContainersListProbe />
		</>,
		{
			chakra: true,
			router: true,
		},
	);
}

// The containers list is the second half of the delete's invalidation (the
// containers DTO carries each container's networks). The Networks tab does
// not render it, so this probe mounts the query to make the refetch
// observable through MSW — the invalidation only refetches active queries.
// The cross-feature import is test-only and bends the connect ritual's
// "networks imports nothing new from containers" direction: the probe
// observes the refetch, it never renders containers into the Networks tab.
function ContainersListProbe() {
	useContainers("m-1");
	return null;
}

async function openDeleteModal(
	permission: "read" | "write" | "admin" = "admin",
) {
	renderNetworkList(permission);
	await screen.findByText("victim");
	const user = userEvent.setup();
	await user.click(screen.getByRole("button", { name: /delete victim/i }));
	expect(await screen.findByTestId("modal")).toBeInTheDocument();
	return user;
}

describe("NetworkList delete", () => {
	beforeEach(resetCounters);

	it("hides the delete action from read-level users", async () => {
		installHandlers();
		renderNetworkList("read");

		await screen.findByText("victim");

		expect(
			screen.queryByRole("button", { name: /delete victim/i }),
		).not.toBeInTheDocument();
		expect(deleteRequests.count).toBe(0);
	});

	it("opens a confirmation dialog naming the network and sends nothing before confirming", async () => {
		installHandlers();
		await openDeleteModal();

		expect(screen.getAllByText(/victim/).length).toBeGreaterThan(0);
		expect(deleteRequests.count).toBe(0);
	});

	it("deletes without force on confirm, refetches both lists, and toasts", async () => {
		installHandlers({ removeOnDelete: true });
		const user = await openDeleteModal();

		await user.click(screen.getByTestId("confirm-delete-button"));

		await waitFor(() => expect(deleteRequests.count).toBe(1));
		expect(deleteRequests.pathname).toBe("/api/hosts/m-1/networks/victim");
		expect(deleteRequests.search).toBe("");

		await waitFor(() =>
			expect(screen.queryByTestId("modal")).not.toBeInTheDocument(),
		);
		expect(await screen.findByText("survivor")).toBeInTheDocument();
		await waitFor(() =>
			expect(screen.queryByText("victim")).not.toBeInTheDocument(),
		);
		expect(
			await screen.findByText(/network deleted: victim/i),
		).toBeInTheDocument();
		expect(
			containerListRequests,
			"the containers list must refetch too: its DTO carries each container's networks",
		).toBeGreaterThanOrEqual(2);
	});

	it("explains the in-use refusal and force-deletes only on the retry", async () => {
		installHandlers({
			deleteResponse: () => ({
				status: 409,
				body: errorJson(
					"NETWORK_IN_USE",
					"network is in use: container abc is attached",
					"req-42",
				),
			}),
		});
		const user = await openDeleteModal();

		await user.click(screen.getByTestId("confirm-delete-button"));

		expect(
			await screen.findByTestId("network-in-use-warning"),
		).toBeInTheDocument();
		expect(deleteRequests.count).toBe(1);
		expect(
			deleteRequests.search,
			"the first confirm must not carry force",
		).toBe("");

		await user.click(screen.getByTestId("force-delete-button"));

		await waitFor(() => expect(deleteRequests.count).toBe(2));
		expect(deleteRequests.search).toBe("?force=true");
	});

	it("renders the delete button beside connect in the row's action cell", async () => {
		installHandlers();
		renderNetworkList();

		const [row] = await screen.findAllByTestId("network-row");
		const actions = within(row).getAllByRole("cell").at(-1);
		expect(actions).toBeDefined();
		expect(
			within(actions as HTMLElement).getByRole("button", {
				name: /connect container victim/i,
			}),
		).toBeInTheDocument();
		expect(
			within(actions as HTMLElement).getByRole("button", {
				name: /delete victim/i,
			}),
		).toBeInTheDocument();
	});
});
