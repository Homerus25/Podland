import { QuadletDetailPage } from "@features/quadlets/components/QuadletDetailPage";
import type { QuadletDetail } from "@features/quadlets/services/quadletsApi";
import { LOG_ROUTE_PATTERN } from "@shared/logs/logRoute";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { Route, Routes, useLocation, useParams } from "react-router-dom";
import { beforeEach, describe, expect, it } from "vitest";
import { errorResponse } from "@/test/errors";
import {
	hostFixture,
	quadletDetailFixture,
	quadletMovePreviewFixture,
} from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

// The single-quadlet GET the page's overview rides.
function mockQuadlet(quadlet: QuadletDetail) {
	server.use(
		http.get("*/api/hosts/m-1/quadlets/web", () => HttpResponse.json(quadlet)),
	);
}

// The host query feeds the page's capability gates (move/copy, remove).
function mockHost(permission: "read" | "write" | "admin" = "admin") {
	server.use(
		http.get("*/api/hosts/m-1", () =>
			HttpResponse.json(
				hostFixture({ permission, isOwner: permission === "admin" }),
			),
		),
	);
}

function renderDetailPage() {
	return renderWithQuery(
		<Routes>
			<Route
				path="/hosts/:hostId/quadlets/:quadletId"
				element={<QuadletDetailPage />}
			/>
			<Route path={LOG_ROUTE_PATTERN} element={<LogsRouteProbe />} />
		</Routes>,
		{ router: { initialEntries: ["/hosts/m-1/quadlets/web"] } },
	);
}

// The logs route's stub: the Logs button must land here, with the quadlet's
// id as the target param.
function LogsRouteProbe() {
	const params = useParams();
	const location = useLocation();
	return (
		<div data-testid="logs-route-probe">
			{location.pathname} {params.kind} {params.targetId}
		</div>
	);
}

describe("QuadletDetailPage overview", () => {
	it("renders all overview attributes and the file content", async () => {
		mockHost("admin");
		mockQuadlet(quadletDetailFixture());
		renderDetailPage();

		expect(await screen.findByTestId("quadlet-detail-name")).toHaveTextContent(
			"web",
		);
		expect(screen.getByTestId("detail-page-title")).toHaveTextContent("web");
		expect(screen.getByTestId("quadlet-detail-resource")).toHaveTextContent(
			"container",
		);
		expect(screen.getByTestId("state-badge")).toHaveTextContent("running");
		expect(screen.getByTestId("quadlet-detail-path")).toHaveTextContent(
			"/etc/containers/systemd/web.container",
		);
		expect(screen.getByTestId("quadlet-detail-unit")).toHaveTextContent(
			"web.service",
		);
		const content = screen.getByTestId("quadlet-file-content");
		expect(content.tagName).toBe("PRE");
		expect(content).toHaveTextContent("[Container]");
		expect(content).toHaveTextContent("Image=docker.io/library/nginx:1.27");
	});

	it("links the back button to the host's quadlets tab", async () => {
		mockHost("admin");
		mockQuadlet(quadletDetailFixture());
		renderDetailPage();

		await screen.findByTestId("quadlet-detail-name");
		expect(screen.getByTestId("detail-back-button")).toHaveAttribute(
			"href",
			"/hosts/m-1?tab=quadlets",
		);
	});

	it("renders the loading state while the detail is pending", async () => {
		mockHost("admin");
		server.use(
			http.get("*/api/hosts/m-1/quadlets/web", async () => {
				await new Promise((resolve) => setTimeout(resolve, 50));
				return HttpResponse.json(quadletDetailFixture());
			}),
		);
		renderDetailPage();

		expect(screen.getByTestId("detail-page-loading")).toBeInTheDocument();
		await screen.findByTestId("quadlet-detail-name");
	});

	it("renders the error state when the detail request fails", async () => {
		mockHost("admin");
		server.use(
			http.get("*/api/hosts/m-1/quadlets/web", () =>
				errorResponse(403, "ACCESS_DENIED", "Access denied", "req-7"),
			),
		);
		renderDetailPage();

		const alert = await screen.findByTestId("detail-page-error");
		expect(alert).toHaveTextContent("ACCESS_DENIED");
	});

	it("renders the not-found state when the quadlet no longer exists", async () => {
		mockHost("admin");
		server.use(
			http.get("*/api/hosts/m-1/quadlets/web", () =>
				errorResponse(404, "QUADLET_NOT_FOUND", "Quadlet not found", "req-9"),
			),
		);
		renderDetailPage();

		expect(
			await screen.findByTestId("detail-page-not-found"),
		).toHaveTextContent("This item no longer exists on the host.");
		expect(screen.queryByTestId("detail-page-error")).not.toBeInTheDocument();
	});

	// Quadlets have no podman inspect report, so the page passes no inspect
	// slice and the shared layout renders no reveal (issue #472).
	it("renders no inspect JSON reveal", async () => {
		mockHost("admin");
		mockQuadlet(quadletDetailFixture());
		renderDetailPage();

		await screen.findByTestId("quadlet-detail-name");
		expect(
			screen.queryByTestId("toggle-inspect-button"),
		).not.toBeInTheDocument();
		expect(screen.queryByTestId("inspect-json")).not.toBeInTheDocument();
	});
});

