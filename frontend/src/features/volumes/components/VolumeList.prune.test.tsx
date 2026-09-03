import { ChakraProvider, theme } from "@chakra-ui/react";
import { VolumeList } from "@features/volumes/components/VolumeList";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http, type JsonBodyType } from "msw";
import { beforeEach, describe, expect, it } from "vitest";
import { volumeFixture } from "@/test/factories";
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
		http.get("*/api/machines/m-1/volumes", () => {
			listRequests.count += 1;
			return HttpResponse.json([
				volumeFixture({ name: "unused-a", size: 1024 }),
				volumeFixture({ name: "unused-b", size: 2048 }),
			]);
		}),
		http.post("*/api/machines/m-1/volumes/prune", ({ request }) => {
			pruneRequests.count += 1;
			pruneRequests.pathname = new URL(request.url).pathname;
			const response = pruneResponse();
			return HttpResponse.json(response.body, { status: response.status });
		}),
	);
}

function renderVolumeList(permission: "read" | "write" | "admin" = "admin") {
	const queryClient = new QueryClient({
		defaultOptions: { queries: { retry: false } },
	});
	// The Chakra provider is required for useToast to render its portal.
	render(
		<ChakraProvider theme={theme}>
			<QueryClientProvider client={queryClient}>
				<VolumeList machineId="m-1" permission={permission} />
			</QueryClientProvider>
		</ChakraProvider>,
	);
}

async function openPruneModal(
	permission: "read" | "write" | "admin" = "admin",
) {
	renderVolumeList(permission);
	await screen.findByText("unused-a");
	const user = userEvent.setup();
	await user.click(screen.getByTestId("prune-volumes-button"));
	expect(await screen.findByTestId("modal")).toBeInTheDocument();
	return user;
}

describe("VolumeList prune", () => {
	beforeEach(resetCounters);

	it("hides the prune action from read-level users", async () => {
		installHandlers();
		renderVolumeList("read");

		await screen.findByText("unused-a");

		expect(
			screen.queryByTestId("prune-volumes-button"),
		).not.toBeInTheDocument();
		expect(pruneRequests.count).toBe(0);
	});

	it("opens a danger confirmation and sends no prune request before confirming", async () => {
		installHandlers();
		await openPruneModal();

		expect(screen.getAllByText(/unused volumes/i).length).toBeGreaterThan(0);
		expect(
			screen.getByText(/in use by containers are safe/i),
		).toBeInTheDocument();
		expect(pruneRequests.count).toBe(0);
	});

	it("prunes on confirm, refreshes the list, and reports removed volumes with sizes", async () => {
		installHandlers(() => ({
			status: 200,
			body: [
				{ name: "unused-a", size: 1024 },
				{ name: "unused-b", size: 2048 },
			],
		}));
		const user = await openPruneModal();

		await user.click(screen.getByTestId("confirm-prune-button"));

		await waitFor(() => expect(pruneRequests.count).toBe(1));
		expect(pruneRequests.pathname).toBe("/api/machines/m-1/volumes/prune");
		expect(listRequests.count).toBeGreaterThanOrEqual(2);
		const report = await screen.findByTestId("prune-report");
		expect(within(report).getByText("unused-a")).toBeInTheDocument();
		expect(within(report).getByText("unused-b")).toBeInTheDocument();
		expect(within(report).getByText("1 KB")).toBeInTheDocument();
		expect(within(report).getByText("2 KB")).toBeInTheDocument();
	});

	it("lists per-volume errors alongside removed volumes in the report", async () => {
		installHandlers(() => ({
			status: 200,
			body: [
				{ name: "unused-a", size: 1024 },
				{ name: "unused-b", size: 0, error: "volume is busy" },
			],
		}));
		const user = await openPruneModal();

		await user.click(screen.getByTestId("confirm-prune-button"));

		expect(await screen.findByTestId("prune-report")).toBeInTheDocument();
		const report = screen.getByTestId("prune-report");
		expect(within(report).getByText("unused-a")).toBeInTheDocument();
		expect(within(report).getByText("unused-b")).toBeInTheDocument();
		expect(within(report).getByText("volume is busy")).toBeInTheDocument();
	});

	it("reports nothing to prune when the result is empty", async () => {
		installHandlers();
		const user = await openPruneModal();

		await user.click(screen.getByTestId("confirm-prune-button"));

		expect(await screen.findByText(/nothing to prune/i)).toBeInTheDocument();
	});
});
