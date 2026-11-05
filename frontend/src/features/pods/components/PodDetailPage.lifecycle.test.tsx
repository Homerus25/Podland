import { PodDetailPage } from "@features/pods/components/PodDetailPage";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { Route, Routes } from "react-router-dom";
import { describe, expect, it } from "vitest";
import { errorResponse } from "@/test/errors";
import { hostFixture, podDetailFixture } from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

// The page reads the host's per-caller facts for the manage gate.
function mockHost(permission: "read" | "write" | "admin" = "admin") {
	server.use(
		http.get("*/api/hosts/m-1", () =>
			HttpResponse.json(
				hostFixture({ permission, isOwner: permission === "admin" }),
			),
		),
	);
}

function mockHostError() {
	server.use(
		http.get("*/api/hosts/m-1", () =>
			errorResponse(500, "HOST_UNREACHABLE", "Host unreachable"),
		),
	);
}

function mockPodDetail(state = "Running") {
	server.use(
		http.get("*/api/hosts/m-1/pods/p-1", () =>
			HttpResponse.json(podDetailFixture({ state })),
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

// The pod detail page's start/stop wiring (spec 402 ticket #407): the
// button's label follows the pod's state, the request hits the pod's
// lifecycle endpoint, a success refetches the pod detail through MSW, a
// failure surfaces the API's error envelope, and below the manage
// capability the shared write hint replaces the buttons. The shared
// layout's skeleton is pinned once in the shared DetailPage suite; this
// suite keeps domain assertions only.
describe("PodDetailPage lifecycle actions", () => {
	it("sends stop to the running pod's endpoint and refetches the pod detail", async () => {
		const stopRequests: string[] = [];
		let detailRequests = 0;
		mockHost();
		server.use(
			http.get("*/api/hosts/m-1/pods/p-1", () => {
				detailRequests++;
				return HttpResponse.json(podDetailFixture({ state: "Running" }));
			}),
			http.post("*/api/hosts/m-1/pods/p-1/stop", ({ request }) => {
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
		mockHost();
		mockPodDetail("Exited");
		server.use(
			http.post("*/api/hosts/m-1/pods/p-1/start", ({ request }) => {
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
		mockHost();
		mockPodDetail();
		server.use(
			http.post("*/api/hosts/m-1/pods/p-1/stop", () =>
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

	// The gate renders from the hook's tri-state (spec #437 ticket #436):
	// allowed renders the lifecycle section, denied renders the shared
	// write hint — for a read-only caller and a failed host query
	// alike, the divergence fix — and pending renders neither. The
	// classification itself is pinned once in the hook's suite.
	it("shows the write hint in place of the lifecycle buttons for a read-only caller", async () => {
		mockHost("read");
		mockPodDetail();
		renderPodDetailPage();

		await screen.findByTestId("pod-detail-name");
		const hint = await screen.findByTestId("pod-lifecycle-permission-hint");
		expect(hint).toHaveAttribute("data-requirement", "write");
		expect(hint).toHaveTextContent(
			"You have read-only access to this host. Ask an owner or admin for write access to manage its pods.",
		);
		expect(screen.queryByTestId("stop-pod-button")).not.toBeInTheDocument();
		expect(screen.queryByTestId("start-pod-button")).not.toBeInTheDocument();
	});

	it("renders the write hint when the host query fails, not a silent gap", async () => {
		mockHostError();
		mockPodDetail();
		renderPodDetailPage();

		await screen.findByTestId("pod-detail-name");
		expect(
			await screen.findByTestId("pod-lifecycle-permission-hint"),
		).toBeInTheDocument();
		expect(screen.queryByTestId("stop-pod-button")).not.toBeInTheDocument();
		expect(screen.queryByTestId("start-pod-button")).not.toBeInTheDocument();
	});

	it("renders neither the lifecycle buttons nor the hint while the gate is pending", async () => {
		mockPodDetail();
		let release: () => void = () => {};
		const gate = new Promise<void>((resolve) => {
			release = resolve;
		});
		server.use(
			http.get("*/api/hosts/m-1", async () => {
				await gate;
				return HttpResponse.json(hostFixture({ permission: "write" }));
			}),
		);
		renderPodDetailPage();

		await screen.findByTestId("pod-detail-name");
		expect(screen.queryByTestId("stop-pod-button")).not.toBeInTheDocument();
		expect(screen.queryByTestId("start-pod-button")).not.toBeInTheDocument();
		expect(
			screen.queryByTestId("pod-lifecycle-permission-hint"),
		).not.toBeInTheDocument();

		release();
		await screen.findByTestId("stop-pod-button");
	});
});