describe("QuadletDetailPage actions", () => {
	// The host the detail GET reports, mutable so a lifecycle action's
	// success can change what the refetch answers — the state-freshness
	// contract is the refetch, not the mutation's own answer.
	let reported: QuadletDetail;

	function mockMutableQuadlet() {
		server.use(
			http.get("*/api/hosts/m-1/quadlets/web", () =>
				HttpResponse.json(reported),
			),
		);
	}

	function captureLifecycle(action: "start" | "stop" | "restart") {
		return http.post(
			`*/api/hosts/m-1/quadlets/web/${action}`,
			({ request }) => {
				lifecycleRequests.push(new URL(request.url));
				// The host's next detail report answers with the action's
				// outcome, so the badge refresh rides the refetch.
				reported = quadletDetailFixture({
					state: action === "stop" ? "stopped" : "running",
				});
				return HttpResponse.json({ id: "web", state: "running" });
			},
		);
	}

	let lifecycleRequests: URL[];
	let putRequests: { content: string }[];

	beforeEach(() => {
		lifecycleRequests = [];
		putRequests = [];
		reported = quadletDetailFixture();
		mockHost("admin");
		mockMutableQuadlet();
	});

	it("starts a stopped quadlet and refreshes the state badge from the host's report", async () => {
		reported = quadletDetailFixture({ state: "stopped" });
		server.use(captureLifecycle("start"));
		const user = userEvent.setup();
		renderDetailPage();

		expect(await screen.findByTestId("state-badge")).toHaveTextContent(
			"stopped",
		);
		await user.click(screen.getByTestId("start-quadlet-button"));

		expect(await screen.findByTestId("state-badge")).toHaveTextContent(
			"running",
		);
		expect(lifecycleRequests).toHaveLength(1);
		expect(lifecycleRequests[0].pathname).toBe(
			"/api/hosts/m-1/quadlets/web/start",
		);
	});

	it("stops a running quadlet through the lifecycle options modal with the timeout field", async () => {
		server.use(captureLifecycle("stop"));
		const user = userEvent.setup();
		renderDetailPage();

		await screen.findByTestId("state-badge");
		await user.click(screen.getByTestId("stop-quadlet-button"));

		const modal = await screen.findByTestId("modal");
		expect(modal).toHaveTextContent("Stop");
		expect(screen.getByTestId("lifecycle-timeout-input")).toBeInTheDocument();
		await user.type(screen.getByTestId("lifecycle-timeout-input"), "30");
		await user.click(screen.getByTestId("confirm-lifecycle-button"));

		expect(await screen.findByTestId("state-badge")).toHaveTextContent(
			"stopped",
		);
		expect(lifecycleRequests).toHaveLength(1);
		expect(lifecycleRequests[0].pathname).toBe(
			"/api/hosts/m-1/quadlets/web/stop",
		);
		expect(lifecycleRequests[0].searchParams.get("timeout")).toBe("30");
	});

	it("restarts a quadlet through the lifecycle options modal", async () => {
		server.use(captureLifecycle("restart"));
		const user = userEvent.setup();
		renderDetailPage();

		await screen.findByTestId("state-badge");
		await user.click(screen.getByTestId("restart-quadlet-button"));

		expect(await screen.findByTestId("modal")).toHaveTextContent("Restart");
		await user.click(screen.getByTestId("confirm-lifecycle-button"));

		expect(lifecycleRequests).toHaveLength(1);
		expect(lifecycleRequests[0].pathname).toBe(
			"/api/hosts/m-1/quadlets/web/restart",
		);
		// An untouched timeout input means systemd's own default: no query
		// parameter travels.
		expect(lifecycleRequests[0].search).toBe("");
	});

	it("navigates to the logs route when Logs is clicked", async () => {
		const user = userEvent.setup();
		renderDetailPage();

		await screen.findByTestId("state-badge");
		await user.click(screen.getByRole("button", { name: "Logs web" }));

		expect(await screen.findByTestId("logs-route-probe")).toHaveTextContent(
			"/hosts/m-1/logs/quadlet/web",
		);
	});

	it("opens the edit dialog seeded with the file content and saves the update", async () => {
		server.use(
			http.put("*/api/hosts/m-1/quadlets/web", async ({ request }) => {
				putRequests.push((await request.json()) as { content: string });
				return HttpResponse.json(
					quadletDetailFixture({ content: "[Container]\nImage=alpine:3.20\n" }),
				);
			}),
		);
		const user = userEvent.setup();
		renderDetailPage();

		await screen.findByTestId("state-badge");
		await user.click(screen.getByTestId("edit-quadlet-button"));

		const modal = await screen.findByTestId("modal");
		expect(modal).toHaveTextContent("Edit quadlet");
		const textarea = screen.getByLabelText(/Content/);
		expect(textarea).toHaveValue(
			"[Container]\nImage=docker.io/library/nginx:1.27\n",
		);
		await user.type(textarea, "alpine");
		await user.click(screen.getByTestId("quadlet-form-submit"));

		await waitFor(() => expect(putRequests).toHaveLength(1));
		expect(putRequests[0]).toEqual({
			content: "[Container]\nImage=docker.io/library/nginx:1.27\nalpine",
		});
	});

	it("shows Move/Copy to a manage-capable caller and opens the transfer dialog", async () => {
		server.use(
			http.get("*/api/hosts", () =>
				HttpResponse.json([
					hostFixture(),
					hostFixture({ id: "m-2", name: "staging-01" }),
				]),
			),
			http.get("*/api/hosts/m-1/quadlets/web/move-preview", () =>
				HttpResponse.json(quadletMovePreviewFixture()),
			),
		);
		const user = userEvent.setup();
		renderDetailPage();

		await screen.findByTestId("state-badge");
		await user.click(screen.getByTestId("move-quadlet-button"));

		expect(await screen.findByTestId("modal")).toBeInTheDocument();
	});

	it("renders no Move/Copy action for a read-only caller", async () => {
		mockHost("read");
		renderDetailPage();

		await screen.findByTestId("state-badge");
		expect(screen.queryByTestId("move-quadlet-button")).not.toBeInTheDocument();
	});
});
