import { PodList } from "@features/pods/components/PodList";
import type { PodSummary } from "@features/pods/services/podsApi";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import { errorResponse } from "@/test/errors";
import { hostFixture, podFixture } from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

function podsListResponse(pods: PodSummary[]) {
	return HttpResponse.json({
		pods,
		total: pods.length,
		timestamp: "2026-10-24T12:00:00Z",
	});
}

function renderPodList(permission: "read" | "write" | "admin" = "admin") {
	return renderWithQuery(<PodList host={hostFixture({ permission })} />, {
		router: true,
	});
}

// The prune flow (spec 352 ticket #355): the stay-open prune-report
// dialog with the pod noun — the confirm sends nothing until clicked, the
// report renders removed ids and per-item errors in place, and the pods
// list refetches. The dialog skeleton invariants are pinned once in the
// shared PruneReportDialog suite; this suite keeps the domain flow.
describe("PodList prune", () => {
	it("reports removed pod ids and per-item errors in place, then refetches", async () => {
		const pruneRequests: string[] = [];
		let listRequests = 0;
		server.use(
			http.get("*/api/hosts/m-1/pods", () => {
				listRequests++;
				return podsListResponse([podFixture()]);
			}),
			http.post("*/api/hosts/m-1/pods/prune", ({ request }) => {
				pruneRequests.push(request.url);
				return HttpResponse.json([
					{ id: "p-1" },
					{ id: "p-2", error: "pod has running containers" },
				]);
			}),
		);
		renderPodList();
		const user = userEvent.setup();

		await screen.findByTestId("pod-row");
		expect(screen.getByTestId("prune-pods-button")).toHaveTextContent(
			/^Prune$/,
		);
		await user.click(screen.getByTestId("prune-pods-button"));
		// The confirm is open; nothing sent yet.
		expect(pruneRequests).toHaveLength(0);

		await user.click(screen.getByTestId("confirm-prune-button"));

		expect(await screen.findByTestId("prune-report")).toBeInTheDocument();
		expect(screen.getAllByTestId("prune-report-row")).toHaveLength(2);
		expect(screen.getByText("p-1")).toBeInTheDocument();
		expect(screen.getByText("p-2")).toBeInTheDocument();
		expect(screen.getByText("pod has running containers")).toBeInTheDocument();
		await waitFor(() => {
			expect(listRequests).toBeGreaterThan(1);
		});
	});

	it("reports an empty prune result", async () => {
		server.use(
			http.get("*/api/hosts/m-1/pods", () => podsListResponse([podFixture()])),
			http.post("*/api/hosts/m-1/pods/prune", () => HttpResponse.json([])),
		);
		renderPodList();
		const user = userEvent.setup();

		await screen.findByTestId("pod-row");
		await user.click(screen.getByTestId("prune-pods-button"));
		await user.click(screen.getByTestId("confirm-prune-button"));

		expect(await screen.findByTestId("prune-report-empty")).toBeInTheDocument();
	});

	it("surfaces a prune failure in the dialog's error region", async () => {
		server.use(
			http.get("*/api/hosts/m-1/pods", () => podsListResponse([podFixture()])),
			// 5xx collapses to the generic message (getApiError), so the
			// domain-observable failure is a 4xx envelope code.
			http.post("*/api/hosts/m-1/pods/prune", () =>
				errorResponse(409, "POD_STATE_CONFLICT", "pod is in use"),
			),
		);
		renderPodList();
		const user = userEvent.setup();

		await screen.findByTestId("pod-row");
		await user.click(screen.getByTestId("prune-pods-button"));
		await user.click(screen.getByTestId("confirm-prune-button"));

		expect(await screen.findByText("POD_STATE_CONFLICT")).toBeInTheDocument();
	});
});
