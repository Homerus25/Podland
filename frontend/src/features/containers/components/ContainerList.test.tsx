import { ContainerList } from "@features/containers/components/ContainerList";
import type { ContainerSummary } from "@features/containers/services/containersApi";
import { containersQueryKey } from "@shared/queries/queryKeys";
import type { Machine, MachinePermission } from "@shared/types/api";
import { screen, waitFor, within } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import { containerFixture, machineFixture } from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

function containersResponse(containers: ContainerSummary[]) {
	return {
		containers,
		total: containers.length,
		timestamp: "2026-08-26T00:00:00Z",
	};
}

function mockContainers(containers: ContainerSummary[]) {
	server.use(
		http.get("*/api/machines/m-1/containers", () =>
			HttpResponse.json(containersResponse(containers)),
		),
	);
}

// Default fixture is the owner's own view, so the ungated suites below
// exercise the toolbar-on path; grantee fixtures override permission and
// isOwner together.
function renderContainerList(machine: Machine = machineFixture()) {
	return renderWithQuery(<ContainerList machine={machine} />, {
		router: true,
	});
}

function granteeWith(permission: MachinePermission): Machine {
	return machineFixture({ permission, isOwner: false });
}

describe("ContainerList permission gating", () => {
	it("hides write controls from read users and explains the access level instead", async () => {
		mockContainers([containerFixture()]);
		renderContainerList(granteeWith("read"));

		const row = await screen.findByTestId("container-row");
		expect(
			screen.queryByTestId("create-container-button"),
		).not.toBeInTheDocument();
		expect(
			screen.queryByTestId("start-container-button"),
		).not.toBeInTheDocument();
		expect(
			screen.queryByTestId("stop-container-button"),
		).not.toBeInTheDocument();
		expect(
			screen.queryByRole("button", { name: /delete/i }),
		).not.toBeInTheDocument();
		expect(
			within(row).getByRole("button", { name: /logs/i }),
		).toBeInTheDocument();
		const hint = screen.getByTestId("containers-permission-hint");
		expect(hint).toHaveAttribute("data-requirement", "write");
		expect(hint.textContent).toMatch(/read-only/i);
	});

	it.each(["write", "admin"] as const)(
		"shows the create affordance to %s users",
		async (permission) => {
			mockContainers([containerFixture()]);
			renderContainerList(granteeWith(permission));

			expect(
				await screen.findByTestId("create-container-button"),
			).toBeInTheDocument();
			expect(
				screen.queryByTestId("containers-permission-hint"),
			).not.toBeInTheDocument();
		},
	);

	it("shows the toolbar to the machine's owner", async () => {
		mockContainers([containerFixture()]);
		renderContainerList();

		expect(
			await screen.findByTestId("create-container-button"),
		).toBeInTheDocument();
		expect(
			screen.queryByTestId("containers-permission-hint"),
		).not.toBeInTheDocument();
	});
});

describe("ContainerList", () => {
	it("renders one row per container with name, image, and a state badge", async () => {
		mockContainers([
			containerFixture({ id: "abc123def4567890" }),
			containerFixture({
				id: "fedcba0987654321",
				names: ["/other-container"],
				image: "docker.io/library/redis:7",
				state: "exited",
			}),
		]);
		renderContainerList();

		await waitFor(() => {
			expect(screen.queryAllByTestId("container-row")).toHaveLength(2);
		});
		expect(screen.getByText("my-container")).toBeInTheDocument();
		expect(
			screen.getByText("docker.io/library/nginx:1.27"),
		).toBeInTheDocument();
		expect(screen.getByText("other-container")).toBeInTheDocument();
		expect(screen.getByText("docker.io/library/redis:7")).toBeInTheDocument();
		expect(
			screen.getAllByTestId("state-badge").map((badge) => badge.textContent),
		).toEqual(["running", "exited"]);
	});

	it("strips the leading slash podman puts on container names", async () => {
		mockContainers([containerFixture({ names: ["/slashy"] })]);
		renderContainerList();

		expect(await screen.findByText("slashy")).toBeInTheDocument();
		expect(screen.queryByText("/slashy")).not.toBeInTheDocument();
	});

	it("falls back to the short container id when the container has no name", async () => {
		mockContainers([containerFixture({ id: "abc123def4567890", names: [] })]);
		renderContainerList();

		expect(await screen.findByText("abc123def456")).toBeInTheDocument();
	});

	it.each([
		"running",
		"exited",
		"paused",
		"dead",
		"created",
		"restarting",
		"configured",
	])("shows a badge for a container in state %s", async (state) => {
		mockContainers([containerFixture({ state })]);
		renderContainerList();

		expect(await screen.findByText(state)).toBeInTheDocument();
	});

	it("caches containers under the per-machine query key hierarchy", async () => {
		mockContainers([containerFixture({ id: "abc123def4567890" })]);
		const { queryClient } = renderContainerList();

		await screen.findByText("my-container");
		const cached = queryClient.getQueryData<ContainerSummary[]>(
			containersQueryKey("m-1"),
		);
		expect(cached).toHaveLength(1);
		expect(cached?.[0].id).toBe("abc123def4567890");
	});

	it("shows a loading state before containers arrive, with the toolbar still above it", async () => {
		server.use(
			http.get("*/api/machines/m-1/containers", async () => {
				await new Promise((resolve) => setTimeout(resolve, 50));
				return HttpResponse.json(containersResponse([containerFixture()]));
			}),
		);
		renderContainerList();

		expect(screen.getByTestId("containers-loading")).toBeInTheDocument();
		expect(screen.getByTestId("create-container-button")).toBeInTheDocument();
	});
});
