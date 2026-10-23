import { ContainerList } from "@features/containers/components/ContainerList";
import { CreateContainerPage } from "@features/containers/components/CreateContainerPage";
import type { ContainerSummary } from "@features/containers/services/containersApi";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {
	type DefaultBodyType,
	HttpResponse,
	http,
	type JsonBodyType,
} from "msw";
import { Route, Routes, useSearchParams } from "react-router-dom";
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

// Shared MSW + render harness for the create-container page suites
// (CreateContainerPage.test.tsx and
// CreateContainerPage.integration.test.tsx): the request counters, the
// list/option/create handlers, and the open/type/submit helpers both
// files drive. The option-list handlers count their requests so the
// integration suite can prove the dropdown data travels through MSW, and
// the list handler counts its requests so the suites can prove the
// navigate-back refetches the containers tab's list.

export const existing = containerFixture({ names: ["/existing"] });

export const createRequests: { count: number; body: unknown } = {
	count: 0,
	body: null,
};
export const optionRequests = { networks: 0, volumes: 0, secrets: 0 };
export const listRequests = { count: 0 };
// The image pull stream the create page's image-not-found flow rides
// (spec 341 ticket #349); counted like the create request so the pull
// flow's request shape is assertable.
export const pullRequests: { count: number; body: unknown } = {
	count: 0,
	body: null,
};
export const startRequests = { count: 0 };

export function resetCounters() {
	createRequests.count = 0;
	createRequests.body = null;
	optionRequests.networks = 0;
	optionRequests.volumes = 0;
	optionRequests.secrets = 0;
	listRequests.count = 0;
	pullRequests.count = 0;
	pullRequests.body = null;
	startRequests.count = 0;
}

export function containersHandler(containers: () => ContainerSummary[]) {
	return http.get("*/api/machines/m-1/containers", () => {
		listRequests.count++;
		return HttpResponse.json({
			containers: containers(),
			total: containers().length,
			timestamp: "2026-08-26T00:00:00Z",
		});
	});
}

// The create page fetches the machine's networks, volumes, and secrets to
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

// The image pull stream endpoint (spec 341 ticket #349): the create page's
// image-not-found flow pulls the missing image through the real stream
// transport. The handler counts the request and captures its body; the
// caller hands the built NDJSON response (ndjsonResponse /
// abortableNdjsonResponse from @/test/stream).
export function pullStreamHandler(response: HttpResponse<DefaultBodyType>) {
	return http.post("*/api/machines/m-1/images/pull", async ({ request }) => {
		pullRequests.count++;
		pullRequests.body = await request.json();
		return response;
	});
}

// The start endpoint the run-after-create flow chains onto after a
// successful create (spec 341, ticket #346). The URL pins the created
// container's ID (the create handler answers with "c-new"), so a request
// here proves the chain carries the create response's ID.
export function startHandler(status = 200, body?: JsonBodyType) {
	return http.post("*/api/machines/m-1/containers/c-new/start", () => {
		startRequests.count++;
		if (status !== 200) return HttpResponse.json(body, { status });
		return new HttpResponse(null, { status: 200 });
	});
}

// The standard preamble of a create-page test: reset the counters and
// arm the list, option, create, and start handlers. A test that needs a
// failing create (or a failing start) passes its own handler.
export function setupCreatePage(create = createHandler()) {
	resetCounters();
	server.use(
		containersHandler(() => [existing]),
		create,
		startHandler(),
		...optionListHandlers(),
	);
}

// The machine detail page's containers-tab stand-in at the create page's
// return target: the real ContainerList (so a navigate-back exercises the
// tab's fresh list fetch) behind a marker exposing the ?tab= param the
// page's cancel/success navigation carries.
function ContainersTab() {
	const [searchParams] = useSearchParams();
	return (
		<div>
			<span data-testid="containers-tab">{searchParams.get("tab")}</span>
			<ContainerList machine={machineFixture()} />
		</div>
	);
}

function renderCreateRoutes(initialEntry: string) {
	renderWithQuery(
		<Routes>
			<Route path="/machines/:machineId" element={<ContainersTab />} />
			<Route
				path="/machines/:machineId/containers/new"
				element={<CreateContainerPage />}
			/>
		</Routes>,
		{
			// chakra for the run-after-create toast portal (spec 341,
			// ticket #346); the router seeds the entry route.
			chakra: true,
			router: { initialEntries: [initialEntry] },
		},
	);
}

// Renders the containers tab first, the way the user arrives at the page.
export function renderContainersTab() {
	renderCreateRoutes("/machines/m-1");
}

// Renders the create page directly, for tests that only exercise the form.
export function renderCreatePage() {
	renderCreateRoutes("/machines/m-1/containers/new");
}

export type User = ReturnType<typeof userEvent.setup>;

// The standard entry of a create-page test: render the containers tab,
// then click the toolbar's Create container button to navigate to the
// page. Returns the user once the form is up.
export async function openCreatePage() {
	renderContainersTab();
	await screen.findByText("existing");
	const user = userEvent.setup();
	await user.click(screen.getByTestId("create-container-button"));
	expect(await screen.findByLabelText(/image/i)).toBeInTheDocument();
	return user;
}

// Navigates from the containers tab back to the create page (the form
// remounts, so it starts from the empty defaults).
export async function reopenCreatePage(user: User) {
	await user.click(screen.getByTestId("create-container-button"));
	expect(await screen.findByLabelText(/image/i)).toBeInTheDocument();
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
