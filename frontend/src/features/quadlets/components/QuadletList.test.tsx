import { QuadletList } from "@features/quadlets/components/QuadletList";
import type { QuadletSummary } from "@features/quadlets/services/quadletsApi";
import { quadletsQueryKey } from "@shared/queries/queryKeys";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import { hostFixture, quadletFixture } from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

function mockQuadlets(quadlets: QuadletSummary[]) {
	server.use(
		http.get("*/api/hosts/m-1/quadlets", () => HttpResponse.json(quadlets)),
	);
}

function renderQuadletList() {
	return renderWithQuery(<QuadletList host={hostFixture()} />, {
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

	it("caches quadlets under the per-host query key hierarchy", async () => {
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
			http.get("*/api/hosts/m-1/quadlets", async () => {
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

	// Spec 461, ticket #470: quadlets have no detail page, so the row's delete
	// button is gone and the bulk button is the only removal path — a
	// mis-click on a row can never destroy a quadlet. Spec 486, ticket #489
	// retired the row's Move/Copy button the same way: a transfer is a
	// selection-scoped toolbar action now.
	it("renders no per-row delete or move/copy button", async () => {
		mockQuadlets([quadletFixture()]);
		renderQuadletList();

		const row = await screen.findByTestId("quadlet-row");
		expect(
			within(row).queryByRole("button", { name: /delete/i }),
		).not.toBeInTheDocument();
		// The row keeps its surviving actions: lifecycle, logs, and edit.
		expect(
			within(row).getByRole("button", { name: "Restart web" }),
		).toBeInTheDocument();
		expect(
			within(row).getByRole("button", { name: "Edit web" }),
		).toBeInTheDocument();
		expect(
			within(row).queryByRole("button", { name: "Move or copy web" }),
		).not.toBeInTheDocument();
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
