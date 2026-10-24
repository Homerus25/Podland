import { PodList } from "@features/pods/components/PodList";
import type { PodSummary } from "@features/pods/services/podsApi";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import { errorResponse } from "@/test/errors";
import { machineFixture, podFixture } from "@/test/factories";
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
	return renderWithQuery(<PodList machine={machineFixture({ permission })} />, {
		router: true,
	});
}

// The remove flow (spec 352 ticket #355): no request leaves the browser
// until the destructive confirm; a running pod's removal is refused with
// 409 POD_STATE_CONFLICT and the retry carries force=true. The dialog
// skeleton invariants are pinned once in the shared MutationDialog suite;
// this suite keeps the domain flow.
describe("PodList remove", () => {
	it("sends the remove only after the confirm and refetches the list", async () => {
		const removeRequests: string[] = [];
		let listRequests = 0;
		server.use(
			http.get("*/api/machines/m-1/pods", () => {
				listRequests++;
				return podsListResponse([podFixture()]);
			}),
			http.delete("*/api/machines/m-1/pods/*", ({ request }) => {
				removeRequests.push(request.url);
				return new HttpResponse(null, { status: 204 });
			}),
		);
		renderPodList();
		const user = userEvent.setup();

		await screen.findByTestId("pod-row");
		await user.click(screen.getByTestId("remove-pod-button"));
		// The confirm is open; nothing sent yet.
		expect(removeRequests).toHaveLength(0);

		await user.click(screen.getByTestId("confirm-delete-button"));

		await waitFor(() => {
			expect(removeRequests).toHaveLength(1);
		});
		expect(removeRequests[0]).not.toContain("force=true");
		await waitFor(() => {
			expect(listRequests).toBeGreaterThan(1);
		});
	});

	it("offers a forced retry after a running-refusal error", async () => {
		const removeRequests: { url: string; force: boolean }[] = [];
		server.use(
			http.get("*/api/machines/m-1/pods", () =>
				podsListResponse([podFixture({ state: "Running" })]),
			),
			http.delete("*/api/machines/m-1/pods/*", ({ request }) => {
				const url = new URL(request.url);
				removeRequests.push({
					url: request.url,
					force: url.searchParams.get("force") === "true",
				});
				if (url.searchParams.get("force") === "true") {
					return new HttpResponse(null, { status: 204 });
				}
				return errorResponse(
					409,
					"POD_STATE_CONFLICT",
					"pod has running containers",
				);
			}),
		);
		renderPodList();
		const user = userEvent.setup();

		await screen.findByTestId("pod-row");
		await user.click(screen.getByTestId("remove-pod-button"));
		await user.click(screen.getByTestId("confirm-delete-button"));

		// The refusal surfaces as the running warning; the footer swap and
		// the rest of the ritual are pinned in the shared MutationDialog suite.
		expect(
			await screen.findByTestId("pod-running-warning"),
		).toBeInTheDocument();

		await user.click(screen.getByTestId("force-remove-button"));

		await waitFor(() => {
			expect(removeRequests).toHaveLength(2);
		});
		expect(removeRequests[0].force).toBe(false);
		expect(removeRequests[1].force).toBe(true);
	});
});
