import { ContainerList } from "@features/containers/components/ContainerList";
import type { ContainerSummary } from "@features/containers/services/containersApi";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http, type JsonBodyType } from "msw";
import { expect } from "vitest";
import {
	containerFixture,
	machineFixture,
	networkFixture,
	secretFixture,
	volumeFixture,
} from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

// Shared MSW + render harness for the create-container dialog suites
// (ContainerList.create.test.tsx and
// ContainerList.create.integration.test.tsx): the request counters, the
// list/option/create handlers, and the open/type/submit helpers both
// files drive. The option-list handlers count their requests so the
// integration suite can prove the dropdown data travels through MSW.

export const existing = containerFixture({ names: ["/existing"] });

export const createRequests: { count: number; body: unknown } = {
	count: 0,
	body: null,
};
export const optionRequests = { networks: 0, volumes: 0, secrets: 0 };

export function resetCounters() {
	createRequests.count = 0;
	createRequests.body = null;
	optionRequests.networks = 0;
	optionRequests.volumes = 0;
	optionRequests.secrets = 0;
}

export function containersHandler(containers: () => ContainerSummary[]) {
	return http.get("*/api/machines/m-1/containers", () =>
		HttpResponse.json({
			containers: containers(),
			total: containers().length,
			timestamp: "2026-08-26T00:00:00Z",
		}),
	);
}

// The create dialog fetches the machine's networks, volumes, and secrets to
// populate its dropdowns and checkbox lists (spec 313 ticket #317); the
// counters prove the requests travel through the MSW handlers.
export function optionListHandlers() {
	return [
		http.get("*/api/machines/m-1/networks", () => {
			optionRequests.networks++;
			return HttpResponse.json([
				networkFixture(),
				networkFixture({ name: "other-net", subnets: ["10.89.0.0/16"] }),
			]);
		}),
		http.get("*/api/machines/m-1/volumes", () => {
			optionRequests.volumes++;
			return HttpResponse.json([
				volumeFixture(),
				volumeFixture({ name: "pgdata" }),
			]);
		}),
		http.get("*/api/machines/m-1/secrets", () => {
			optionRequests.secrets++;
			return HttpResponse.json([
				secretFixture(),
				secretFixture({ id: "s-2", name: "api-token" }),
			]);
		}),
	];
}

export function createHandler(status = 201, body?: JsonBodyType) {
	return http.post("*/api/machines/m-1/containers", async ({ request }) => {
		createRequests.count++;
		createRequests.body = await request.json();
		if (status !== 201) return HttpResponse.json(body, { status });
		return HttpResponse.json(
			{
				id: "c-new",
				name: "fresh",
				state: "created",
				image: "docker.io/library/redis:7",
				createdAt: "2026-08-26T00:00:00Z",
			},
			{ status: 201 },
		);
	});
}

// The standard preamble of a create-dialog test: reset the counters and
// arm the list, option, and create handlers. A test that needs a failing
// create passes its own create handler.
export function setupCreateDialog(create = createHandler()) {
	resetCounters();
	server.use(
		containersHandler(() => [existing]),
		create,
		...optionListHandlers(),
	);
}

export function renderContainerList() {
	renderWithQuery(<ContainerList machine={machineFixture()} />, {
		router: true,
	});
}

export async function openCreateModal() {
	renderContainerList();
	await screen.findByText("existing");
	const user = userEvent.setup();
	await user.click(screen.getByTestId("create-container-button"));
	expect(await screen.findByTestId("modal")).toBeInTheDocument();
	return user;
}

export type User = ReturnType<typeof userEvent.setup>;

export async function reopenCreateModal(user: User) {
	await user.click(screen.getByTestId("create-container-button"));
	expect(await screen.findByTestId("modal")).toBeInTheDocument();
}

export async function typeImage(
	user: User,
	image = "docker.io/library/redis:7",
) {
	await user.type(screen.getByLabelText(/image/i), image);
}

export async function submitForm(user: User) {
	await user.click(screen.getByTestId("container-form-submit"));
	await waitFor(() => expect(createRequests.count).toBe(1));
}

// Populates every one of the 14 fields with the standard values (the
// full-payload test and the 404-preservation test share this flow). The
// image is parameterized so the 404 test can point at a missing one.
export async function populateAllFields(
	user: User,
	image = "docker.io/library/redis:7",
) {
	await typeImage(user, image);
	// Basic
	await user.type(screen.getByLabelText(/name/i), "cache");
	await user.type(
		screen.getByLabelText(/command/i),
		"redis-server --appendonly yes",
	);
	await user.type(screen.getByLabelText(/user/i), "1000:1000");
	await user.type(screen.getByLabelText(/timezone/i), "Europe/Berlin");
	await user.selectOptions(screen.getByLabelText(/restart policy/i), "always");
	await user.click(screen.getByLabelText(/remove on exit/i));
	// Environment
	await user.click(screen.getByTestId("add-env-variable"));
	await user.type(screen.getByLabelText("Environment variable 1 key"), "FOO");
	await user.type(screen.getByLabelText("Environment variable 1 value"), "bar");
	await user.click(screen.getByTestId("add-env-variable"));
	await user.type(screen.getByLabelText("Environment variable 2 key"), "MODE");
	await user.type(
		screen.getByLabelText("Environment variable 2 value"),
		"prod",
	);
	// Ports
	await user.click(screen.getByTestId("add-port-binding"));
	await user.type(screen.getByLabelText("Host port 1"), "8080");
	await user.type(screen.getByLabelText("Container port 1"), "80");
	await user.selectOptions(screen.getByLabelText("Protocol 1"), "udp");
	// Storage: bind mount + named volume
	await user.click(screen.getByTestId("add-bind-mount"));
	await user.type(screen.getByLabelText("Mount source 1"), "/srv/app");
	await user.type(screen.getByLabelText("Mount destination 1"), "/app");
	await user.selectOptions(screen.getByLabelText("Mount mode 1"), "ro");
	await user.click(screen.getByTestId("add-named-volume"));
	const volumeSelect = screen.getByLabelText("Volume name 1");
	await within(volumeSelect).findByText("pgdata");
	await user.selectOptions(volumeSelect, "pgdata");
	await user.type(screen.getByLabelText("Volume destination 1"), "/data");
	// Network + Secrets
	await user.click(await screen.findByLabelText("app-net"));
	await user.click(await screen.findByLabelText("db-password"));
	// Devices
	await user.click(screen.getByTestId("add-device"));
	await user.type(
		screen.getByLabelText("Device host path 1"),
		"/dev/dri/renderD128",
	);
	await user.type(
		screen.getByLabelText("Device container path 1"),
		"/dev/dri/renderD128",
	);
}
