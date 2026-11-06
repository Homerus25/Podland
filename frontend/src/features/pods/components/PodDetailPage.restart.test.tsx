import { PodDetailPage } from "@features/pods/components/PodDetailPage";
import type { PodDetail } from "@features/pods/services/podsApi";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it } from "vitest";
import { hostFixture, podDetailFixture } from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

// The pod detail page's Restart action (spec #477 ticket #479): the row's
// button, page-sized in the header-actions slot. Restart takes no
// parameters (podman's defaults), so the assertion is the endpoint plus the
// refresh: the pods-prefix invalidation covers this page's detail key, so
// the state badge reflects the host's next report without a reload. The
// gate's classification is pinned once in the hook's suite and the
// allowed/denied/pending smokes live in the lifecycle suite; this suite
// keeps the request shape and the refresh.

// The host the detail GET reports, mutable so the action's success can
// change what the refetch answers.
let reported: PodDetail;
let restartRequests: URL[];

function mockHost() {
	server.use(
		http.get("*/api/hosts/m-1", () =>
			HttpResponse.json(hostFixture({ permission: "admin", isOwner: true })),
		),
	);
}

function renderPodDetailPage() {
	return renderWithQuery(
		<Routes>
			<Route path="/hosts/:hostId/pods/:podId" element={<PodDetailPage />} />
		</Routes>,
		{ router: { initialEntries: ["/hosts/m-1/pods/p-1"] } },
	);
}

describe("PodDetailPage restart action", () => {
	beforeEach(() => {
		restartRequests = [];
		// A stopped pod: the restart's own report is what flips the badge
		// back to Running, so the assertion cannot pass on the pre-click
		// state.
		reported = podDetailFixture({ state: "Exited" });
		mockHost();
		server.use(
			http.get("*/api/hosts/m-1/pods/p-1", () => HttpResponse.json(reported)),
			http.post("*/api/hosts/m-1/pods/p-1/restart", ({ request }) => {
				restartRequests.push(new URL(request.url));
				// The host's next detail report answers with the action's
				// outcome, so the badge refresh rides the refetch.
				reported = podDetailFixture({ state: "Running" });
				return HttpResponse.json({
					id: "p-1",
					state: "Running",
					timestamp: "2026-10-24T12:00:00Z",
				});
			}),
		);
	});

	it("renders the Restart action for a write-level caller", async () => {
		renderPodDetailPage();

		expect(await screen.findByTestId("restart-pod-button")).toHaveTextContent(
			"Restart",
		);
	});

	it("sends restart to the pod's endpoint and refreshes the state badge", async () => {
		const user = userEvent.setup();
		renderPodDetailPage();

		expect(await screen.findByTestId("pod-detail-state")).toHaveTextContent(
			"Exited",
		);
		await user.click(screen.getByTestId("restart-pod-button"));

		await waitFor(() =>
			expect(screen.getByTestId("pod-detail-state")).toHaveTextContent(
				"Running",
			),
		);
		expect(restartRequests).toHaveLength(1);
		expect(restartRequests[0].pathname).toBe("/api/hosts/m-1/pods/p-1/restart");
		expect(restartRequests[0].search).toBe("");
	});
});
