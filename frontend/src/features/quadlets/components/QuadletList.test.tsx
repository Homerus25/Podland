import { QuadletList } from "@features/quadlets/components/QuadletList";
import type { QuadletSummary } from "@features/quadlets/services/quadletsApi";
import { quadletsQueryKey } from "@shared/queries/queryKeys";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import { machineFixture, quadletFixture } from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

function mockQuadlets(quadlets: QuadletSummary[]) {
	server.use(
		http.get("*/api/machines/m-1/quadlets", () => HttpResponse.json(quadlets)),
	);
}

function renderQuadletList() {
	return renderWithQuery(<QuadletList machine={machineFixture()} />, {
		router: true,
	});
}

describe("QuadletList", () => {
	it("renders one row per quadlet with name, resource, and a state badge", async () => {
		mockQuadlets([
			quadletFixture(),
			quadletFixture({
				id: "db",
				name: "db",
				path: "/etc/containers/systemd/db.volume",
				unit: "db-volume.service",
				resource: "volume",
				state: "stopped",
			}),
		]);
		renderQuadletList();

		await waitFor(() => {
			expect(screen.queryAllByTestId("quadlet-row")).toHaveLength(2);
		});
		expect(screen.getByText("web")).toBeInTheDocument();
		expect(screen.getByText("db")).toBeInTheDocument();
		expect(
			screen.getAllByTestId("state-badge").map((badge) => badge.textContent),
		).toEqual(["running", "stopped"]);
	});

	it("renders every quadlet resource type the backend reports", async () => {
		mockQuadlets([
			quadletFixture(),
			quadletFixture({
				id: "front",
				name: "front",
				path: "/etc/containers/systemd/front.pod",
				resource: "pod",
			}),
			quadletFixture({
				id: "app",
				name: "app",
				path: "/etc/containers/systemd/app.image",
				resource: "image",
			}),
			quadletFixture({
				id: "db",
				name: "db",
				path: "/etc/containers/systemd/db.volume",
				resource: "volume",
			}),
			quadletFixture({
				id: "net",
				name: "net",
				path: "/etc/containers/systemd/net.network",
				resource: "network",
			}),
		]);
		renderQuadletList();

		await waitFor(() => {
			expect(screen.queryAllByTestId("quadlet-row")).toHaveLength(5);
		});
		expect(screen.getByText("container")).toBeInTheDocument();
		expect(screen.getByText("pod")).toBeInTheDocument();
		expect(screen.getByText("image")).toBeInTheDocument();
		expect(screen.getByText("volume")).toBeInTheDocument();
		expect(screen.getByText("network")).toBeInTheDocument();
	});

	it("caches quadlets under the per-machine query key hierarchy", async () => {
		mockQuadlets([quadletFixture()]);
		const { queryClient } = renderQuadletList();

		await screen.findByText("web");
		const cached = queryClient.getQueryData<QuadletSummary[]>(
			quadletsQueryKey("m-1"),
		);
		expect(cached).toHaveLength(1);
		expect(cached?.[0].id).toBe("web");
	});

	it.each(["running", "stopped", "error", "unknown"])(
		"shows a badge for a quadlet in state %s",
		async (state) => {
			mockQuadlets([quadletFixture({ state })]);
			renderQuadletList();

			expect(await screen.findByTestId("state-badge")).toHaveTextContent(state);
		},
	);

	it("shows a loading state before quadlets arrive, with the toolbar still above it", async () => {
		server.use(
			http.get("*/api/machines/m-1/quadlets", async () => {
				await new Promise((resolve) => setTimeout(resolve, 50));
				return HttpResponse.json([quadletFixture()]);
			}),
		);
		renderQuadletList();

		expect(screen.getByTestId("quadlets-loading")).toBeInTheDocument();
		expect(screen.getByTestId("create-quadlet-button")).toBeInTheDocument();
	});

	it("opens the import compose modal from the toolbar button", async () => {
		mockQuadlets([quadletFixture()]);
		renderQuadletList();
		const user = userEvent.setup();

		await user.click(screen.getByTestId("import-compose-button"));

		expect(await screen.findByTestId("modal")).toBeInTheDocument();
		expect(screen.getByTestId("compose-content-input")).toBeInTheDocument();
	});

	it("sorts rows when a column header is clicked", async () => {
		mockQuadlets([
			quadletFixture({ id: "z-1", name: "zebra" }),
			quadletFixture({ id: "a-1", name: "apple" }),
		]);
		renderQuadletList();
		await screen.findAllByTestId("quadlet-row");
		const user = userEvent.setup();
		await user.click(screen.getByTestId("sort-quadlet-0"));
		const rows = screen.getAllByTestId("quadlet-row");
		expect(within(rows[0]).getByText("apple")).toBeInTheDocument();
		expect(within(rows[1]).getByText("zebra")).toBeInTheDocument();
	});
});
