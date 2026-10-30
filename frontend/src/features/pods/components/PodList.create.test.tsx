import { PodList } from "@features/pods/components/PodList";
import type { PodSummary } from "@features/pods/services/podsApi";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import {
	machineFixture,
	networkFixture,
	podFixture,
	volumeFixture,
} from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

function podsListResponse(pods: PodSummary[]) {
	return HttpResponse.json({
		pods,
		total: pods.length,
		timestamp: "2026-10-24T12:00:00Z",
	});
}

function renderPodList() {
	return renderWithQuery(<PodList machine={machineFixture()} />, {
		router: true,
	});
}

// The full-form flow drives a long userEvent chain; under full-suite
// parallel load it outgrows vitest's default 5s test timeout, so the
// file sets its own.
const TEST_TIMEOUT = 20_000;

// The create flow (spec 352): the toolbar's Create pod button opens the
// form dialog, whose storage rows ride the shared row-editor module. The
// row ritual itself is pinned once in the shared RowEditor suite; this
// suite keeps the domain — the payload the form sends, the dialog's row
// nouns and copy wiring, and the resource-limit validation gate.
describe("PodList create", () => {
	it(
		"sends the full form as the create payload",
		async () => {
			const createRequests: unknown[] = [];
			server.use(
				http.get("*/api/machines/m-1/pods", () =>
					podsListResponse([podFixture()]),
				),
				http.get("*/api/machines/m-1/networks", () =>
					HttpResponse.json([
						networkFixture(),
						networkFixture({ name: "db-net", subnets: ["10.89.0.0/16"] }),
					]),
				),
				http.get("*/api/machines/m-1/volumes", () =>
					HttpResponse.json([
						volumeFixture(),
						volumeFixture({ name: "pgdata" }),
					]),
				),
				http.post("*/api/machines/m-1/pods", async ({ request }) => {
					createRequests.push(await request.json());
					return HttpResponse.json(
						{
							id: "p-new",
							name: "web",
							state: "Created",
							createdAt: "2026-10-24T12:00:00Z",
						},
						{ status: 201 },
					);
				}),
			);
			renderPodList();
			const user = userEvent.setup();

			await screen.findByTestId("pod-row");
			await user.click(screen.getByTestId("create-pod-button"));
			await screen.findByLabelText("Name");
			await user.type(screen.getByLabelText("Name"), "web");
			await user.click(await screen.findByLabelText("app-net"));
			// Port mapping published on the pod's infra container.
			await user.click(screen.getByTestId("add-port-mapping"));
			await user.type(screen.getByLabelText("Host port 1"), "8080");
			await user.type(screen.getByLabelText("Container port 1"), "80");
			await user.selectOptions(screen.getByLabelText("Protocol 1"), "udp");
			// Bind mount with the z flag.
			await user.click(screen.getByTestId("add-bind-mount"));
			await user.type(screen.getByLabelText("Mount source 1"), "/srv/app");
			await user.type(screen.getByLabelText("Mount destination 1"), "/app");
			await user.selectOptions(screen.getByLabelText("Mount mode 1"), "ro");
			await user.click(
				screen.getByRole("checkbox", {
					name: "Bind mount 1 SELinux shared (z)",
				}),
			);
			// Named volume picked from the machine's volume list.
			await user.click(screen.getByTestId("add-named-volume"));
			const volumeSelect = screen.getByLabelText("Volume name 1");
			await within(volumeSelect).findByText("pgdata");
			await user.selectOptions(volumeSelect, "pgdata");
			await user.type(screen.getByLabelText("Volume destination 1"), "/data");
			await user.selectOptions(screen.getByLabelText("Volume mode 1"), "ro");
			// Label entry.
			await user.click(screen.getByTestId("add-label"));
			await user.type(screen.getByLabelText("Label key 1"), "env");
			await user.type(screen.getByLabelText("Label value 1"), "prod");
			// Resource limits.
			await user.type(screen.getByLabelText("CPUs"), "1.5");
			await user.type(screen.getByLabelText("Memory"), "512m");

			await user.click(screen.getByTestId("pod-form-submit"));

			await waitFor(() => {
				expect(createRequests).toHaveLength(1);
			});
			expect(createRequests[0]).toEqual({
				name: "web",
				networks: ["app-net"],
				portMappings: [{ hostPort: 8080, containerPort: 80, protocol: "udp" }],
				mounts: [
					{
						source: "/srv/app",
						destination: "/app",
						mode: "ro",
						selinuxShared: true,
					},
				],
				volumes: [{ name: "pgdata", destination: "/data", mode: "ro" }],
				labels: { env: "prod" },
				cpus: 1.5,
				memory: "512m",
			});
		},
		TEST_TIMEOUT,
	);

	it("derives the storage rows' testids, copy, and remove aria labels", async () => {
		server.use(
			http.get("*/api/machines/m-1/pods", () =>
				podsListResponse([podFixture()]),
			),
			http.get("*/api/machines/m-1/networks", () =>
				HttpResponse.json([networkFixture()]),
			),
			http.get("*/api/machines/m-1/volumes", () =>
				HttpResponse.json([volumeFixture()]),
			),
		);
		renderPodList();
		const user = userEvent.setup();

		await screen.findByTestId("pod-row");
		await user.click(screen.getByTestId("create-pod-button"));
		await screen.findByLabelText("Name");

		expect(screen.getByTestId("add-bind-mount")).toHaveTextContent(
			"Add bind mount",
		);
		await user.click(screen.getByTestId("add-bind-mount"));
		await user.click(screen.getByTestId("add-bind-mount"));
		expect(screen.getByTestId("remove-bind-mount-0")).toHaveAttribute(
			"aria-label",
			"Remove bind mount 1",
		);
		expect(screen.getByTestId("remove-bind-mount-1")).toHaveAttribute(
			"aria-label",
			"Remove bind mount 2",
		);
		await user.click(screen.getByTestId("remove-bind-mount-0"));
		expect(screen.queryByTestId("remove-bind-mount-1")).not.toBeInTheDocument();
		expect(screen.getByLabelText("Mount source 1")).toBeInTheDocument();

		expect(screen.getByTestId("add-port-mapping")).toHaveTextContent(
			"Add port mapping",
		);
		await user.click(screen.getByTestId("add-port-mapping"));
		expect(screen.getByTestId("remove-port-mapping-0")).toHaveAttribute(
			"aria-label",
			"Remove port mapping 1",
		);
		await user.click(screen.getByTestId("remove-port-mapping-0"));
		expect(
			screen.queryByTestId("remove-port-mapping-0"),
		).not.toBeInTheDocument();

		expect(screen.getByTestId("add-named-volume")).toHaveTextContent(
			"Add volume",
		);
		await user.click(screen.getByTestId("add-named-volume"));
		expect(screen.getByTestId("remove-named-volume-0")).toHaveAttribute(
			"aria-label",
			"Remove named volume 1",
		);
	});

	it("filters incomplete port-mapping rows and defaults the protocol to tcp", async () => {
		const createRequests: unknown[] = [];
		server.use(
			http.get("*/api/machines/m-1/pods", () =>
				podsListResponse([podFixture()]),
			),
			http.get("*/api/machines/m-1/networks", () =>
				HttpResponse.json([networkFixture()]),
			),
			http.get("*/api/machines/m-1/volumes", () =>
				HttpResponse.json([volumeFixture()]),
			),
			http.post("*/api/machines/m-1/pods", async ({ request }) => {
				createRequests.push(await request.json());
				return HttpResponse.json(
					{
						id: "p-new",
						name: "web",
						state: "Created",
						createdAt: "2026-10-24T12:00:00Z",
					},
					{ status: 201 },
				);
			}),
		);
		renderPodList();
		const user = userEvent.setup();

		await screen.findByTestId("pod-row");
		await user.click(screen.getByTestId("create-pod-button"));
		await screen.findByLabelText("Name");
		await user.type(screen.getByLabelText("Name"), "web");

		// A complete row with the protocol left on its default.
		await user.click(screen.getByTestId("add-port-mapping"));
		await user.type(screen.getByLabelText("Host port 1"), "9090");
		await user.type(screen.getByLabelText("Container port 1"), "90");
		// An incomplete row — silently filtered, the same validation
		// feedback as the container create page's port bindings.
		await user.click(screen.getByTestId("add-port-mapping"));
		await user.type(screen.getByLabelText("Host port 2"), "8080");
		expect(screen.getByTestId("pod-form-submit")).toBeEnabled();

		await user.click(screen.getByTestId("pod-form-submit"));

		await waitFor(() => {
			expect(createRequests).toHaveLength(1);
		});
		expect(createRequests[0]).toEqual({
			name: "web",
			portMappings: [{ hostPort: 9090, containerPort: 90, protocol: "tcp" }],
		});
	});

	// The resource-limit fields render through the shared module (spec
	// 437, ticket #432) with the pod id prefix; the module's own suite
	// pins the labels, checks, and stacking once.
	it("renders the resource-limit fields through the shared module with the pod id prefix", async () => {
		server.use(
			http.get("*/api/machines/m-1/pods", () =>
				podsListResponse([podFixture()]),
			),
			http.get("*/api/machines/m-1/networks", () =>
				HttpResponse.json([networkFixture()]),
			),
			http.get("*/api/machines/m-1/volumes", () =>
				HttpResponse.json([volumeFixture()]),
			),
		);
		renderPodList();
		const user = userEvent.setup();

		await screen.findByTestId("pod-row");
		await user.click(screen.getByTestId("create-pod-button"));
		await screen.findByLabelText("Name");

		expect(screen.getByLabelText("CPUs")).toHaveAttribute("id", "pod-cpus");
		expect(screen.getByLabelText("Memory")).toHaveAttribute("id", "pod-memory");
	});

	it("blocks the submit while a resource limit is unparseable and explains why", async () => {
		server.use(
			http.get("*/api/machines/m-1/pods", () =>
				podsListResponse([podFixture()]),
			),
			http.get("*/api/machines/m-1/networks", () =>
				HttpResponse.json([networkFixture()]),
			),
			http.get("*/api/machines/m-1/volumes", () =>
				HttpResponse.json([volumeFixture()]),
			),
		);
		renderPodList();
		const user = userEvent.setup();

		await screen.findByTestId("pod-row");
		await user.click(screen.getByTestId("create-pod-button"));
		const name = await screen.findByLabelText("Name");
		await user.type(name, "web");
		expect(screen.getByTestId("pod-form-submit")).toBeEnabled();

		const memory = screen.getByLabelText("Memory");
		await user.type(memory, "abc");
		expect(
			screen.getByText("Use podman units syntax, e.g. 512m or 1g"),
		).toBeInTheDocument();
		expect(screen.getByTestId("pod-form-submit")).toBeDisabled();

		const cpus = screen.getByLabelText("CPUs");
		await user.type(cpus, "0");
		expect(
			screen.getByText("CPU limit must be a positive number"),
		).toBeInTheDocument();

		await user.clear(memory);
		await user.clear(cpus);
		await user.type(memory, "512m");
		expect(screen.getByTestId("pod-form-submit")).toBeEnabled();
	});
});
