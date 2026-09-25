import type { Machine } from "@shared/types/api";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import { renderApp } from "@/test/auth";
import { errorResponse } from "@/test/errors";
import { machineFixture } from "@/test/factories";
import { server } from "@/test/server";

function machineHandler(machine: Machine) {
	return http.get(`*/api/machines/${machine.id}`, () =>
		HttpResponse.json(machine),
	);
}

const emptyContainersHandler = http.get("*/api/machines/m-1/containers", () =>
	HttpResponse.json({
		containers: [],
		total: 0,
		timestamp: "2026-08-26T00:00:00Z",
	}),
);

const emptyQuadletsHandler = http.get("*/api/machines/m-1/quadlets", () =>
	HttpResponse.json([]),
);

const emptySecretsHandler = http.get("*/api/machines/m-1/secrets", () =>
	HttpResponse.json([]),
);

function renderMachineDetailPage(id = "m-1") {
	return renderApp(`/machines/${id}`, {
		resolveInitialState: async () => ({ username: "alice", role: "user" }),
	});
}

describe("MachineDetailPage", () => {
	it("navigates from the machines list into the detail page via the card click", async () => {
		server.use(
			http.get("*/api/machines", () => HttpResponse.json([machineFixture()])),
		);
		server.use(
			machineHandler(machineFixture()),
			emptyContainersHandler,
			emptyQuadletsHandler,
		);
		renderApp("/machines", {
			resolveInitialState: async () => ({ username: "alice", role: "user" }),
		});

		await userEvent.click(await screen.findByTestId("machine-card"));

		expect(await screen.findByText("prod-web-01")).toBeInTheDocument();
		expect(screen.getByText("web01.example.com:22")).toBeInTheDocument();
	});
	it("shows a loading state before the machine arrives", async () => {
		server.use(
			http.get("*/api/machines/m-1", async () => {
				await new Promise((resolve) => setTimeout(resolve, 50));
				return HttpResponse.json(machineFixture());
			}),
		);
		renderMachineDetailPage();

		expect(await screen.findByTestId("machine-loading")).toBeInTheDocument();
	});

	it("renders machine identity with permission badge", async () => {
		server.use(
			machineHandler(machineFixture()),
			emptyContainersHandler,
			emptyQuadletsHandler,
		);
		renderMachineDetailPage();

		expect(await screen.findByText("prod-web-01")).toBeInTheDocument();
		expect(screen.getByTestId("machine-endpoint")).toHaveTextContent(
			"web01.example.com:22",
		);
		expect(screen.getByTestId("machine-ssh-user")).toHaveTextContent("deploy");
		expect(screen.getByTestId("machine-permission")).toHaveTextContent("admin");
	});

	it("switches between the Containers, Images, Quadlets, Secrets, and Settings tabs", async () => {
		server.use(
			machineHandler(machineFixture()),
			emptyContainersHandler,
			emptyQuadletsHandler,
			emptySecretsHandler,
		);
		server.use(
			http.get("*/api/machines/m-1/images", () => HttpResponse.json([])),
		);
		renderMachineDetailPage();

		await screen.findByText("prod-web-01");
		expect(await screen.findByTestId("containers-empty")).toBeInTheDocument();
		expect(
			screen.queryByTestId("tab-containers-placeholder"),
		).not.toBeInTheDocument();

		await userEvent.click(screen.getByRole("tab", { name: /images/i }));
		expect(await screen.findByTestId("images-empty")).toBeInTheDocument();

		await userEvent.click(screen.getByRole("tab", { name: /quadlets/i }));
		expect(await screen.findByTestId("quadlets-empty")).toBeInTheDocument();
		expect(
			screen.queryByTestId("tab-quadlets-placeholder"),
		).not.toBeInTheDocument();

		await userEvent.click(screen.getByRole("tab", { name: /secrets/i }));
		expect(await screen.findByTestId("secrets-empty")).toBeInTheDocument();

		await userEvent.click(screen.getByRole("tab", { name: /settings/i }));
		expect(screen.getByTestId("machine-form-submit")).toBeInTheDocument();
	});

	it("renders the container list inside the Containers tab", async () => {
		server.use(machineHandler(machineFixture()), emptyQuadletsHandler);
		server.use(
			http.get("*/api/machines/m-1/containers", () =>
				HttpResponse.json({
					containers: [
						{
							id: "abc123def4567890",
							names: ["/prod-web"],
							image: "docker.io/library/nginx:1.27",
							createdAt: "2026-08-20T10:00:00Z",
							state: "running",
							status: "Up 2 hours",
						},
					],
					total: 1,
					timestamp: "2026-08-26T00:00:00Z",
				}),
			),
		);
		renderMachineDetailPage();

		expect(await screen.findByText("prod-web")).toBeInTheDocument();
		expect(
			screen.getByText("docker.io/library/nginx:1.27"),
		).toBeInTheDocument();
		expect(screen.getByTestId("state-badge")).toHaveTextContent("running");
	});

	it("renders the image list inside the Images tab", async () => {
		server.use(
			machineHandler(machineFixture()),
			emptyContainersHandler,
			emptyQuadletsHandler,
		);
		server.use(
			http.get("*/api/machines/m-1/images", () =>
				HttpResponse.json([
					{
						id: "sha256:abc123def4567890abc123def4567890abc123def4567890abc123def4567890",
						repoTags: ["docker.io/library/nginx:1.27"],
						repoDigests: [],
						createdAt: "2026-08-18T10:00:00Z",
						size: 157286400,
					},
				]),
			),
		);
		renderMachineDetailPage();

		await userEvent.click(await screen.findByRole("tab", { name: /images/i }));
		expect(
			await screen.findByText("docker.io/library/nginx:1.27"),
		).toBeInTheDocument();
		expect(screen.getByText("157.3 MB")).toBeInTheDocument();
	});

	it("renders the quadlet list inside the Quadlets tab", async () => {
		server.use(machineHandler(machineFixture()), emptyContainersHandler);
		server.use(
			http.get("*/api/machines/m-1/quadlets", () =>
				HttpResponse.json([
					{
						id: "web",
						name: "web",
						path: "/etc/containers/systemd/web.container",
						unit: "web.service",
						resource: "container",
						state: "running",
					},
				]),
			),
		);
		renderMachineDetailPage();

		await userEvent.click(
			await screen.findByRole("tab", { name: /quadlets/i }),
		);
		expect(await screen.findByText("web")).toBeInTheDocument();
		expect(screen.getByTestId("state-badge")).toHaveTextContent("running");
	});

	it("renders the secret list inside the Secrets tab", async () => {
		server.use(
			machineHandler(machineFixture()),
			emptyContainersHandler,
			emptyQuadletsHandler,
		);
		server.use(
			http.get("*/api/machines/m-1/secrets", () =>
				HttpResponse.json([
					{
						id: "3f2e1d0c",
						name: "db-password",
						driver: "file",
						createdAt: "2026-08-20T10:00:00Z",
						updatedAt: "2026-08-20T10:00:00Z",
					},
				]),
			),
		);
		renderMachineDetailPage();

		await userEvent.click(await screen.findByRole("tab", { name: /secrets/i }));
		expect(await screen.findByText("db-password")).toBeInTheDocument();
		expect(screen.getByText("file")).toBeInTheDocument();
	});

	it("renders the error envelope for an unknown machine id", async () => {
		server.use(
			http.get("*/api/machines/m-missing", () =>
				errorResponse(404, "MACHINE_NOT_FOUND", "Machine not found", "req-9"),
			),
		);
		renderMachineDetailPage("m-missing");

		expect(await screen.findByRole("alert")).toBeInTheDocument();
		expect(screen.getByText("MACHINE_NOT_FOUND")).toBeInTheDocument();
		expect(screen.getByText("Machine not found")).toBeInTheDocument();
	});
});

describe("page-level permission gating on the machine detail page", () => {
	it("gates from the detail response's own per-caller facts, never the machines list", async () => {
		// The list answers owner-admin for the same machine while the
		// detail answers read-grantee. Only the detail facts may drive
		// the page: the list query is not part of the detail page's data.
		server.use(
			http.get("*/api/machines", () =>
				HttpResponse.json([
					machineFixture({ permission: "admin", isOwner: true }),
				]),
			),
			http.get("*/api/machines/m-1", () =>
				HttpResponse.json(
					machineFixture({ permission: "read", isOwner: false }),
				),
			),
			http.get("*/api/machines/m-1/secrets", () => HttpResponse.json([])),
		);
		renderApp("/machines/m-1", {
			resolveInitialState: async () => ({ username: "alice", role: "user" }),
		});
		await screen.findByText(machineFixture().name);
		expect(screen.getByTestId("machine-permission")).toHaveTextContent("read");

		await userEvent.click(screen.getByRole("tab", { name: /secrets/i }));
		await screen.findByTestId("secrets-permission-hint");
		expect(
			screen.queryByTestId("create-secret-button"),
		).not.toBeInTheDocument();
	});
});
