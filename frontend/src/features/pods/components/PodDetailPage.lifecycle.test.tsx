import { PodDetailPage } from "@features/pods/components/PodDetailPage";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { Route, Routes } from "react-router-dom";
import { describe, expect, it } from "vitest";
import { errorResponse } from "@/test/errors";
import { machineFixture, podDetailFixture } from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

// The page reads the machine's per-caller facts for the manage gate.
function mockMachine(permission: "read" | "write" | "admin" = "admin") {
	server.use(
		http.get("*/api/machines/m-1", () =>
			HttpResponse.json(
				machineFixture({ permission, isOwner: permission === "admin" }),
			),
		),
	);
}

function mockPodDetail(state = "Running") {
	server.use(
		http.get("*/api/machines/m-1/pods/p-1", () =>
			HttpResponse.json(podDetailFixture({ state })),
		),
	);
}

function renderPodDetailPage() {
	return renderWithQuery(
		<Routes>
			<Route
				path="/machines/:machineId/pods/:podId"
				element={<PodDetailPage />}
			/>
		</Routes>,
		{ router: { initialEntries: ["/machines/m-1/pods/p-1"] } },
	);
}

// The pod detail page's start/stop wiring (spec 402 ticket #407): the
// button's label follows the pod's state, the request hits the pod's
// lifecycle endpoint, a success refetches the pod detail through MSW, a
// failure surfaces the API's error envelope, and the buttons hide below
// the manage capability. The shared layout's skeleton is pinned once in
// the shared DetailPage suite; this suite keeps domain assertions only.
describe("PodDetailPage lifecycle actions", () => {
	it("sends stop to the running pod's endpoint and refetches the pod detail", async () => {
		const stopRequests: string[] = [];
		let detailRequests = 0;
		mockMachine();
		server.use(
			http.get("*/api/machines/m-1/pods/p-1", () => {
				detailRequests++;
				return HttpResponse.json(podDetailFixture({ state: "Running" }));
			}),
			http.post("*/api/machines/m-1/pods/p-1/stop", ({ request }) => {
				stopRequests.push(request.url);
				return HttpResponse.json({
					id: "p-1",
					state: "stopped",
					timestamp: "2026-10-24T12:00:00Z",
				});
			}),
		);
		renderPodDetailPage();
		const user = userEvent.setup();

		const button = await screen.findByTestId("stop-pod-button");
		await user.click(button);

		await waitFor(() => {
			expect(stopRequests).toHaveLength(1);
		});
		expect(stopRequests[0]).toContain("/pods/p-1/stop");
		// The mutation invalidates the pods prefix, which covers the detail
		// query; the refetch rides MSW.
		await waitFor(() => {
			expect(detailRequests).toBeGreaterThan(1);
		});
	});

	it("labels the button Start on a stopped pod and sends start to its endpoint", async () => {
		const startRequests: string[] = [];
		mockMachine();
		mockPodDetail("Exited");
		server.use(
			http.post("*/api/machines/m-1/pods/p-1/start", ({ request }) => {
				startRequests.push(request.url);
				return HttpResponse.json({
					id: "p-1",
					state: "running",
					timestamp: "2026-10-24T12:00:00Z",
				});
			}),
		);
		renderPodDetailPage();
		const user = userEvent.setup();

		await user.click(await screen.findByTestId("start-pod-button"));

		await waitFor(() => {
			expect(startRequests).toHaveLength(1);
		});
		expect(startRequests[0]).toContain("/pods/p-1/start");
	});

	it("shows the error envelope when the action fails", async () => {
		mockMachine();
		mockPodDetail();
		server.use(
			http.post("*/api/machines/m-1/pods/p-1/stop", () =>
				errorResponse(
					409,
					"POD_STATE_CONFLICT",
					"pod has containers with non-zero exit codes",
					"req-13",
				),
			),
		);
		renderPodDetailPage();
		const user = userEvent.setup();

		const button = await screen.findByTestId("stop-pod-button");
		await user.click(button);

		expect(await screen.findByRole("alert")).toBeInTheDocument();
		expect(screen.getByText("POD_STATE_CONFLICT")).toBeInTheDocument();
		expect(
			screen.getByText("pod has containers with non-zero exit codes"),
		).toBeInTheDocument();
		await waitFor(() => expect(button).toBeEnabled());
	});

	it("hides the lifecycle buttons below the manage capability", async () => {
		mockMachine("read");
		mockPodDetail();
		renderPodDetailPage();

		await screen.findByTestId("pod-detail-name");
		expect(screen.queryByTestId("stop-pod-button")).not.toBeInTheDocument();
		expect(screen.queryByTestId("start-pod-button")).not.toBeInTheDocument();
	});
});
