import type { ContainerSummary } from "@features/containers/services/containersApi";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import { errorJson } from "@/test/errors";
import { containerFixture } from "@/test/factories";
import { server } from "@/test/server";
import {
	containersHandler,
	createHandler,
	createRequests,
	existing,
	listRequests,
	openCreatePage,
	optionListHandlers,
	renderContainersTab,
	resetCounters,
	setupCreatePage,
	startHandler,
	startRequests,
	submitForm,
	typeImage,
} from "./createContainerTestHarness";

// Page-behavior pins for the create-container page (spec 341, ticket
// #344): navigation to and from the page, form shape and validation,
// submit gating, and the error envelope. The form-to-payload mapping
// contract is pinned without a DOM in
// services/containersApi.create.test.ts (issue 328, candidate C3); the
// full-payload smoke lives in CreateContainerPage.integration.test.tsx.

describe("CreateContainerPage", () => {
	it("navigates to the create page when the Create container button is clicked", async () => {
		setupCreatePage();
		renderContainersTab();
		await screen.findByText("existing");
		const user = userEvent.setup();

		await user.click(screen.getByTestId("create-container-button"));

		expect(await screen.findByLabelText(/image/i)).toBeInTheDocument();
		expect(
			screen.getByRole("heading", { name: "Create container" }),
		).toBeInTheDocument();
		// The page replaced the dialog: the tab's list unmounts behind it.
		expect(screen.queryByText("existing")).not.toBeInTheDocument();
	});

	it("renders the form with required image and optional name and command", async () => {
		setupCreatePage();
		await openCreatePage();

		expect(screen.getByLabelText(/image/i)).toBeRequired();
		expect(screen.getByLabelText(/name/i)).not.toBeRequired();
		expect(screen.getByLabelText(/command/i)).not.toBeRequired();
		// The empty form is invalid, so the submit starts disabled and
		// enables only once the image is filled.
		expect(screen.getByTestId("container-form-submit")).toBeDisabled();
	});

	it("starts with the submit disabled on an empty form and sends no request", async () => {
		setupCreatePage();
		const user = await openCreatePage();

		await user.click(screen.getByTestId("container-form-submit"));

		expect(createRequests.count).toBe(0);
		expect(screen.getByLabelText(/image/i)).toBeInTheDocument();
	});

	it("shows the image error while typing a whitespace-only value and sends no request", async () => {
		setupCreatePage();
		const user = await openCreatePage();

		await user.type(screen.getByLabelText(/image/i), "   ");
		expect(await screen.findByText("Image is required")).toBeInTheDocument();
		expect(screen.getByTestId("container-form-submit")).toBeDisabled();
		await user.click(screen.getByTestId("container-form-submit"));

		expect(createRequests.count).toBe(0);
	});

	it("returns to the containers tab on cancel without sending a create request", async () => {
		setupCreatePage();
		const user = await openCreatePage();
		await user.type(
			screen.getByLabelText(/image/i),
			"docker.io/library/redis:7",
		);

		await user.click(screen.getByTestId("container-form-cancel"));

		expect(await screen.findByTestId("containers-tab")).toHaveTextContent(
			"containers",
		);
		expect(createRequests.count).toBe(0);
	});

	it("offers the create action on an empty machine and creates the first container", async () => {
		resetCounters();
		let containers: ContainerSummary[] = [];
		server.use(
			containersHandler(() => containers),
			startHandler(),
			...optionListHandlers(),
			http.post("*/api/machines/m-1/containers", async ({ request }) => {
				createRequests.count++;
				await request.json();
				containers = [
					containerFixture({
						id: "c-new",
						names: ["/fresh"],
						image: "docker.io/library/redis:7",
						state: "created",
					}),
				];
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
			}),
		);
		renderContainersTab();

		expect(await screen.findByTestId("containers-empty")).toBeInTheDocument();
		const user = userEvent.setup();
		await user.click(screen.getByTestId("create-container-button"));
		await user.type(
			await screen.findByLabelText(/image/i),
			"docker.io/library/redis:7",
		);
		await user.click(screen.getByTestId("container-form-submit"));

		expect(await screen.findByText("fresh")).toBeInTheDocument();
		expect(createRequests.count).toBe(1);
	});

	it("navigates back to the containers tab with the refreshed list after success", async () => {
		resetCounters();
		let containers = [existing];
		server.use(
			containersHandler(() => containers),
			startHandler(),
			...optionListHandlers(),
			http.post("*/api/machines/m-1/containers", async ({ request }) => {
				createRequests.count++;
				await request.json();
				containers = [
					existing,
					containerFixture({
						id: "c-new",
						names: ["/fresh"],
						image: "docker.io/library/redis:7",
						state: "created",
					}),
				];
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
			}),
		);
		const user = await openCreatePage();
		expect(listRequests.count).toBe(1);

		await user.type(
			screen.getByLabelText(/image/i),
			"docker.io/library/redis:7",
		);
		await user.click(screen.getByTestId("container-form-submit"));

		expect(await screen.findByTestId("containers-tab")).toHaveTextContent(
			"containers",
		);
		expect(
			await screen.findByText("docker.io/library/redis:7"),
		).toBeInTheDocument();
		// The tab remounted and refetched the list, so the new container
		// shows without a manual refresh.
		await waitFor(() => expect(listRequests.count).toBe(2));
	});

	it("stays on the page and shows the error envelope on failure", async () => {
		setupCreatePage(
			createHandler(
				404,
				errorJson("IMAGE_NOT_FOUND", "image not found", "req-14"),
			),
		);
		const user = await openCreatePage();

		await user.type(
			screen.getByLabelText(/image/i),
			"docker.io/library/nope:latest",
		);
		await user.click(screen.getByTestId("container-form-submit"));

		expect(await screen.findByRole("alert")).toBeInTheDocument();
		expect(screen.getByText("IMAGE_NOT_FOUND")).toBeInTheDocument();
		expect(screen.getByText("image not found")).toBeInTheDocument();
		expect(screen.getByLabelText(/image/i)).toBeInTheDocument();
		expect(
			screen.getByDisplayValue("docker.io/library/nope:latest"),
		).toBeInTheDocument();
	});

	it("renders the environment rows through the shared key/value editor", async () => {
		setupCreatePage();
		const user = await openCreatePage();

		const addButton = screen.getByTestId("add-env-variable");
		expect(addButton).toHaveTextContent("Add variable");
		await user.click(addButton);

		expect(screen.getByLabelText("Environment variable 1 key")).toHaveAttribute(
			"placeholder",
			"KEY",
		);
		expect(
			screen.getByLabelText("Environment variable 1 value"),
		).toBeInTheDocument();
		expect(screen.getByTestId("remove-env-variable-0")).toHaveAttribute(
			"aria-label",
			"Remove environment variable 1",
		);
	});

	it("renders the Security section with a privileged checkbox and cap-add rows", async () => {
		setupCreatePage();
		const user = await openCreatePage();

		const privileged = screen.getByLabelText(/privileged/i);
		expect(privileged).not.toBeChecked();
		await user.click(screen.getByTestId("add-capability"));

		expect(screen.getByLabelText("Capability 1")).toBeInTheDocument();
		expect(screen.getByTestId("remove-capability-0")).toHaveAttribute(
			"aria-label",
			"Remove capability 1",
		);
	});

	it("submits privileged and the trimmed cap-add entries, dropping empty rows", async () => {
		setupCreatePage();
		const user = await openCreatePage();
		await typeImage(user, "redis:7");
		await user.click(screen.getByLabelText(/privileged/i));
		await user.click(screen.getByTestId("add-capability"));
		await user.type(screen.getByLabelText("Capability 1"), "  NET_ADMIN  ");
		await user.click(screen.getByTestId("add-capability"));
		await user.type(screen.getByLabelText("Capability 2"), "   ");

		await submitForm(user);

		expect(createRequests.body).toEqual({
			image: "redis:7",
			privileged: true,
			capAdd: ["NET_ADMIN"],
		});
	});

	// The flags render per bind-mount row through the shared row-editor
	// module; the z/Z mutual-exclusion ritual is pinned once in the
	// module suite (shared/components/RowEditor.test.tsx) and the
	// flags' payload shape in the integration suite below.
	it("renders the z, Z, and U checkboxes per bind-mount row", async () => {
		setupCreatePage();
		const user = await openCreatePage();
		await user.click(screen.getByTestId("add-bind-mount"));

		expect(
			screen.getByLabelText("Bind mount 1 SELinux shared (z)"),
		).not.toBeChecked();
		expect(
			screen.getByLabelText("Bind mount 1 SELinux private (Z)"),
		).not.toBeChecked();
		expect(
			screen.getByLabelText("Bind mount 1 chown to user (U)"),
		).not.toBeChecked();
	});

	// The Resources section (issue #377), mirroring the create-pod
	// dialog's resource limits: the fields render, an unparseable value
	// blocks the submit with the field error, and the payload carries
	// cpus as a number and memory as the trimmed string.
	//
	// The fields render through the shared resource-limit module
	// (spec 437, ticket #431) with the container id prefix; the
	// module's own suite pins the labels, checks, and stacking once.
	it("renders the resource-limit fields through the shared module with the container id prefix", async () => {
		setupCreatePage();
		await openCreatePage();

		expect(screen.getByLabelText("CPUs")).toHaveAttribute(
			"id",
			"container-cpus",
		);
		expect(screen.getByLabelText("Memory")).toHaveAttribute(
			"id",
			"container-memory",
		);
	});

	it("blocks the submit while a resource limit is unparseable and explains why", async () => {
		setupCreatePage();
		const user = await openCreatePage();
		await typeImage(user);
		expect(screen.getByTestId("container-form-submit")).toBeEnabled();

		const memory = screen.getByLabelText("Memory");
		await user.type(memory, "abc");
		expect(
			screen.getByText("Use podman units syntax, e.g. 512m or 1g"),
		).toBeInTheDocument();
		expect(screen.getByTestId("container-form-submit")).toBeDisabled();

		// The CPUs error blocks the submit on its own, not only alongside
		// the memory error.
		await user.clear(memory);
		expect(screen.getByTestId("container-form-submit")).toBeEnabled();
		const cpus = screen.getByLabelText("CPUs");
		await user.type(cpus, "0");
		expect(
			screen.getByText("CPU limit must be a positive number"),
		).toBeInTheDocument();
		expect(screen.getByTestId("container-form-submit")).toBeDisabled();

		await user.clear(cpus);
		await user.type(memory, "512m");
		expect(screen.getByTestId("container-form-submit")).toBeEnabled();
	});

	it("submits cpus as a number and memory as the trimmed string", async () => {
		setupCreatePage();
		const user = await openCreatePage();
		await typeImage(user);
		await user.type(screen.getByLabelText("CPUs"), "1.5");
		await user.type(screen.getByLabelText("Memory"), " 512m ");

		await submitForm(user);

		expect(createRequests.body).toEqual({
			image: "docker.io/library/redis:7",
			cpus: 1.5,
			memory: "512m",
		});
	});
});

