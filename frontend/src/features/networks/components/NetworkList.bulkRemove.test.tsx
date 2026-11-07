import { useContainers } from "@features/containers/hooks/useContainers";
import { NetworkList } from "@features/networks/components/NetworkList";
import type { NetworkSummary } from "@features/networks/services/networksApi";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http, type JsonBodyType } from "msw";
import { useLocation } from "react-router-dom";
import { beforeEach, describe, expect, it } from "vitest";
import { errorResponse } from "@/test/errors";
import {
	containerFixture,
	hostFixture,
	networkFixture,
} from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

// The bulk-remove flow (spec 461, ticket #469) at the MSW/HTTP seam: the
// request shapes (the plain first pass, the forced retry carrying only the
// failures), the per-entry report, the list refetch, and the selection
// clearing. The dialog skeleton itself is pinned once in the shared
// ReportDialog suite; this suite keeps the network domain assertions —
// including the both-lists invalidation the single-item delete already
// carries, because the containers DTO holds each container's networks.

const victimA = networkFixture({ name: "victim-a" });
const victimB = networkFixture({ name: "victim-b" });

type RemoveBody = { ids: string[]; force?: boolean };

let listRequests: { count: number; networks: () => NetworkSummary[] } = {
	count: 0,
	networks: () => [],
};
let removeRequests: {
	count: number;
	pathname: string;
	bodies: RemoveBody[];
} = { count: 0, pathname: "", bodies: [] };
let containerListRequests = 0;

function resetCounters() {
	listRequests = { count: 0, networks: () => [victimA, victimB] };
	removeRequests = { count: 0, pathname: "", bodies: [] };
	containerListRequests = 0;
}

function installHandlers(
	removeResponse: (body: RemoveBody) => JsonBodyType = (body) =>
		body.ids.map((id) => ({ id, success: true })),
	removeOnSuccess = false,
) {
	server.use(
		http.get("*/api/hosts/m-1/networks", () => {
			listRequests.count += 1;
			return HttpResponse.json(listRequests.networks());
		}),
		http.get("*/api/hosts/m-1/containers", () => {
			containerListRequests += 1;
			return HttpResponse.json({
				containers: [containerFixture()],
				total: 1,
				timestamp: "2026-08-26T00:00:00Z",
			});
		}),
		http.post("*/api/hosts/m-1/networks/remove", async ({ request }) => {
			removeRequests.count += 1;
			removeRequests.pathname = new URL(request.url).pathname;
			const body = (await request.json()) as RemoveBody;
			removeRequests.bodies.push(body);
			if (removeOnSuccess) {
				const removed = new Set(body.ids);
				const before = listRequests.networks;
				listRequests.networks = () =>
					before().filter((network) => !removed.has(network.name));
			}
			return HttpResponse.json(removeResponse(body));
		}),
	);
}

// The containers list is the second half of the bulk remove's invalidation
// (the containers DTO carries each container's networks). The Networks tab
// does not render it, so this probe mounts the query to make the refetch
// observable through MSW — the invalidation only refetches active queries.
function ContainersListProbe() {
	useContainers("m-1");
	return null;
}

function LocationProbe() {
	const location = useLocation();
	return <div data-testid="location">{location.pathname}</div>;
}

function renderNetworkList(permission: "read" | "write" | "admin" = "admin") {
	renderWithQuery(
		<>
			<LocationProbe />
			<NetworkList
				host={hostFixture({ permission, isOwner: permission === "admin" })}
			/>
			<ContainersListProbe />
		</>,
		{ chakra: true, router: { initialEntries: ["/hosts/m-1"] } },
	);
}

async function openBulkRemove(...names: string[]) {
	renderNetworkList();
	await screen.findByText(names[0]);
	const user = userEvent.setup();
	for (const name of names) {
		await user.click(screen.getByRole("checkbox", { name: `Select ${name}` }));
	}
	await user.click(screen.getByTestId("remove-selected-networks-button"));
	expect(await screen.findByTestId("modal")).toBeInTheDocument();
	return user;
}

