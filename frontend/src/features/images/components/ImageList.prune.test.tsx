import { ImageList } from "@features/images/components/ImageList";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http, type JsonBodyType } from "msw";
import { beforeEach, describe, expect, it } from "vitest";
import { imageFixture, machineFixture } from "@/test/factories";
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
		http.get("*/api/machines/m-1/images", () => {
			listRequests.count += 1;
			return HttpResponse.json([
				imageFixture(),
				imageFixture({
					id: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
					repoTags: ["docker.io/library/redis:7"],
				}),
			]);
		}),
		http.post("*/api/machines/m-1/images/prune", ({ request }) => {
			pruneRequests.count += 1;
			pruneRequests.pathname = new URL(request.url).pathname;
			const response = pruneResponse();
			return HttpResponse.json(response.body, { status: response.status });
		}),
	);
}

function renderImageList(permission: "read" | "write" | "admin" = "admin") {
	renderWithQuery(<ImageList machine={machineFixture({ permission })} />, {
		chakra: true,
		router: { initialEntries: ["/machines/m-1"] },
	});
}

async function openPruneModal(
	permission: "read" | "write" | "admin" = "admin",
) {
	renderImageList(permission);
	await screen.findByText("docker.io/library/nginx:1.27");
	const user = userEvent.setup();
	await user.click(screen.getByTestId("prune-images-button"));
	expect(await screen.findByTestId("modal")).toBeInTheDocument();
	return user;
}

describe("ImageList prune", () => {
	beforeEach(resetCounters);

	it("hides the pull form and the prune action from read-level users", async () => {
		installHandlers();
		renderImageList("read");

		await screen.findByText("docker.io/library/nginx:1.27");

		expect(screen.queryByTestId("prune-images-button")).not.toBeInTheDocument();
		expect(screen.queryByTestId("pull-image-form")).not.toBeInTheDocument();
		const hint = screen.getByTestId("images-permission-hint");
		expect(hint).toHaveAttribute("data-requirement", "write");
		expect(pruneRequests.count).toBe(0);
	});

	it('labels the toolbar prune button "Prune", matching the other tabs', async () => {
		installHandlers();
		renderImageList();

		await screen.findByText("docker.io/library/nginx:1.27");

		expect(screen.getByTestId("prune-images-button")).toHaveTextContent(
			/^Prune$/,
		);
	});

	it("opens a danger confirmation and sends no prune request before confirming", async () => {
		installHandlers();
		await openPruneModal();

		expect(screen.getAllByText(/unused images/i).length).toBeGreaterThan(0);
		expect(
			screen.getByText(/in use by containers are safe/i),
		).toBeInTheDocument();
		expect(pruneRequests.count).toBe(0);
	});

	it("prunes on confirm, refreshes the list, and shows the report", async () => {
		installHandlers(() => ({
			status: 200,
			body: [
				{
					id: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
					size: 1024,
				},
				{
					id: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
					size: 2048,
				},
			],
		}));
		const user = await openPruneModal();

		await user.click(screen.getByTestId("confirm-prune-button"));

		await waitFor(() => expect(pruneRequests.count).toBe(1));
		expect(pruneRequests.pathname).toBe("/api/machines/m-1/images/prune");
		expect(listRequests.count).toBeGreaterThanOrEqual(2);
		expect(await screen.findByTestId("prune-report")).toBeInTheDocument();
	});
});
