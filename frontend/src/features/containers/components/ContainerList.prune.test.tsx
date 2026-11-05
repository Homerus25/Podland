import { ContainerList } from "@features/containers/components/ContainerList";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http, type JsonBodyType } from "msw";
import { beforeEach, describe, expect, it } from "vitest";
import { containerFixture, hostFixture } from "@/test/factories";
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
		http.get("*/api/hosts/m-1/containers", () => {
			listRequests.count += 1;
			return HttpResponse.json({
				containers: [
					containerFixture({
						id: "stopped-a",
						names: ["/stopped-a"],
						state: "exited",
					}),
					containerFixture({
						id: "stopped-b",
						names: ["/stopped-b"],
						state: "exited",
					}),
				],
				total: 2,
				timestamp: "2026-10-26T00:00:00Z",
			});
		}),
		http.post("*/api/hosts/m-1/containers/prune", ({ request }) => {
			pruneRequests.count += 1;
			pruneRequests.pathname = new URL(request.url).pathname;
			const response = pruneResponse();
			return HttpResponse.json(response.body, { status: response.status });
		}),
	);
}

function renderContainerList(permission: "read" | "write" | "admin" = "admin") {
	renderWithQuery(
		<ContainerList
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
	renderContainerList(permission);
	await screen.findByText("stopped-a");
	const user = userEvent.setup();
	await user.click(screen.getByTestId("prune-containers-button"));
	expect(await screen.findByTestId("modal")).toBeInTheDocument();
	return user;
}

describe("ContainerList prune", () => {
	beforeEach(resetCounters);

	it("hides the prune action from read-level users", async () => {
		installHandlers();
		renderContainerList("read");

		await screen.findByText("stopped-a");

		expect(
			screen.queryByTestId("prune-containers-button"),
		).not.toBeInTheDocument();
		expect(pruneRequests.count).toBe(0);
	});

	it("opens a danger confirmation and sends no prune request before confirming", async () => {
		installHandlers();
		await openPruneModal();

		expect(screen.getAllByText(/unused containers/i).length).toBeGreaterThan(0);
		expect(pruneRequests.count).toBe(0);
	});

	it("prunes on confirm, refreshes the list, and shows the report", async () => {
		installHandlers(() => ({
			status: 200,
			body: [
				{ id: "stopped-a", size: 1024 },
				{ id: "stopped-b", size: 2048 },
			],
		}));
		const user = await openPruneModal();

		await user.click(screen.getByTestId("confirm-prune-button"));

		await waitFor(() => expect(pruneRequests.count).toBe(1));
		expect(pruneRequests.pathname).toBe("/api/hosts/m-1/containers/prune");
		expect(listRequests.count).toBeGreaterThanOrEqual(2);
		expect(await screen.findByTestId("prune-report")).toBeInTheDocument();
	});
});
