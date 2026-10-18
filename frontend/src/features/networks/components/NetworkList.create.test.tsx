import type { ContainerSummary } from "@features/containers/services/containersApi";
import { NetworkList } from "@features/networks/components/NetworkList";
import { QueryClient } from "@tanstack/react-query";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http, type JsonBodyType } from "msw";
import { describe, expect, it } from "vitest";
import { errorJson, errorResponse } from "@/test/errors";
import {
	containerFixture,
	machineFixture,
	networkFixture,
} from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

const runningWeb = containerFixture({ id: "c-1", names: ["/web"] });
const runningDb = containerFixture({ id: "c-2", names: ["/db"] });
const stoppedCache = containerFixture({
	id: "c-3",
	names: ["/cache"],
	state: "exited",
	status: "Exited (0) 5 minutes ago",
});

let networksGets = 0;
let containersGets = 0;
let createRequests: { count: number; body: unknown } = { count: 0, body: null };

function resetCounters() {
	networksGets = 0;
	containersGets = 0;
	createRequests = { count: 0, body: null };
}

function mockLists(
	containers: ContainerSummary[] = [runningWeb, runningDb, stoppedCache],
) {
	return [
		http.get("*/api/machines/m-1/networks", () => {
			networksGets++;
			return HttpResponse.json([networkFixture()]);
		}),
		http.get("*/api/machines/m-1/containers", () => {
			containersGets++;
			return HttpResponse.json({
				containers,
				total: containers.length,
				timestamp: "2026-08-26T00:00:00Z",
			});
		}),
	];
}

function createHandler(status = 201, body?: JsonBodyType) {
	return http.post("*/api/machines/m-1/networks", async ({ request }) => {
		createRequests.count++;
		createRequests.body = await request.json();
		if (status !== 201) return HttpResponse.json(body, { status });
		return HttpResponse.json(networkFixture({ name: "new-net" }), {
			status: 201,
		});
	});
}

async function openCreateDialog() {
	renderWithQuery(<NetworkList machine={machineFixture()} />);
	await screen.findByTestId("network-row");
	const user = userEvent.setup();
	await user.click(screen.getByTestId("create-network-button"));
	expect(await screen.findByTestId("modal")).toBeInTheDocument();
	return user;
}