// Run-after-create pins (spec 341, ticket #346): the checkbox defaults to
// checked, a checked box chains the start endpoint onto a successful
// create before returning to the tab, an unchecked box creates only, and
// a start failure after a successful create still returns to the tab
// with a toast.
describe("CreateContainerPage run after create", () => {
	it("renders the run-after-create checkbox checked by default", async () => {
		setupCreatePage();
		await openCreatePage();

		expect(screen.getByLabelText("Run after create")).toBeChecked();
	});

	it("starts the created container and returns to the tab when the checkbox is checked", async () => {
		setupCreatePage();
		const user = await openCreatePage();
		await user.type(
			screen.getByLabelText(/image/i),
			"docker.io/library/redis:7",
		);

		await user.click(screen.getByTestId("container-form-submit"));

		// The start request targets the created container's ID (the
		// harness's start handler only matches /containers/c-new/start).
		await waitFor(() => expect(startRequests.count).toBe(1));
		expect(await screen.findByTestId("containers-tab")).toHaveTextContent(
			"containers",
		);
	});

	it("creates only, without a start request, when the checkbox is unchecked", async () => {
		setupCreatePage();
		const user = await openCreatePage();
		await user.click(screen.getByTestId("container-form-run-after-create"));
		await user.type(
			screen.getByLabelText(/image/i),
			"docker.io/library/redis:7",
		);

		await user.click(screen.getByTestId("container-form-submit"));

		expect(await screen.findByTestId("containers-tab")).toHaveTextContent(
			"containers",
		);
		expect(createRequests.count).toBe(1);
		expect(startRequests.count).toBe(0);
	});

	it("still returns to the tab with a toast when the start fails after a successful create", async () => {
		setupCreatePage();
		// A failing start overrides the harness's default start handler.
		server.use(
			startHandler(
				409,
				errorJson(
					"CONTAINER_ALREADY_RUNNING",
					"container is already running",
					"req-21",
				),
			),
		);
		const user = await openCreatePage();
		await user.type(
			screen.getByLabelText(/image/i),
			"docker.io/library/redis:7",
		);

		await user.click(screen.getByTestId("container-form-submit"));

		await waitFor(() => expect(startRequests.count).toBe(1));
		expect(
			await screen.findByText(/container was created but failed to start/i),
		).toBeInTheDocument();
		expect(
			screen.getByText("container is already running"),
		).toBeInTheDocument();
		// The container exists, so the user still lands on the tab.
		expect(await screen.findByTestId("containers-tab")).toHaveTextContent(
			"containers",
		);
	});
});
