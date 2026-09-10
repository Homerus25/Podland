import { ChakraProvider, theme } from "@chakra-ui/react";
import { ImageList } from "@features/images/components/ImageList";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http, type JsonBodyType } from "msw";
import { beforeEach, describe, expect, it } from "vitest";
import { imageFixture } from "@/test/factories";
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
	const queryClient = new QueryClient({
		defaultOptions: { queries: { retry: false } },
	});
	render(
		<ChakraProvider theme={theme}>
			<QueryClientProvider client={queryClient}>
				<ImageList machineId="m-1" permission={permission} />
			</QueryClientProvider>
		</ChakraProvider>,
	);
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
		expect(hint).toHaveAttribute("data-permission", "read");
		expect(pruneRequests.count).toBe(0);
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

	it("prunes on confirm, refreshes the list, and reports removed images with reclaimed sizes", async () => {
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
		const report = await screen.findByTestId("prune-report");
		expect(within(report).getByText("aaaaaaaaaaaa")).toBeInTheDocument();
		expect(within(report).getByText("bbbbbbbbbbbb")).toBeInTheDocument();
		expect(within(report).getByText("1 KB")).toBeInTheDocument();
		expect(within(report).getByText("2 KB")).toBeInTheDocument();
	});

	it("lists per-image errors alongside removed images in the report", async () => {
		installHandlers(() => ({
			status: 200,
			body: [
				{
					id: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
					size: 1024,
				},
				{
					id: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
					size: 0,
					error: "image is in use",
				},
			],
		}));
		const user = await openPruneModal();

		await user.click(screen.getByTestId("confirm-prune-button"));

		expect(await screen.findByTestId("prune-report")).toBeInTheDocument();
		const report = screen.getByTestId("prune-report");
		expect(within(report).getByText("aaaaaaaaaaaa")).toBeInTheDocument();
		expect(within(report).getByText("bbbbbbbbbbbb")).toBeInTheDocument();
		expect(within(report).getByText("image is in use")).toBeInTheDocument();
	});

	it("reports nothing to prune when the result is empty", async () => {
		installHandlers();
		const user = await openPruneModal();

		await user.click(screen.getByTestId("confirm-prune-button"));

		expect(await screen.findByText(/nothing to prune/i)).toBeInTheDocument();
	});
});