describe("NetworkList bulk remove", () => {
	beforeEach(resetCounters);

	it("renders Remove selected permanently, disabled while nothing is selected", async () => {
		installHandlers();
		renderNetworkList();
		await screen.findByText("victim-a");
		const user = userEvent.setup();

		expect(
			screen.getByTestId("remove-selected-networks-button"),
		).toBeDisabled();

		const checkbox = screen.getByRole("checkbox", { name: "Select victim-a" });
		await user.click(checkbox);
		expect(screen.getByTestId("remove-selected-networks-button")).toBeEnabled();

		await user.click(checkbox);
		expect(
			screen.getByTestId("remove-selected-networks-button"),
		).toBeDisabled();
	});

	it("hides the selection from read-level users", async () => {
		installHandlers();
		renderNetworkList("read");
		await screen.findByText("victim-a");

		expect(screen.queryAllByRole("checkbox")).toHaveLength(0);
		expect(
			screen.queryByTestId("remove-selected-networks-button"),
		).not.toBeInTheDocument();
	});

	it("selects a row without navigating to its detail page", async () => {
		installHandlers();
		renderNetworkList();
		await screen.findByText("victim-a");
		const user = userEvent.setup();

		await user.click(screen.getByRole("checkbox", { name: "Select victim-a" }));

		expect(screen.getByTestId("location").textContent).toBe("/hosts/m-1");
	});

	it("confirms with the count and the selected names, sending nothing yet", async () => {
		installHandlers();
		await openBulkRemove("victim-a", "victim-b");

		expect(screen.getByText(/remove 2 networks/i)).toBeInTheDocument();
		const names = screen.getByTestId("bulk-remove-names");
		expect(within(names).getByText("victim-a")).toBeInTheDocument();
		expect(within(names).getByText("victim-b")).toBeInTheDocument();
		expect(removeRequests.count).toBe(0);
	});

	it("sends one plain bulk request with the selected names and refetches both lists", async () => {
		installHandlers(undefined, true);
		const user = await openBulkRemove("victim-a", "victim-b");
		const listCountBefore = listRequests.count;
		const containerCountBefore = containerListRequests;

		await user.click(screen.getByTestId("confirm-bulk-remove-button"));

		await waitFor(() => expect(removeRequests.count).toBe(1));
		expect(removeRequests.pathname).toBe("/api/hosts/m-1/networks/remove");
		// The first pass is plain: the body carries no force at all.
		expect(removeRequests.bodies[0]).toEqual({
			ids: ["victim-a", "victim-b"],
		});
		await waitFor(() =>
			expect(listRequests.count).toBeGreaterThan(listCountBefore),
		);
		await waitFor(() =>
			expect(
				containerListRequests,
				"the containers list must refetch too: its DTO carries each container's networks",
			).toBeGreaterThan(containerCountBefore),
		);
	});

	it("reports every entry, successes and failures with their messages", async () => {
		installHandlers((body) =>
			body.ids.map((id) =>
				id === "victim-b"
					? {
							id,
							success: false,
							error: {
								code: "NETWORK_IN_USE",
								message: "network is in use: container abc is attached",
							},
						}
					: { id, success: true },
			),
		);
		const user = await openBulkRemove("victim-a", "victim-b");

		await user.click(screen.getByTestId("confirm-bulk-remove-button"));

		const report = await screen.findByTestId("bulk-remove-report");
		const rows = within(report).getAllByTestId("bulk-remove-report-row");
		expect(rows).toHaveLength(2);
		expect(within(rows[0]).getByText("victim-a")).toBeInTheDocument();
		expect(within(rows[0]).getByText("Removed")).toBeInTheDocument();
		expect(within(rows[1]).getByText("victim-b")).toBeInTheDocument();
		expect(
			within(rows[1]).getByText("network is in use: container abc is attached"),
		).toBeInTheDocument();
	});

	it("force-removes only the failed names on the retry", async () => {
		installHandlers((body) =>
			body.ids.map((id) =>
				id === "victim-b" && !body.force
					? {
							id,
							success: false,
							error: {
								code: "NETWORK_IN_USE",
								message: "network is in use: container abc is attached",
							},
						}
					: { id, success: true },
			),
		);
		const user = await openBulkRemove("victim-a", "victim-b");

		await user.click(screen.getByTestId("confirm-bulk-remove-button"));

		const retry = await screen.findByTestId("force-remove-failed-button");
		expect(retry).toHaveTextContent("Force remove 1 failed");
		await user.click(retry);

		await waitFor(() => expect(removeRequests.count).toBe(2));
		expect(removeRequests.bodies[1]).toEqual({
			ids: ["victim-b"],
			force: true,
		});
		await waitFor(() =>
			expect(
				screen.queryByTestId("force-remove-failed-button"),
			).not.toBeInTheDocument(),
		);
	});

	// #468: "The list refetches after the run; the selection clears." The
	// clear rides the run's success, not the dialog's close, so the report
	// stays open over a list that has already let go of its selection. The
	// toolbar button is the selection's visible edge and, unlike the row
	// checkboxes, it stays queryable while the modal hides the page —
	// rendered permanently (ticket #493), its disabled state is the
	// cleared selection's evidence.
	it("clears the selection after a successful run, while the report stays open", async () => {
		installHandlers();
		const user = await openBulkRemove("victim-a", "victim-b");

		await user.click(screen.getByTestId("confirm-bulk-remove-button"));
		await screen.findByTestId("bulk-remove-report");

		expect(screen.getByTestId("modal")).toBeInTheDocument();
		expect(
			screen.getByTestId("remove-selected-networks-button"),
		).toBeDisabled();

		await user.click(screen.getByTestId("close-bulk-remove-report-button"));
		for (const checkbox of screen.getAllByRole("checkbox")) {
			expect(checkbox).not.toBeChecked();
		}
	});

	// A failed run removed nothing, so the selection survives: the user
	// retries the same rows without re-picking them.
	it("keeps the selection when the run fails", async () => {
		installHandlers();
		server.use(
			http.post("*/api/hosts/m-1/networks/remove", () =>
				errorResponse(500, "INTERNAL_ERROR", "boom"),
			),
		);
		const user = await openBulkRemove("victim-a", "victim-b");

		await user.click(screen.getByTestId("confirm-bulk-remove-button"));

		expect(await screen.findByRole("alert")).toBeInTheDocument();
		expect(
			screen.getByTestId("remove-selected-networks-button"),
		).toBeInTheDocument();

		await user.click(screen.getByRole("button", { name: "Cancel" }));
		for (const checkbox of screen.getAllByRole("checkbox")) {
			expect(checkbox).toBeChecked();
		}
	});
});