describe("NetworkList create", () => {
	it("opens the dialog with the submit disabled until the name is filled", async () => {
		resetCounters();
		server.use(...mockLists(), createHandler());
		const user = await openCreateDialog();

		expect(screen.getByLabelText(/name/i)).toBeRequired();
		expect(screen.getByTestId("create-network-submit")).toBeDisabled();
		await user.type(screen.getByLabelText(/name/i), "new-net");
		await waitFor(() =>
			expect(screen.getByTestId("create-network-submit")).toBeEnabled(),
		);
	});

	it("offers only the machine's running containers", async () => {
		resetCounters();
		server.use(...mockLists(), createHandler());
		const user = await openCreateDialog();

		await user.click(screen.getByTestId("searchable-select-input"));
		expect(
			await screen.findByTestId("searchable-select-option-c-1"),
		).toBeInTheDocument();
		expect(
			screen.getByTestId("searchable-select-option-c-2"),
		).toBeInTheDocument();
		expect(
			screen.queryByTestId("searchable-select-option-c-3"),
		).not.toBeInTheDocument();
	});

	it("accumulates multiple container selections as chips", async () => {
		resetCounters();
		server.use(...mockLists(), createHandler());
		const user = await openCreateDialog();

		await user.click(screen.getByTestId("searchable-select-input"));
		await user.click(screen.getByTestId("searchable-select-option-c-1"));
		await user.click(screen.getByTestId("searchable-select-option-c-2"));
		expect(
			screen.getByTestId("searchable-select-chip-c-1"),
		).toBeInTheDocument();
		expect(
			screen.getByTestId("searchable-select-chip-c-2"),
		).toBeInTheDocument();
	});

	it("sends {name, containers} and refetches both lists on success", async () => {
		resetCounters();
		server.use(...mockLists(), createHandler());
		const user = await openCreateDialog();

		await user.type(screen.getByLabelText(/name/i), "new-net");
		await user.click(screen.getByTestId("searchable-select-input"));
		await user.click(screen.getByTestId("searchable-select-option-c-1"));
		await user.click(screen.getByTestId("searchable-select-option-c-2"));
		await user.click(screen.getByTestId("create-network-submit"));

		await waitFor(() => expect(createRequests.count).toBe(1));
		expect(createRequests.body).toEqual({
			name: "new-net",
			containers: ["c-1", "c-2"],
		});
		// Both lists refetch: the containers DTO carries each container's
		// networks, so the attach is visible from both tabs.
		await waitFor(() => expect(networksGets).toBe(2));
		await waitFor(() => expect(containersGets).toBe(2));
		await waitFor(() =>
			expect(screen.queryByTestId("modal")).not.toBeInTheDocument(),
		);
	});

	it("sends only the name when no container is selected", async () => {
		resetCounters();
		server.use(...mockLists(), createHandler());
		const user = await openCreateDialog();

		await user.type(screen.getByLabelText(/name/i), "new-net");
		await user.click(screen.getByTestId("create-network-submit"));

		await waitFor(() => expect(createRequests.count).toBe(1));
		expect(createRequests.body).toEqual({ name: "new-net" });
	});

	it("issues a fresh containers request every time the dialog opens, inside the five-minute cache window", async () => {
		resetCounters();
		server.use(...mockLists(), createHandler());
		// The app-wide stale time is five minutes (main.tsx); mirroring it
		// pins the refetch inside the cache window.
		const queryClient = new QueryClient({
			defaultOptions: { queries: { retry: false, staleTime: 5 * 60 * 1000 } },
		});
		renderWithQuery(<NetworkList machine={machineFixture()} />, {
			queryClient,
		});
		await screen.findByTestId("network-row");
		const user = userEvent.setup();

		await user.click(screen.getByTestId("create-network-button"));
		expect(await screen.findByTestId("modal")).toBeInTheDocument();
		await waitFor(() => expect(containersGets).toBe(1));

		await user.click(screen.getByRole("button", { name: /cancel/i }));
		await waitFor(() =>
			expect(screen.queryByTestId("modal")).not.toBeInTheDocument(),
		);

		await user.click(screen.getByTestId("create-network-button"));
		expect(await screen.findByTestId("modal")).toBeInTheDocument();
		await waitFor(() => expect(containersGets).toBe(2));
	});

	it("renders the duplicate-name envelope and stays open on 409", async () => {
		resetCounters();
		server.use(
			...mockLists(),
			createHandler(
				409,
				errorJson(
					"NETWORK_ALREADY_EXISTS",
					"network app-net already exists",
					"req-9",
				),
			),
		);
		const user = await openCreateDialog();

		await user.type(screen.getByLabelText(/name/i), "app-net");
		await user.click(screen.getByTestId("create-network-submit"));

		const error = await screen.findByTestId("mutation-dialog-error");
		expect(error).toHaveTextContent("NETWORK_ALREADY_EXISTS");
		expect(error).toHaveTextContent("network app-net already exists");
		expect(screen.getByTestId("modal")).toBeInTheDocument();
	});

	it("renders the attach-failure envelope and stays open", async () => {
		resetCounters();
		server.use(
			...mockLists(),
			createHandler(
				409,
				errorJson(
					"NETWORK_ATTACH_FAILED",
					"failed to attach container c-2 to network new-net",
					"req-10",
				),
			),
		);
		const user = await openCreateDialog();

		await user.type(screen.getByLabelText(/name/i), "new-net");
		await user.click(screen.getByTestId("searchable-select-input"));
		await user.click(screen.getByTestId("searchable-select-option-c-2"));
		await user.click(screen.getByTestId("create-network-submit"));

		const error = await screen.findByTestId("mutation-dialog-error");
		expect(error).toHaveTextContent("NETWORK_ATTACH_FAILED");
		expect(error).toHaveTextContent("failed to attach container c-2");
		expect(screen.getByTestId("modal")).toBeInTheDocument();
	});

	it("keeps the name and container selections across a 409, then closes on retry", async () => {
		resetCounters();
		let attempts = 0;
		server.use(
			...mockLists(),
			http.post("*/api/machines/m-1/networks", async ({ request }) => {
				createRequests.count++;
				createRequests.body = await request.json();
				attempts++;
				if (attempts === 1) {
					return errorResponse(
						409,
						"NETWORK_ALREADY_EXISTS",
						"network app-net already exists",
						"req-11",
					);
				}
				return HttpResponse.json(networkFixture({ name: "app-net-2" }), {
					status: 201,
				});
			}),
		);
		const user = await openCreateDialog();

		await user.type(screen.getByLabelText(/name/i), "app-net");
		await user.click(screen.getByTestId("searchable-select-input"));
		await user.click(screen.getByTestId("searchable-select-option-c-1"));
		await user.click(screen.getByTestId("searchable-select-option-c-2"));
		await user.click(screen.getByTestId("create-network-submit"));

		// The conflict renders in the dialog's error region and the dialog
		// stays open with the typed name and both container chips intact.
		const error = await screen.findByTestId("mutation-dialog-error");
		expect(error).toHaveTextContent("NETWORK_ALREADY_EXISTS");
		expect(error).toHaveTextContent("network app-net already exists");
		expect(screen.getByTestId("modal")).toBeInTheDocument();
		expect(screen.getByLabelText(/name/i)).toHaveValue("app-net");
		expect(
			screen.getByTestId("searchable-select-chip-c-1"),
		).toBeInTheDocument();
		expect(
			screen.getByTestId("searchable-select-chip-c-2"),
		).toBeInTheDocument();

		// Rename and retry: the second attempt carries the selections along
		// and its success closes the dialog.
		const nameField = screen.getByLabelText(/name/i);
		await user.clear(nameField);
		await user.type(nameField, "app-net-2");
		await waitFor(() =>
			expect(screen.getByTestId("create-network-submit")).toBeEnabled(),
		);
		await user.click(screen.getByTestId("create-network-submit"));

		await waitFor(() => expect(createRequests.count).toBe(2));
		expect(createRequests.body).toEqual({
			name: "app-net-2",
			containers: ["c-1", "c-2"],
		});
		await waitFor(() =>
			expect(screen.queryByTestId("modal")).not.toBeInTheDocument(),
		);
	});
});
