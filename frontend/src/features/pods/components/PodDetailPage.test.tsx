import { PodDetailPage } from "@features/pods/components/PodDetailPage";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { Route, Routes } from "react-router-dom";
import { describe, expect, it } from "vitest";
import { errorResponse } from "@/test/errors";
import { podDetailFixture, rawInspectFixture } from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

function mockPodDetail() {
	server.use(
		http.get("*/api/hosts/m-1/pods/p-1", () =>
			HttpResponse.json(podDetailFixture()),
		),
		http.get("*/api/hosts/m-1/pods/p-1/inspect", () =>
			HttpResponse.json(rawInspectFixture({ Id: "p-1", Name: "app-pod" })),
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

// The pod detail page's domain assertions (spec 352 ticket #355): the
// overview rows for the detail DTO, the cross-links to the pod's
// containers, and the lazy inspect reveal. The shared layout's skeleton —
// loading/error/not-found states, the reveal toggle mechanics, the copy
// button, the back-button target — is pinned once in the shared
// DetailPage suite; this suite keeps domain assertions only.
describe("PodDetailPage", () => {
	it("renders the pod's name, id, state, created date, restart policy, labels, mounts, and networks", async () => {
		mockPodDetail();
		renderPodDetailPage();

		expect(await screen.findByTestId("pod-detail-name")).toHaveTextContent(
			"app-pod",
		);
		expect(screen.getByTestId("pod-detail-id")).toHaveTextContent("p-1");
		expect(screen.getByTestId("pod-detail-state")).toHaveTextContent("Running");
		expect(screen.getByTestId("pod-detail-created")).toBeInTheDocument();
		expect(screen.getByTestId("pod-detail-restart-policy")).toHaveTextContent(
			"always",
		);
		expect(screen.getByTestId("pod-detail-labels")).toHaveTextContent(
			"app=web",
		);
		expect(screen.getByTestId("pod-detail-mounts")).toHaveTextContent(
			"type=volume",
		);
		expect(screen.getByTestId("pod-detail-networks")).toHaveTextContent(
			"app-net",
		);
	});

	it("cross-links the pod's containers to their detail pages", async () => {
		mockPodDetail();
		renderPodDetailPage();

		const links = await screen.findAllByTestId("pod-detail-container-link");
		expect(links).toHaveLength(2);
		expect(links[0]).toHaveAttribute("href", "/hosts/m-1/containers/c-1");
		expect(links[1]).toHaveAttribute("href", "/hosts/m-1/containers/c-infra");
	});

	it("fetches the raw inspect report lazily on the first reveal", async () => {
		let inspectRequests = 0;
		server.use(
			http.get("*/api/hosts/m-1/pods/p-1", () =>
				HttpResponse.json(podDetailFixture()),
			),
			http.get("*/api/hosts/m-1/pods/p-1/inspect", () => {
				inspectRequests++;
				return HttpResponse.json(rawInspectFixture({ Id: "p-1" }));
			}),
		);
		renderPodDetailPage();
		const user = userEvent.setup();

		await screen.findByTestId("pod-detail-name");
		expect(inspectRequests).toBe(0);

		await user.click(screen.getByTestId("toggle-inspect-button"));

		await waitFor(() => {
			expect(inspectRequests).toBe(1);
		});
		expect(screen.getByTestId("inspect-json")).toHaveTextContent('"Id"');
	});

	it("answers a missing pod with the not-found state", async () => {
		server.use(
			http.get("*/api/hosts/m-1/pods/p-1", () =>
				errorResponse(404, "POD_NOT_FOUND", "pod not found"),
			),
		);
		renderPodDetailPage();

		expect(
			await screen.findByText("This item no longer exists on the host."),
		).toBeInTheDocument();
	});
});
