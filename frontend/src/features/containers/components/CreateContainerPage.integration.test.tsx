import { screen, waitFor, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { errorJson } from "@/test/errors";
import {
	createHandler,
	createRequests,
	openCreatePage,
	optionRequests,
	populateAllFields,
	reopenCreatePage,
	setField,
	setupCreatePage,
	submitForm,
	typeImage,
} from "./createContainerTestHarness";

// Integration tests for the create-container page (spec 341, ticket
// #344). They drive the real containers tab -> CreateContainerPage ->
// MSW handler path: one full-payload smoke (wiring, not mapping — the
// form-to-payload contract is pinned without a DOM in
// services/containersApi.create.test.ts, issue 328 candidate C3) plus
// the page-behavior pins — option-list fetching through MSW,
// error-envelope value preservation, and the discard-on-navigate reset.
// CreateContainerPage.test.tsx owns the form-shape, validation, and
// navigation pins.

// The heavy tests drive long userEvent interaction chains (the full
// 14-field flow, five page open/submit cycles); under full-suite
// parallel load they outgrow vitest's default 5s test timeout, so the
// file sets its own.
const TEST_TIMEOUT = 20_000;

describe("CreateContainerPage integration", () => {
	it(
		"sends the complete backend DTO payload when all 14 fields are populated",
		async () => {
			setupCreatePage();
			const user = await openCreatePage();

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
				cpus: 1.5,
				memory: "512m",
			});
		},
		TEST_TIMEOUT,
	);

	it(
		"loads networks, volumes, and secrets through MSW and populates the options",
		async () => {
			setupCreatePage();
			const user = await openCreatePage();

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

			// Each list endpoint was requested exactly once through MSW
			// (the pods list rides along for the pod selector, spec 356).
			expect(optionRequests).toEqual({
				networks: 1,
				volumes: 1,
				secrets: 1,
				pods: 1,
			});
		},
		TEST_TIMEOUT,
	);

	it(
		"stays on the page and preserves every form value on a 404 IMAGE_NOT_FOUND",
		async () => {
			setupCreatePage(
				createHandler(
					404,
					errorJson("IMAGE_NOT_FOUND", "image not found", "req-318"),
				),
			);
			const user = await openCreatePage();

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
				cpus: 1.5,
				memory: "512m",
			});

			// The error envelope renders in the page's error region and the
			// page stays put.
			expect(await screen.findByRole("alert")).toBeInTheDocument();
			expect(screen.getByText("IMAGE_NOT_FOUND")).toBeInTheDocument();
			expect(screen.getByText("image not found")).toBeInTheDocument();
			expect(screen.getByLabelText(/image/i)).toBeInTheDocument();

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
			expect(screen.getByDisplayValue("1.5")).toBeInTheDocument();
			expect(screen.getByDisplayValue("512m")).toBeInTheDocument();
		},
		TEST_TIMEOUT,
	);

	it(
		"discards the form silently when navigating away and returning",
		async () => {
			setupCreatePage();
			const user = await openCreatePage();
			typeImage();
			await user.type(screen.getByLabelText(/name/i), "cache");
			await submitForm(user);
			// The success navigation returned to the containers tab.
			await waitFor(() =>
				expect(screen.queryByLabelText(/image/i)).not.toBeInTheDocument(),
			);

			await reopenCreatePage(user);

			// The remounted page starts from the empty default form —
			// navigating away discarded the values with no guard.
			expect(screen.getByLabelText(/image/i)).toHaveValue("");
			expect(screen.getByLabelText(/name/i)).toHaveValue("");
			expect(screen.getByTestId("container-form-submit")).toBeDisabled();
		},
		TEST_TIMEOUT,
	);

	it(
		"sends the checked bind-mount flags and no flag fields for untouched rows",
		async () => {
			setupCreatePage();
			const user = await openCreatePage();
			typeImage();
			await user.click(screen.getByTestId("add-bind-mount"));
			setField(screen.getByLabelText("Mount source 1"), "/srv/app");
			setField(screen.getByLabelText("Mount destination 1"), "/app");
			await user.click(
				screen.getByLabelText("Bind mount 1 SELinux shared (z)"),
			);
			await user.click(screen.getByTestId("add-bind-mount"));
			setField(screen.getByLabelText("Mount source 2"), "/srv/other");
			setField(screen.getByLabelText("Mount destination 2"), "/opt");
			await user.click(screen.getByLabelText("Bind mount 2 chown to user (U)"));

			await submitForm(user);

			// The checked flags travel as the wire booleans; the untouched
			// row and the unchecked flags of both rows send nothing.
			const body = createRequests.body as {
				mounts: {
					selinuxShared?: boolean;
					selinuxPrivate?: boolean;
					chownUser?: boolean;
				}[];
			};
			expect(body.mounts).toEqual([
				{
					source: "/srv/app",
					destination: "/app",
					mode: "rw",
					selinuxShared: true,
				},
				{
					source: "/srv/other",
					destination: "/opt",
					mode: "rw",
					chownUser: true,
				},
			]);
		},
		TEST_TIMEOUT,
	);
});
