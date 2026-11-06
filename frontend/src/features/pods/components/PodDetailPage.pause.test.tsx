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

// The pod detail page's Pause/Unpause action (spec #477 ticket #479): the
// row's toggle, page-sized in the header-actions slot. The label follows
// the pod's state, the request hits the pod's own endpoint, and a success
// refetches the page's overview through the pods-prefix invalidation — the
// state badge flips from the host's next report, not from the mutation's
// own answer. The gate's classification is pinned once in the hook's
// suite; this suite keeps adapter-wiring smokes only.

// The host the detail GET reports, mutable so a lifecycle action's success
// can change what the refetch answers.
let reported: PodDetail;

function mockHost(permission: "read" | "write" | "admin" = "admin") {
	server.use(
		http.get("*/api/hosts/m-1", () =>
			HttpResponse.json(
				hostFixture({ permission, isOwner: permission === "admin" }),
			),
		),
	);
}

function mockMutablePod() {
	server.use(
		http.get("*/api/hosts/m-1/pods/p-1", () => HttpResponse.json(reported)),
	);
}

function captureAction(action: "pause" | "unpause") {
	return http.post(`*/api/hosts/m-1/pods/p-1/${action}`, ({ request }) => {
		actionRequests.push(new URL(request.url));
		// The host's next detail report answers with the action's outcome,
		// so the badge refresh rides the refetch.
		reported = podDetailFixture({
			state: action === "pause" ? "Paused" : "Running",
		});
		return HttpResponse.json({
			id: "p-1",
			state: action === "pause" ? "Paused" : "Running",
			timestamp: "2026-10-24T12:00:00Z",
		});
	});
}

let actionRequests: URL[];

function renderPodDetailPage() {
	return renderWithQuery(
		<Routes>
			<Route path="/hosts/:hostId/pods/:podId" element={<PodDetailPage />} />
		</Routes>,
		{ router: { initialEntries: ["/hosts/m-1/pods/p-1"] } },
	);
}

describe("PodDetailPage pause action", () => {
	beforeEach(() => {
		actionRequests = [];
		reported = podDetailFixture();
		mockHost();
		mockMutablePod();
	});

	it("renders the Pause action on a running pod", async () => {
		renderPodDetailPage();

		expect(await screen.findByTestId("pause-pod-button")).toHaveTextContent(
			"Pause",
		);
		expect(screen.queryByTestId("unpause-pod-button")).not.toBeInTheDocument();
	});

	it("renders the Unpause action on a paused pod", async () => {
		reported = podDetailFixture({ state: "Paused" });
		renderPodDetailPage();

		expect(await screen.findByTestId("unpause-pod-button")).toHaveTextContent(
			"Unpause",
		);
		expect(screen.queryByTestId("pause-pod-button")).not.toBeInTheDocument();
	});

	it("sends pause to the pod's endpoint and refreshes the state badge", async () => {
		server.use(captureAction("pause"));
		const user = userEvent.setup();
		renderPodDetailPage();

		expect(await screen.findByTestId("pod-detail-state")).toHaveTextContent(
			"Running",
		);
		await user.click(screen.getByTestId("pause-pod-button"));

		await waitFor(() =>
			expect(screen.getByTestId("pod-detail-state")).toHaveTextContent(
				"Paused",
			),
		);
		expect(actionRequests).toHaveLength(1);
		expect(actionRequests[0].pathname).toBe("/api/hosts/m-1/pods/p-1/pause");
	});

	it("sends unpause to a paused pod's endpoint and refreshes the state badge", async () => {
		reported = podDetailFixture({ state: "Paused" });
		server.use(captureAction("unpause"));
		const user = userEvent.setup();
		renderPodDetailPage();

		expect(await screen.findByTestId("pod-detail-state")).toHaveTextContent(
			"Paused",
		);
		await user.click(screen.getByTestId("unpause-pod-button"));

		await waitFor(() =>
			expect(screen.getByTestId("pod-detail-state")).toHaveTextContent(
				"Running",
			),
		);
		expect(actionRequests).toHaveLength(1);
		expect(actionRequests[0].pathname).toBe("/api/hosts/m-1/pods/p-1/unpause");
	});
});
