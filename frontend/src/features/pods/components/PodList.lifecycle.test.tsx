import { PodList } from "@features/pods/components/PodList";
import type { PodSummary } from "@features/pods/services/podsApi";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { Route, Routes, useParams } from "react-router-dom";
import { describe, expect, it } from "vitest";
import { machineFixture, podFixture } from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

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
	return renderWithQuery(<PodList machine={machineFixture({ permission })} />, {
		router: true,
	});
}

// The lifecycle wiring suite (spec 352 ticket #355): the pause/unpause
// label follows the pod's state, each action's request hits its endpoint,
// and a successful action refetches the pods list through MSW. The
// dialog-module invariants (pending wiring, error region) are pinned once
// in the shared suites; this suite keeps domain assertions only.
describe("PodList lifecycle actions", () => {
	it("labels the state action Pause on a running pod and Unpause on a paused pod", async () => {
		mockPods([
			podFixture({ id: "p-1", state: "Running" }),
			podFixture({ id: "p-2", name: "paused-pod", state: "Paused" }),
		]);
		renderPodList();

		await screen.findAllByTestId("pod-row");
		expect(screen.getByTestId("pause-pod-button")).toBeInTheDocument();
		expect(screen.getByTestId("unpause-pod-button")).toBeInTheDocument();
	});

	it("sends pause to the paused pod's endpoint and refetches the list", async () => {
		const pauseRequests: string[] = [];
		let listRequests = 0;
		server.use(
			http.get("*/api/machines/m-1/pods", () => {
				listRequests++;
				return podsListResponse([
					podFixture({ id: "p-1", state: "Running" }),
					podFixture({ id: "p-2", name: "paused-pod", state: "Paused" }),
				]);
			}),
			http.post("*/api/machines/m-1/pods/*/unpause", ({ request }) => {
				pauseRequests.push(request.url);
				return HttpResponse.json({
					id: "p-2",
					state: "paused",
					timestamp: "2026-10-24T12:00:00Z",
				});
			}),
		);
		renderPodList();
		const user = userEvent.setup();

		await screen.findAllByTestId("pod-row");
		await user.click(screen.getByTestId("unpause-pod-button"));

		await waitFor(() => {
			expect(pauseRequests).toHaveLength(1);
		});
		expect(pauseRequests[0]).toContain("/pods/p-2/unpause");
		// The mutation invalidates the pods list; the refetch rides MSW.
		await waitFor(() => {
			expect(listRequests).toBeGreaterThan(1);
		});
	});

	it("sends kill and restart to the pod's endpoint", async () => {
		const killRequests: string[] = [];
		const restartRequests: string[] = [];
		server.use(
			http.get("*/api/machines/m-1/pods", () =>
				podsListResponse([podFixture()]),
			),
			http.post("*/api/machines/m-1/pods/*/kill", ({ request }) => {
				killRequests.push(request.url);
				return HttpResponse.json({
					id: "p-1",
					state: "exited",
					timestamp: "2026-10-24T12:00:00Z",
				});
			}),
			http.post("*/api/machines/m-1/pods/*/restart", ({ request }) => {
				restartRequests.push(request.url);
				return HttpResponse.json({
					id: "p-1",
					state: "running",
					timestamp: "2026-10-24T12:00:00Z",
				});
			}),
		);
		renderPodList();
		const user = userEvent.setup();

		await screen.findByTestId("pod-row");
		await user.click(screen.getByTestId("kill-pod-button"));
		await user.click(screen.getByTestId("restart-pod-button"));

		await waitFor(() => {
			expect(killRequests).toHaveLength(1);
			expect(restartRequests).toHaveLength(1);
		});
		expect(killRequests[0]).toContain("/pods/p-1/kill");
		expect(restartRequests[0]).toContain("/pods/p-1/restart");
	});

	it("keeps a lifecycle action button click from navigating to the detail page", async () => {
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

		await user.click(await screen.findByTestId("pause-pod-button"));

		expect(screen.queryByTestId("probe-pod")).not.toBeInTheDocument();
	});
});

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
