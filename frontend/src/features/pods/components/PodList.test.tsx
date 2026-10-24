import { PodList } from "@features/pods/components/PodList";
import type { PodSummary } from "@features/pods/services/podsApi";
import { podsQueryKey } from "@shared/queries/queryKeys";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { Route, Routes, useParams } from "react-router-dom";
import { describe, expect, it } from "vitest";
import { machineFixture, podFixture } from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

// The detail page's stand-in: the row click's navigation target, observed
// through the route params the real detail page reads.
function DetailProbe() {
	const { machineId, podId } = useParams<{
		machineId: string;
		podId: string;
	}>();
	return (
		<div>
			<span data-testid="probe-machine">{machineId}</span>
			<span data-testid="probe-pod">{podId}</span>
		</div>
	);
}

function mockPods(pods: PodSummary[]) {
	server.use(http.get("*/api/machines/m-1/pods", () => podsListResponse(pods)));
}

function podsListResponse(pods: PodSummary[]) {
	return HttpResponse.json({
		pods,
		total: pods.length,
		timestamp: "2026-10-24T12:00:00Z",
	});
}

function renderPodList(permission: "read" | "write" | "admin" = "admin") {
	return renderWithQuery(
		<PodList machine={machineFixture({ permission })} />,
		// The row navigates on click, so the list renders inside a router.
		{ router: true },
	);
}

describe("PodList permission gating", () => {
	it("hides write controls from read users and explains the access level instead", async () => {
		mockPods([podFixture()]);
		renderPodList("read");

		expect(await screen.findByTestId("pod-row")).toBeInTheDocument();
		expect(screen.queryByTestId("create-pod-button")).not.toBeInTheDocument();
		expect(screen.queryByTestId("prune-pods-button")).not.toBeInTheDocument();
		expect(screen.queryByTestId("pause-pod-button")).not.toBeInTheDocument();
		const hint = screen.getByTestId("pods-permission-hint");
		expect(hint).toHaveAttribute("data-requirement", "write");
		expect(hint.textContent).toMatch(/read-only/i);
	});

	it.each(["write", "admin"] as const)(
		"shows the management controls to %s users",
		async (permission) => {
			mockPods([podFixture()]);
			renderPodList(permission);

			expect(
				await screen.findByTestId("create-pod-button"),
			).toBeInTheDocument();
			expect(await screen.findByTestId("pod-row")).toBeInTheDocument();
			expect(screen.getByTestId("prune-pods-button")).toBeInTheDocument();
			expect(screen.getByTestId("pause-pod-button")).toBeInTheDocument();
			expect(
				screen.queryByTestId("pods-permission-hint"),
			).not.toBeInTheDocument();
		},
	);
});

describe("PodList", () => {
	it("renders one row per pod with name, state, container count, networks, and created date", async () => {
		mockPods([
			podFixture(),
			podFixture({
				id: "p-2",
				name: "db-pod",
				state: "Exited",
				containerCount: 1,
				networks: ["db-net"],
				createdAt: "2026-08-22T12:00:00Z",
			}),
		]);
		renderPodList();

		await waitFor(() => {
			expect(screen.queryAllByTestId("pod-row")).toHaveLength(2);
		});
		expect(screen.getByText("app-pod")).toBeInTheDocument();
		expect(screen.getByText("db-pod")).toBeInTheDocument();
		expect(screen.getByText("Running")).toBeInTheDocument();
		expect(screen.getByText("Exited")).toBeInTheDocument();
		expect(screen.getByText("db-net")).toBeInTheDocument();
	});

	it("caches pods under the per-machine query key hierarchy", async () => {
		mockPods([podFixture()]);
		const { queryClient } = renderPodList();

		await screen.findByText("app-pod");
		const cached = queryClient.getQueryData<PodSummary[]>(podsQueryKey("m-1"));
		expect(cached).toHaveLength(1);
		expect(cached?.[0].id).toBe("p-1");
	});

	it("shows a loading state before pods arrive", async () => {
		server.use(
			http.get("*/api/machines/m-1/pods", async () => {
				await new Promise((resolve) => setTimeout(resolve, 50));
				return podsListResponse([podFixture()]);
			}),
		);
		renderPodList();

		expect(screen.getByTestId("pods-loading")).toBeInTheDocument();
	});

	it("sorts rows when a column header is clicked", async () => {
		mockPods([
			podFixture({ id: "p-1", name: "zebra-pod" }),
			podFixture({ id: "p-2", name: "apple-pod" }),
		]);
		renderPodList();
		await screen.findAllByTestId("pod-row");
		const user = userEvent.setup();
		await user.click(screen.getByTestId("sort-pod-0"));
		const rows = screen.getAllByTestId("pod-row");
		expect(within(rows[0]).getByText("apple-pod")).toBeInTheDocument();
		expect(within(rows[1]).getByText("zebra-pod")).toBeInTheDocument();
	});

	it("navigates to the pod's detail page on row click", async () => {
		mockPods([podFixture()]);
		renderWithQuery(
			<Routes>
				<Route
					path="/machines/:machineId"
					element={<PodList machine={machineFixture()} />}
				/>
				<Route
					path="/machines/:machineId/pods/:podId"
					element={<DetailProbe />}
				/>
			</Routes>,
			{ router: { initialEntries: ["/machines/m-1"] } },
		);
		const user = userEvent.setup();

		await user.click(await screen.findByTestId("pod-row"));

		expect(screen.getByTestId("probe-machine")).toHaveTextContent("m-1");
		expect(screen.getByTestId("probe-pod")).toHaveTextContent("p-1");
	});
});
