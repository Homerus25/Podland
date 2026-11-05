import { NetworkList } from "@features/networks/components/NetworkList";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http, type JsonBodyType } from "msw";
import { beforeEach, describe, expect, it } from "vitest";
import { hostFixture, networkFixture } from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

let listRequests: { count: number } = { count: 0 };
let pruneRequests: { count: number; pathname: string } = {
	count: 0,
	pathname: "",
};

function resetCounters() {
	listRequests = { count: 0 };
	pruneRequests = { count: 0, pathname: "" };
}

function installHandlers(
	pruneResponse: () => {
		status: number;
		body: JsonBodyType;
	} = () => ({
		status: 200,
		body: [],
	}),
) {
	server.use(
		http.get("*/api/hosts/m-1/networks", () => {
			listRequests.count += 1;
			return HttpResponse.json([
				networkFixture({ name: "unused-a" }),
				networkFixture({ name: "unused-b" }),
			]);
		}),
		http.post("*/api/hosts/m-1/networks/prune", ({ request }) => {
			pruneRequests.count += 1;
			pruneRequests.pathname = new URL(request.url).pathname;
			const response = pruneResponse();
			return HttpResponse.json(response.body, { status: response.status });
		}),
	);
}

function renderNetworkList(permission: "read" | "write" | "admin" = "admin") {
	renderWithQuery(
		<NetworkList
			host={hostFixture({ permission, isOwner: permission === "admin" })}
		/>,
		{
			chakra: true,
			router: true,
		},
	);
}

async function openPruneModal(
	permission: "read" | "write" | "admin" = "admin",
) {
	renderNetworkList(permission);
	await screen.findByText("unused-a");
	const user = userEvent.setup();
	await user.click(screen.getByTestId("prune-networks-button"));
	expect(await screen.findByTestId("modal")).toBeInTheDocument();
	return user;
}

describe("NetworkList prune", () => {
	beforeEach(resetCounters);

	it("hides the prune action from read-level users", async () => {
		installHandlers();
		renderNetworkList("read");

		await screen.findByText("unused-a");

		expect(
			screen.queryByTestId("prune-networks-button"),
		).not.toBeInTheDocument();
		expect(pruneRequests.count).toBe(0);
	});

	it("opens a danger confirmation and sends no prune request before confirming", async () => {
		installHandlers();
		await openPruneModal();

		expect(screen.getAllByText(/unused networks/i).length).toBeGreaterThan(0);
		expect(pruneRequests.count).toBe(0);
	});

	it("prunes on confirm, refreshes the list, and shows the report", async () => {
		installHandlers(() => ({
			status: 200,
			body: [
				{ name: "unused-a" },
				{ name: "unused-b", error: "network is in use" },
			],
		}));
		const user = await openPruneModal();

		await user.click(screen.getByTestId("confirm-prune-button"));

		await waitFor(() => expect(pruneRequests.count).toBe(1));
		expect(pruneRequests.pathname).toBe("/api/hosts/m-1/networks/prune");
		expect(listRequests.count).toBeGreaterThanOrEqual(2);
		expect(await screen.findByTestId("prune-report")).toBeInTheDocument();
	});
});
