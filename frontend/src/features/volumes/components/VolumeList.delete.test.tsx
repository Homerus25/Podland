import { ChakraProvider, theme } from "@chakra-ui/react";
import { VolumeList } from "@features/volumes/components/VolumeList";
import type { VolumeSummary } from "@features/volumes/services/volumesApi";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http, type JsonBodyType } from "msw";
import { beforeEach, describe, expect, it } from "vitest";
import { volumeFixture } from "@/test/factories";
import { server } from "@/test/server";

const remaining = volumeFixture({ name: "survivor" });

let listRequests: { count: number; volumes: () => VolumeSummary[] } = {
	count: 0,
	volumes: () => [volumeFixture({ name: "victim" }), remaining],
};
let deleteRequests: {
	count: number;
	pathname: string;
	search: string;
} = { count: 0, pathname: "", search: "" };

function resetCounters() {
	listRequests = {
		count: 0,
		volumes: () => [volumeFixture({ name: "victim" }), remaining],
	};
	deleteRequests = { count: 0, pathname: "", search: "" };
}

function installHandlers(
	deleteResponse: () => {
		status: number;
		body?: JsonBodyType;
	} = () => ({ status: 204 }),
	removeOnDelete = false,
) {
	server.use(
		http.get("*/api/machines/m-1/volumes", () => {
			listRequests.count += 1;
			return HttpResponse.json(listRequests.volumes());
		}),
		http.delete("*/api/machines/m-1/volumes/*", ({ request }) => {
			deleteRequests.count += 1;
			const url = new URL(request.url);
			deleteRequests.pathname = url.pathname;
			deleteRequests.search = url.search;
			const response = deleteResponse();
			if (response.status !== 204) {
				return HttpResponse.json(response.body, { status: response.status });
			}
			if (removeOnDelete) {
				listRequests.volumes = () => [remaining];
			}
			return new HttpResponse(null, { status: 204 });
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

async function openDeleteModal(
	permission: "read" | "write" | "admin" = "admin",
) {
	renderVolumeList(permission);
	await screen.findByText("victim");
	const user = userEvent.setup();
	await user.click(screen.getByRole("button", { name: /delete victim/i }));
	expect(await screen.findByTestId("modal")).toBeInTheDocument();
	return user;
}

describe("VolumeList delete", () => {
	beforeEach(resetCounters);

	it("hides the delete action from read-level users", async () => {
		installHandlers();
		renderVolumeList("read");

		await screen.findByText("victim");

		expect(
			screen.queryByRole("button", { name: /delete victim/i }),
		).not.toBeInTheDocument();
		expect(deleteRequests.count).toBe(0);
	});

	it("opens a confirmation dialog naming the volume with an unchecked force checkbox", async () => {
		installHandlers();
		await openDeleteModal();

		expect(screen.getAllByText(/victim/).length).toBeGreaterThan(0);
		const checkbox = screen.getByTestId("force-delete-checkbox");
		expect(checkbox).not.toBeChecked();
		expect(deleteRequests.count).toBe(0);
	});

	it("sends force=false without the checkbox and force=true with it", async () => {
		installHandlers();
		const user = await openDeleteModal();

		await user.click(screen.getByTestId("confirm-delete-button"));
		await waitFor(() => expect(deleteRequests.count).toBe(1));
		expect(deleteRequests.pathname).toBe("/api/machines/m-1/volumes/victim");
		expect(deleteRequests.search).toBe("");

		await waitFor(() =>
			expect(screen.queryByTestId("modal")).not.toBeInTheDocument(),
		);
		await user.click(screen.getByRole("button", { name: /delete victim/i }));
		expect(await screen.findByTestId("modal")).toBeInTheDocument();
		await user.click(screen.getByTestId("force-delete-checkbox"));
		await user.click(screen.getByTestId("confirm-delete-button"));
		await waitFor(() => expect(deleteRequests.count).toBe(2));
		expect(deleteRequests.search).toBe("?force=true");
	});

	it("removes the row, invalidates the list, and toasts on success", async () => {
		installHandlers(undefined, true);
		const user = await openDeleteModal();

		await user.click(screen.getByTestId("confirm-delete-button"));

		expect(await screen.findByText("survivor")).toBeInTheDocument();
		await waitFor(() =>
			expect(screen.queryByText("victim")).not.toBeInTheDocument(),
		);
		expect(screen.queryByTestId("modal")).not.toBeInTheDocument();
		expect(await screen.findByText(/deleted/i)).toBeInTheDocument();
	});

	it("renders the podman message plus a force-required hint on 409 in-use", async () => {
		installHandlers(() => ({
			status: 409,
			body: {
				requestId: "req-42",
				error: {
					message: "volume is in use by container abc",
					code: "VOLUME_IN_USE",
				},
			},
		}));
		const user = await openDeleteModal();

		await user.click(screen.getByTestId("confirm-delete-button"));

		expect(await screen.findByRole("alert")).toBeInTheDocument();
		expect(
			screen.getByText("volume is in use by container abc"),
		).toBeInTheDocument();
		expect(screen.getByText(/force is required/i)).toBeInTheDocument();
		expect(screen.getByText(/failed to delete volume/i)).toBeInTheDocument();
		expect(screen.getByText("victim")).toBeInTheDocument();
	});
});
