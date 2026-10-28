import { PodList } from "@features/pods/components/PodList";
import type { PodSummary } from "@features/pods/services/podsApi";
import { LOG_ROUTE_PATTERN } from "@shared/logs/logRoute";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { Route, Routes } from "react-router-dom";
import { describe, expect, it } from "vitest";
import { LogsPage } from "@/pages/LogsPage";
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
	renderWithQuery(
		<Routes>
			<Route
				path="/machines/:machineId"
				element={<PodList machine={machineFixture({ permission })} />}
			/>
			<Route path={LOG_ROUTE_PATTERN} element={<LogsPage />} />
		</Routes>,
		{ router: { initialEntries: ["/machines/m-1"] } },
	);
}

// The pods logs wiring smoke (spec #402 ticket #408), mirroring the
// container and quadlet logs suites: the row's logs button navigates to
// the shared logs page, the pod logs endpoint gets the request, and the
// history renders. Tail/query/error invariants are pinned once in the log
// kit and viewer suites, not here.
describe("PodList logs", () => {
	it("wires a list row's logs button to the pod logs endpoint and renders its history", async () => {
		const requestedPaths: string[] = [];
		server.use(
			http.get("*/api/machines/m-1/pods", () =>
				podsListResponse([podFixture()]),
			),
			http.get("*/api/machines/m-1/pods/p-1/logs", ({ request }) => {
				requestedPaths.push(new URL(request.url).pathname);
				return HttpResponse.json({
					id: "p-1",
					logs: "hist-line-1\nhist-line-2",
				});
			}),
		);
		renderPodList();

		await screen.findByText("app-pod");
		const user = userEvent.setup();
		await user.click(screen.getByRole("button", { name: /logs.*app-pod/i }));

		expect(
			screen.getByRole("heading", { name: "Logs — app-pod" }),
		).toBeInTheDocument();
		expect(await screen.findAllByTestId("log-viewer-line")).toHaveLength(2);
		expect(screen.getByText(/hist-line-1/)).toBeInTheDocument();
		expect(screen.getByText(/hist-line-2/)).toBeInTheDocument();
		expect(requestedPaths.length).toBeGreaterThan(0);
		for (const path of requestedPaths) {
			expect(path).toBe("/api/machines/m-1/pods/p-1/logs");
		}
	});

	it("keeps the row's logs button for read-only users", async () => {
		// The pod logs endpoint reads at read level, so the logs button
		// stays visible below the manage capability while the write
		// affordances hide — the containers surface's precedent.
		mockPods([podFixture()]);
		renderPodList("read");

		expect(await screen.findByTestId("pod-row")).toBeInTheDocument();
		expect(
			screen.getByRole("button", { name: /logs.*app-pod/i }),
		).toBeInTheDocument();
		expect(screen.queryByTestId("pause-pod-button")).not.toBeInTheDocument();
	});
});
