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

// The pod detail page's Kill action (spec #477 ticket #479): the row's
// button, page-sized in the header-actions slot. Kill takes no parameters
// (podman's default signal), so the assertion is the endpoint plus the
// refresh: the pods-prefix invalidation covers this page's detail key, so
// the state badge flips from the host's next report without a reload. The
// gate's classification is pinned once in the hook's suite and the
// allowed/denied/pending smokes live in the lifecycle suite; this suite
// keeps the request shape and the refresh.

// The host the detail GET reports, mutable so the action's success can
// change what the refetch answers.
let reported: PodDetail;
let killRequests: URL[];

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

describe("PodDetailPage kill action", () => {
	beforeEach(() => {
		killRequests = [];
		reported = podDetailFixture();
		mockHost();
		server.use(
			http.get("*/api/hosts/m-1/pods/p-1", () => HttpResponse.json(reported)),
			http.post("*/api/hosts/m-1/pods/p-1/kill", ({ request }) => {
				killRequests.push(new URL(request.url));
				// The host's next detail report answers with the action's
				// outcome, so the badge refresh rides the refetch.
				reported = podDetailFixture({ state: "Exited" });
				return HttpResponse.json({
					id: "p-1",
					state: "Exited",
					timestamp: "2026-10-24T12:00:00Z",
				});
			}),
		);
	});

	it("renders the Kill action for a write-level caller", async () => {
		renderPodDetailPage();

		expect(await screen.findByTestId("kill-pod-button")).toHaveTextContent(
			"Kill",
		);
	});

	it("sends kill to the pod's endpoint and refreshes the state badge", async () => {
		const user = userEvent.setup();
		renderPodDetailPage();

		expect(await screen.findByTestId("pod-detail-state")).toHaveTextContent(
			"Running",
		);
		await user.click(screen.getByTestId("kill-pod-button"));

		await waitFor(() =>
			expect(screen.getByTestId("pod-detail-state")).toHaveTextContent(
				"Exited",
			),
		);
		expect(killRequests).toHaveLength(1);
		expect(killRequests[0].pathname).toBe("/api/hosts/m-1/pods/p-1/kill");
		expect(killRequests[0].search).toBe("");
	});
});
