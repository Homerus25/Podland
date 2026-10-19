import type { ContainerSummary } from "@features/containers/services/containersApi";
import { screen, waitFor, within } from "@testing-library/react";
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
	openCreateModal,
	optionListHandlers,
	renderContainerList,
	resetCounters,
	setupCreateDialog,
	submitForm,
	typeImage,
} from "./createContainerTestHarness";

describe("ContainerList create", () => {
	it("opens a modal form with required image and optional name and command", async () => {
		setupCreateDialog();
		await openCreateModal();

		expect(screen.getByLabelText(/image/i)).toBeRequired();
		expect(screen.getByLabelText(/name/i)).not.toBeRequired();
		expect(screen.getByLabelText(/command/i)).not.toBeRequired();
		// The empty form is invalid, so the module footer's submit starts
		// disabled and enables only once the image is filled.
		expect(screen.getByTestId("container-form-submit")).toBeDisabled();
	});

	it("starts with the submit disabled on an empty form and sends no request", async () => {
		setupCreateDialog();
		const user = await openCreateModal();

		await user.click(screen.getByTestId("container-form-submit"));

		expect(createRequests.count).toBe(0);
		expect(screen.getByTestId("modal")).toBeInTheDocument();
	});

	it("shows the image error while typing a whitespace-only value and sends no request", async () => {
		setupCreateDialog();
		const user = await openCreateModal();

		await user.type(screen.getByLabelText(/image/i), "   ");
		expect(await screen.findByText("Image is required")).toBeInTheDocument();
		expect(screen.getByTestId("container-form-submit")).toBeDisabled();
		await user.click(screen.getByTestId("container-form-submit"));

		expect(createRequests.count).toBe(0);
	});

	it("offers the create action on an empty machine and creates the first container", async () => {
		resetCounters();
		let containers: ContainerSummary[] = [];
		server.use(
			containersHandler(() => containers),
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
		renderContainerList();

		expect(await screen.findByTestId("containers-empty")).toBeInTheDocument();
		const user = userEvent.setup();
		await user.click(screen.getByTestId("create-container-button"));
		await user.type(
			await screen.findByLabelText(/image/i),
			"docker.io/library/redis:7",
		);
		await user.click(screen.getByTestId("container-form-submit"));

		await waitFor(() =>
			expect(screen.queryByTestId("modal")).not.toBeInTheDocument(),
		);
		expect(await screen.findByText("fresh")).toBeInTheDocument();
		expect(createRequests.count).toBe(1);
	});

	it("sends the create payload in the backend DTO shape", async () => {
		setupCreateDialog();
		const user = await openCreateModal();

		await user.type(
			screen.getByLabelText(/image/i),
			"docker.io/library/redis:7",
		);
		await user.type(
			screen.getByLabelText(/command/i),
			"redis-server --appendonly yes",
		);
		await user.click(screen.getByTestId("container-form-submit"));

		await waitFor(() => expect(createRequests.count).toBe(1));
		expect(createRequests.body).toEqual({
			image: "docker.io/library/redis:7",
			command: ["redis-server", "--appendonly", "yes"],
		});
	});

	it("includes the optional name when one is given", async () => {
		setupCreateDialog();
		const user = await openCreateModal();

		await user.type(
			screen.getByLabelText(/image/i),
			"docker.io/library/redis:7",
		);
		await user.type(screen.getByLabelText(/name/i), "cache");
		await user.click(screen.getByTestId("container-form-submit"));

		await waitFor(() => expect(createRequests.count).toBe(1));
		expect(createRequests.body).toEqual({
			image: "docker.io/library/redis:7",
			name: "cache",
		});
	});

	it("closes the modal and shows the new container after success", async () => {
		resetCounters();
		let containers = [existing];
		server.use(
			containersHandler(() => containers),
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
		const user = await openCreateModal();

		await user.type(
			screen.getByLabelText(/image/i),
			"docker.io/library/redis:7",
		);
		await user.click(screen.getByTestId("container-form-submit"));

		await waitFor(() =>
			expect(screen.queryByTestId("modal")).not.toBeInTheDocument(),
		);
		expect(
			await screen.findByText("docker.io/library/redis:7"),
		).toBeInTheDocument();
	});

	it("keeps the modal open and shows the error envelope on failure", async () => {
		setupCreateDialog(
			createHandler(
				404,
				errorJson("IMAGE_NOT_FOUND", "image not found", "req-14"),
			),
		);
		const user = await openCreateModal();

		await user.type(
			screen.getByLabelText(/image/i),
			"docker.io/library/nope:latest",
		);
		await user.click(screen.getByTestId("container-form-submit"));

		expect(await screen.findByRole("alert")).toBeInTheDocument();
		expect(screen.getByText("IMAGE_NOT_FOUND")).toBeInTheDocument();
		expect(screen.getByText("image not found")).toBeInTheDocument();
		expect(screen.getByTestId("modal")).toBeInTheDocument();
		expect(
			screen.getByDisplayValue("docker.io/library/nope:latest"),
		).toBeInTheDocument();
	});

	it("sends environment variables as a map, skipping rows with an empty key", async () => {
		setupCreateDialog();
		const user = await openCreateModal();
		await typeImage(user);

		await user.click(screen.getByTestId("add-env-variable"));
		await user.type(screen.getByLabelText("Environment variable 1 key"), "FOO");
		await user.type(
			screen.getByLabelText("Environment variable 1 value"),
			"bar",
		);
		await user.click(screen.getByTestId("add-env-variable"));
		await user.type(
			screen.getByLabelText("Environment variable 2 key"),
			"MODE",
		);
		await user.type(
			screen.getByLabelText("Environment variable 2 value"),
			"prod",
		);
		// A row with an empty key carries no variable and is skipped.
		await user.click(screen.getByTestId("add-env-variable"));
		await user.type(
			screen.getByLabelText("Environment variable 3 value"),
			"ignored",
		);

		await submitForm(user);

		expect(createRequests.body).toEqual({
			image: "docker.io/library/redis:7",
			environment: { FOO: "bar", MODE: "prod" },
		});
	});

	it("sends port bindings with numeric ports and filters empty rows", async () => {
		setupCreateDialog();
		const user = await openCreateModal();
		await typeImage(user);

		await user.click(screen.getByTestId("add-port-binding"));
		await user.type(screen.getByLabelText("Host port 1"), "8080");
		await user.type(screen.getByLabelText("Container port 1"), "80");
		await user.selectOptions(screen.getByLabelText("Protocol 1"), "udp");
		// An empty row is filtered out of the payload.
		await user.click(screen.getByTestId("add-port-binding"));

		await submitForm(user);

		expect(createRequests.body).toEqual({
			image: "docker.io/library/redis:7",
			portBindings: [{ hostPort: 8080, containerPort: 80, protocol: "udp" }],
		});
	});

	it("sends bind mounts with source, destination, and mode", async () => {
		setupCreateDialog();
		const user = await openCreateModal();
		await typeImage(user);

		await user.click(screen.getByTestId("add-bind-mount"));
		await user.type(screen.getByLabelText("Mount source 1"), "/srv/app");
		await user.type(screen.getByLabelText("Mount destination 1"), "/app");
		await user.selectOptions(screen.getByLabelText("Mount mode 1"), "ro");

		await submitForm(user);

		expect(createRequests.body).toEqual({
			image: "docker.io/library/redis:7",
			mounts: [{ source: "/srv/app", destination: "/app", mode: "ro" }],
		});
	});

	it("sends named volumes picked from the machine's volume list", async () => {
		setupCreateDialog();
		const user = await openCreateModal();
		await typeImage(user);

		await user.click(screen.getByTestId("add-named-volume"));
		// The dropdown offers the volumes fetched from the machine.
		const volumeSelect = screen.getByLabelText("Volume name 1");
		await within(volumeSelect).findByText("pgdata");
		await user.selectOptions(volumeSelect, "pgdata");
		await user.type(screen.getByLabelText("Volume destination 1"), "/data");

		await submitForm(user);

		expect(createRequests.body).toEqual({
			image: "docker.io/library/redis:7",
			volumes: [{ name: "pgdata", destination: "/data", mode: "rw" }],
		});
	});

	it("sends the selected networks as a string array", async () => {
		setupCreateDialog();
		const user = await openCreateModal();
		await typeImage(user);

		await user.click(await screen.findByLabelText("app-net"));
		await user.click(screen.getByLabelText("other-net"));

		await submitForm(user);

		expect(createRequests.body).toEqual({
			image: "docker.io/library/redis:7",
			networks: ["app-net", "other-net"],
		});
	});

	it("sends selected secrets with the default target path", async () => {
		setupCreateDialog();
		const user = await openCreateModal();
		await typeImage(user);

		await user.click(await screen.findByLabelText("db-password"));

		await submitForm(user);

		expect(createRequests.body).toEqual({
			image: "docker.io/library/redis:7",
			secrets: [{ source: "db-password", target: "/run/secrets/db-password" }],
		});
	});

	it("sends devices with host and container paths", async () => {
		setupCreateDialog();
		const user = await openCreateModal();
		await typeImage(user);

		await user.click(screen.getByTestId("add-device"));
		await user.type(
			screen.getByLabelText("Device host path 1"),
			"/dev/dri/renderD128",
		);
		await user.type(
			screen.getByLabelText("Device container path 1"),
			"/dev/dri/renderD128",
		);

		await submitForm(user);

		expect(createRequests.body).toEqual({
			image: "docker.io/library/redis:7",
			devices: [
				{
					hostPath: "/dev/dri/renderD128",
					containerPath: "/dev/dri/renderD128",
				},
			],
		});
	});

	it("sends the restart policy and the remove-on-exit flag", async () => {
		setupCreateDialog();
		const user = await openCreateModal();
		await typeImage(user);

		await user.selectOptions(
			screen.getByLabelText(/restart policy/i),
			"always",
		);
		await user.click(screen.getByLabelText(/remove on exit/i));

		await submitForm(user);

		expect(createRequests.body).toEqual({
			image: "docker.io/library/redis:7",
			restartPolicy: "always",
			remove: true,
		});
	});

	it("sends user and timezone", async () => {
		setupCreateDialog();
		const user = await openCreateModal();
		await typeImage(user);

		await user.type(screen.getByLabelText(/user/i), "1000:1000");
		await user.type(screen.getByLabelText(/timezone/i), "Europe/Berlin");

		await submitForm(user);

		expect(createRequests.body).toEqual({
			image: "docker.io/library/redis:7",
			user: "1000:1000",
			timezone: "Europe/Berlin",
		});
	});
});
