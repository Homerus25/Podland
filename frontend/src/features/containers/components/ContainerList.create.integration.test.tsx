import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { errorJson } from "@/test/errors";
import {
	createHandler,
	createRequests,
	openCreateModal,
	optionRequests,
	populateAllFields,
	renderContainerList,
	reopenCreateModal,
	resetCounters,
	setupCreateDialog,
	submitForm,
	typeImage,
} from "./createContainerTestHarness";

// Integration tests for the extended create-container dialog (spec 313,
// ticket #318). They drive the real ContainerList -> CreateContainerDialog
// -> MSW handler path and assert the exact JSON payload the backend
// receives: the full 14-field create flow, empty-row filtering, dropdown
// loading, error-envelope value preservation, and the per-field payload
// shapes. ContainerList.create.test.tsx owns the per-field basics; this
// file owns the end-to-end contract.

// The heavy tests drive long userEvent interaction chains (the full 14-field
// flow, five dialog open/submit cycles); under full-suite parallel load they
// outgrow vitest's default 5s test timeout, so the file sets its own.
const TEST_TIMEOUT = 20_000;

describe("ContainerList create integration", () => {
	it(
		"sends the complete backend DTO payload when all 14 fields are populated",
		async () => {
			setupCreateDialog();
			const user = await openCreateModal();

			await populateAllFields(user);

			await submitForm(user);

			expect(createRequests.body).toEqual({
				image: "docker.io/library/redis:7",
				name: "cache",
				command: ["redis-server", "--appendonly", "yes"],
				user: "1000:1000",
				timezone: "Europe/Berlin",
				restartPolicy: "always",
				remove: true,
				environment: { FOO: "bar", MODE: "prod" },
				portBindings: [{ hostPort: 8080, containerPort: 80, protocol: "udp" }],
				mounts: [{ source: "/srv/app", destination: "/app", mode: "ro" }],
				volumes: [{ name: "pgdata", destination: "/data", mode: "rw" }],
				networks: ["app-net"],
				secrets: [
					{ source: "db-password", target: "/run/secrets/db-password" },
				],
				devices: [
					{
						hostPath: "/dev/dri/renderD128",
						containerPath: "/dev/dri/renderD128",
					},
				],
			});
		},
		TEST_TIMEOUT,
	);

	it(
		"filters empty rows from every field array",
		async () => {
			setupCreateDialog();
			const user = await openCreateModal();
			await typeImage(user);

			// Environment: a row with an empty key is skipped.
			await user.click(screen.getByTestId("add-env-variable"));
			await user.type(
				screen.getByLabelText("Environment variable 1 key"),
				"FOO",
			);
			await user.type(
				screen.getByLabelText("Environment variable 1 value"),
				"bar",
			);
			await user.click(screen.getByTestId("add-env-variable"));
			await user.type(
				screen.getByLabelText("Environment variable 2 value"),
				"ignored",
			);
			// Ports: a row with empty ports is skipped.
			await user.click(screen.getByTestId("add-port-binding"));
			await user.type(screen.getByLabelText("Host port 1"), "8080");
			await user.type(screen.getByLabelText("Container port 1"), "80");
			await user.click(screen.getByTestId("add-port-binding"));
			// Mounts: a row with an empty destination is skipped.
			await user.click(screen.getByTestId("add-bind-mount"));
			await user.type(screen.getByLabelText("Mount source 1"), "/srv/app");
			await user.type(screen.getByLabelText("Mount destination 1"), "/app");
			await user.click(screen.getByTestId("add-bind-mount"));
			await user.type(screen.getByLabelText("Mount source 2"), "/srv/other");
			// Volumes: a row with no selected name is skipped.
			await user.click(screen.getByTestId("add-named-volume"));
			const volumeSelect = screen.getByLabelText("Volume name 1");
			await within(volumeSelect).findByText("pgdata");
			await user.selectOptions(volumeSelect, "pgdata");
			await user.type(screen.getByLabelText("Volume destination 1"), "/data");
			await user.click(screen.getByTestId("add-named-volume"));
			await user.type(
				screen.getByLabelText("Volume destination 2"),
				"/var/lib",
			);
			// Devices: a row with an empty container path is skipped.
			await user.click(screen.getByTestId("add-device"));
			await user.type(
				screen.getByLabelText("Device host path 1"),
				"/dev/dri/renderD128",
			);
			await user.type(
				screen.getByLabelText("Device container path 1"),
				"/dev/dri/renderD128",
			);
			await user.click(screen.getByTestId("add-device"));
			await user.type(screen.getByLabelText("Device host path 2"), "/dev/kvm");

			await submitForm(user);

			expect(createRequests.body).toEqual({
				image: "docker.io/library/redis:7",
				environment: { FOO: "bar" },
				portBindings: [{ hostPort: 8080, containerPort: 80, protocol: "tcp" }],
				mounts: [{ source: "/srv/app", destination: "/app", mode: "rw" }],
				volumes: [{ name: "pgdata", destination: "/data", mode: "rw" }],
				devices: [
					{
						hostPath: "/dev/dri/renderD128",
						containerPath: "/dev/dri/renderD128",
					},
				],
			});
		},
		TEST_TIMEOUT,
	);

	it(
		"omits every optional field when all rows are empty",
		async () => {
			setupCreateDialog();
			const user = await openCreateModal();
			await typeImage(user);

			await user.click(screen.getByTestId("add-env-variable"));
			await user.click(screen.getByTestId("add-port-binding"));
			await user.click(screen.getByTestId("add-bind-mount"));
			await user.click(screen.getByTestId("add-named-volume"));
			await user.click(screen.getByTestId("add-device"));

			await submitForm(user);

			expect(createRequests.body).toEqual({
				image: "docker.io/library/redis:7",
			});
		},
		TEST_TIMEOUT,
	);

	it(
		"loads networks, volumes, and secrets through MSW and populates the options",
		async () => {
			setupCreateDialog();
			const user = await openCreateModal();

			// The network checkboxes render from the fetched network list.
			expect(await screen.findByLabelText("app-net")).toBeInTheDocument();
			expect(screen.getByLabelText("other-net")).toBeInTheDocument();
			// The secret checkboxes render from the fetched secret list.
			expect(await screen.findByLabelText("db-password")).toBeInTheDocument();
			expect(screen.getByLabelText("api-token")).toBeInTheDocument();
			// The named-volume dropdown offers the fetched volumes.
			await user.click(screen.getByTestId("add-named-volume"));
			const volumeSelect = screen.getByLabelText("Volume name 1");
			expect(
				await within(volumeSelect).findByText("app-data"),
			).toBeInTheDocument();
			expect(within(volumeSelect).getByText("pgdata")).toBeInTheDocument();

			// Each list endpoint was requested exactly once through MSW.
			expect(optionRequests).toEqual({ networks: 1, volumes: 1, secrets: 1 });
		},
		TEST_TIMEOUT,
	);

	it(
		"keeps the dialog open and preserves every form value on a 404 IMAGE_NOT_FOUND",
		async () => {
			setupCreateDialog(
				createHandler(
					404,
					errorJson("IMAGE_NOT_FOUND", "image not found", "req-318"),
				),
			);
			const user = await openCreateModal();

			await populateAllFields(user, "docker.io/library/nope:latest");

			await submitForm(user);

			// The failed request still carried the populated fields.
			expect(createRequests.body).toMatchObject({
				image: "docker.io/library/nope:latest",
				name: "cache",
				restartPolicy: "always",
				remove: true,
				environment: { FOO: "bar", MODE: "prod" },
				portBindings: [{ hostPort: 8080, containerPort: 80, protocol: "udp" }],
				mounts: [{ source: "/srv/app", destination: "/app", mode: "ro" }],
				volumes: [{ name: "pgdata", destination: "/data", mode: "rw" }],
				networks: ["app-net"],
				secrets: [
					{ source: "db-password", target: "/run/secrets/db-password" },
				],
				devices: [
					{
						hostPath: "/dev/dri/renderD128",
						containerPath: "/dev/dri/renderD128",
					},
				],
			});

			// The error envelope renders in the dialog's error region and the
			// dialog stays open.
			expect(await screen.findByRole("alert")).toBeInTheDocument();
			expect(screen.getByText("IMAGE_NOT_FOUND")).toBeInTheDocument();
			expect(screen.getByText("image not found")).toBeInTheDocument();
			expect(screen.getByTestId("modal")).toBeInTheDocument();

			// Every populated field keeps its value.
			expect(
				screen.getByDisplayValue("docker.io/library/nope:latest"),
			).toBeInTheDocument();
			expect(screen.getByDisplayValue("cache")).toBeInTheDocument();
			expect(
				screen.getByDisplayValue("redis-server --appendonly yes"),
			).toBeInTheDocument();
			expect(screen.getByDisplayValue("1000:1000")).toBeInTheDocument();
			expect(screen.getByDisplayValue("Europe/Berlin")).toBeInTheDocument();
			expect(
				(screen.getByLabelText(/restart policy/i) as HTMLSelectElement).value,
			).toBe("always");
			expect(
				(screen.getByLabelText(/remove on exit/i) as HTMLInputElement).checked,
			).toBe(true);
			expect(screen.getByDisplayValue("FOO")).toBeInTheDocument();
			expect(screen.getByDisplayValue("bar")).toBeInTheDocument();
			expect(screen.getByDisplayValue("8080")).toBeInTheDocument();
			expect(screen.getByDisplayValue("80")).toBeInTheDocument();
			expect(screen.getByDisplayValue("/srv/app")).toBeInTheDocument();
			expect(screen.getByDisplayValue("/app")).toBeInTheDocument();
			expect(
				(screen.getByLabelText("Volume name 1") as HTMLSelectElement).value,
			).toBe("pgdata");
			expect(screen.getByDisplayValue("/data")).toBeInTheDocument();
			expect(
				(screen.getByLabelText("app-net") as HTMLInputElement).checked,
			).toBe(true);
			expect(
				(screen.getByLabelText("db-password") as HTMLInputElement).checked,
			).toBe(true);
			expect(screen.getAllByDisplayValue("/dev/dri/renderD128")).toHaveLength(
				2,
			);
		},
		TEST_TIMEOUT,
	);

	it(
		"sends the selected restart policy string and omits it for (none)",
		async () => {
			setupCreateDialog();
			renderContainerList();
			await screen.findByText("existing");
			const user = userEvent.setup();

			for (const policy of ["no", "always", "unless-stopped", "on-failure"]) {
				resetCounters();
				await reopenCreateModal(user);
				await typeImage(user);
				await user.selectOptions(
					screen.getByLabelText(/restart policy/i),
					policy,
				);
				await submitForm(user);
				expect(createRequests.body).toEqual({
					image: "docker.io/library/redis:7",
					restartPolicy: policy,
				});
				await waitFor(() =>
					expect(screen.queryByTestId("modal")).not.toBeInTheDocument(),
				);
			}

			// "(none)" leaves the field out of the payload entirely.
			resetCounters();
			await reopenCreateModal(user);
			await typeImage(user);
			await submitForm(user);
			expect(createRequests.body).toEqual({
				image: "docker.io/library/redis:7",
			});
		},
		TEST_TIMEOUT,
	);

	it(
		"sends remove: true when the remove-on-exit checkbox is checked",
		async () => {
			setupCreateDialog();
			const user = await openCreateModal();
			await typeImage(user);

			await user.click(screen.getByLabelText(/remove on exit/i));

			await submitForm(user);

			expect(createRequests.body).toEqual({
				image: "docker.io/library/redis:7",
				remove: true,
			});
		},
		TEST_TIMEOUT,
	);

	it(
		"omits remove when the remove-on-exit checkbox is left unchecked",
		async () => {
			setupCreateDialog();
			const user = await openCreateModal();
			await typeImage(user);

			await submitForm(user);

			expect(createRequests.body).toEqual({
				image: "docker.io/library/redis:7",
			});
		},
		TEST_TIMEOUT,
	);

	it(
		"sends selected secrets as { source, target } with the default target path",
		async () => {
			setupCreateDialog();
			const user = await openCreateModal();
			await typeImage(user);

			await user.click(await screen.findByLabelText("db-password"));
			await user.click(screen.getByLabelText("api-token"));

			await submitForm(user);

			expect(createRequests.body).toEqual({
				image: "docker.io/library/redis:7",
				secrets: [
					{ source: "db-password", target: "/run/secrets/db-password" },
					{ source: "api-token", target: "/run/secrets/api-token" },
				],
			});
		},
		TEST_TIMEOUT,
	);

	it(
		"sends the selected networks as a string array, dropping toggled-off ones",
		async () => {
			setupCreateDialog();
			const user = await openCreateModal();
			await typeImage(user);

			await user.click(await screen.findByLabelText("app-net"));
			await user.click(screen.getByLabelText("other-net"));
			// Toggling a selected network off removes it from the payload.
			await user.click(screen.getByLabelText("app-net"));

			await submitForm(user);

			expect(createRequests.body).toEqual({
				image: "docker.io/library/redis:7",
				networks: ["other-net"],
			});
		},
		TEST_TIMEOUT,
	);

	it(
		"sends device rows as { hostPath, containerPath } entries",
		async () => {
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
			await user.click(screen.getByTestId("add-device"));
			await user.type(screen.getByLabelText("Device host path 2"), "/dev/kvm");
			await user.type(
				screen.getByLabelText("Device container path 2"),
				"/dev/kvm",
			);

			await submitForm(user);

			expect(createRequests.body).toEqual({
				image: "docker.io/library/redis:7",
				devices: [
					{
						hostPath: "/dev/dri/renderD128",
						containerPath: "/dev/dri/renderD128",
					},
					{ hostPath: "/dev/kvm", containerPath: "/dev/kvm" },
				],
			});
		},
		TEST_TIMEOUT,
	);

	it(
		"offers the machine's volumes in the named-volume dropdown and sends { name, destination, mode }",
		async () => {
			setupCreateDialog();
			const user = await openCreateModal();
			await typeImage(user);

			await user.click(screen.getByTestId("add-named-volume"));
			const volumeSelect = screen.getByLabelText("Volume name 1");
			// The dropdown lists the volumes fetched through useVolumes.
			expect(
				await within(volumeSelect).findByText("app-data"),
			).toBeInTheDocument();
			expect(within(volumeSelect).getByText("pgdata")).toBeInTheDocument();
			await user.selectOptions(volumeSelect, "pgdata");
			await user.type(screen.getByLabelText("Volume destination 1"), "/data");
			await user.selectOptions(screen.getByLabelText("Volume mode 1"), "ro");

			await submitForm(user);

			expect(createRequests.body).toEqual({
				image: "docker.io/library/redis:7",
				volumes: [{ name: "pgdata", destination: "/data", mode: "ro" }],
			});
		},
		TEST_TIMEOUT,
	);
});
