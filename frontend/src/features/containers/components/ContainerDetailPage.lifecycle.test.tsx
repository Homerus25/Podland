import { ContainerDetailPage } from "@features/containers/components/ContainerDetailPage";
import type { ContainerDetail } from "@features/containers/services/containersApi";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it } from "vitest";
import { errorResponse } from "@/test/errors";
import { containerDetailFixture, hostFixture } from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

// The container the page's overview rides. The lifecycle handlers rewrite it
// inside the POST, so the refetched detail — not the mutation's own answer —
// is what flips the rendered state badge.
let reported: ContainerDetail;
let lifecycleRequests: { method: string; pathname: string }[];

function mockDetail() {
	server.use(
		http.get("*/api/hosts/m-1/containers/c-1", () =>
			HttpResponse.json(reported),
		),
	);
}

// The host query feeds the page's capability gates.
function mockHost(permission: "read" | "write" | "admin" = "write") {
	server.use(
		http.get("*/api/hosts/m-1", () =>
			HttpResponse.json(
				hostFixture({ permission, isOwner: permission === "admin" }),
			),
		),
	);
}

// One lifecycle endpoint: records the request and reports the container's
// new state on the next detail fetch.
function lifecycleHandler(
	action: "start" | "stop" | "restart",
	nextState: string,
) {
	return http.post(
		`*/api/hosts/m-1/containers/c-1/${action}`,
		({ request }) => {
			lifecycleRequests.push({
				method: request.method,
				pathname: new URL(request.url).pathname,
			});
			reported = { ...reported, state: nextState };
			return HttpResponse.json({ id: "c-1" });
		},
	);
}

function renderDetailPage() {
	return renderWithQuery(
		<Routes>
			<Route
				path="/hosts/:hostId/containers/:containerId"
				element={<ContainerDetailPage />}
			/>
		</Routes>,
		{ router: { initialEntries: ["/hosts/m-1/containers/c-1"] } },
	);
}

describe("ContainerDetailPage lifecycle", () => {
	beforeEach(() => {
		reported = containerDetailFixture();
		lifecycleRequests = [];
		mockHost("write");
		mockDetail();
	});

	it("renders Stop for a running container and Start for an exited one", async () => {
		renderDetailPage();

		expect(
			await screen.findByTestId("stop-container-button"),
		).toHaveTextContent("Stop");
		expect(
			screen.queryByTestId("start-container-button"),
		).not.toBeInTheDocument();
	});

	it("renders Start instead of Stop once the container is exited", async () => {
		reported = containerDetailFixture({ state: "exited" });
		renderDetailPage();

		expect(
			await screen.findByTestId("start-container-button"),
		).toHaveTextContent("Start");
		expect(
			screen.queryByTestId("stop-container-button"),
		).not.toBeInTheDocument();
	});

	it("posts to the stop endpoint and refreshes the state badge from the host's report", async () => {
		server.use(lifecycleHandler("stop", "exited"));
		const user = userEvent.setup();
		renderDetailPage();

		expect(await screen.findByTestId("state-badge")).toHaveTextContent(
			"running",
		);
		await user.click(screen.getByTestId("stop-container-button"));

		expect(await screen.findByTestId("state-badge")).toHaveTextContent(
			"exited",
		);
		expect(lifecycleRequests).toEqual([
			{ method: "POST", pathname: "/api/hosts/m-1/containers/c-1/stop" },
		]);
	});

	it("posts to the start endpoint and refreshes the state badge from the host's report", async () => {
		reported = containerDetailFixture({ state: "exited" });
		server.use(lifecycleHandler("start", "running"));
		const user = userEvent.setup();
		renderDetailPage();

		expect(await screen.findByTestId("state-badge")).toHaveTextContent(
			"exited",
		);
		await user.click(screen.getByTestId("start-container-button"));

		expect(await screen.findByTestId("state-badge")).toHaveTextContent(
			"running",
		);
		expect(lifecycleRequests).toEqual([
			{ method: "POST", pathname: "/api/hosts/m-1/containers/c-1/start" },
		]);
	});

	it("renders Restart only while the container is running", async () => {
		renderDetailPage();

		expect(
			await screen.findByTestId("restart-container-button"),
		).toHaveTextContent("Restart");
	});

	it("hides Restart once the container is exited", async () => {
		reported = containerDetailFixture({ state: "exited" });
		renderDetailPage();

		await screen.findByTestId("start-container-button");
		expect(
			screen.queryByTestId("restart-container-button"),
		).not.toBeInTheDocument();
	});

	it("posts to the restart endpoint for the container on Restart click", async () => {
		server.use(lifecycleHandler("restart", "running"));
		const user = userEvent.setup();
		renderDetailPage();

		await user.click(await screen.findByTestId("restart-container-button"));

		await waitFor(() =>
			expect(lifecycleRequests).toEqual([
				{ method: "POST", pathname: "/api/hosts/m-1/containers/c-1/restart" },
			]),
		);
	});

	// The gate smokes: the classification itself is pinned once in
	// useCapabilityGate's suite; these only wire the page to it. The
	// lifecycle actions carry no hint of their own (spec #477): Remove's
	// cascade already renders the write hint for the same capability, so a
	// denied caller reads one sentence, not two.
	it("renders the write-permission hint instead of the lifecycle actions for a read-only caller", async () => {
		mockHost("read");
		renderDetailPage();

		const hint = await screen.findByTestId("containers-permission-hint");
		expect(hint).toHaveAttribute("data-requirement", "write");
		// One hint per capability in the header: Remove's write hint for
		// manage-containers, and Connect's own for manage-networks — the
		// lifecycle actions add none.
		const header = screen.getByTestId("detail-page-header-actions");
		expect(
			within(header)
				.getAllByTestId(/permission-hint$/)
				.map((node) => node.getAttribute("data-testid"))
				.sort(),
		).toEqual([
			"container-connect-permission-hint",
			"containers-permission-hint",
		]);
		expect(
			screen.queryByTestId("container-lifecycle-permission-hint"),
		).not.toBeInTheDocument();
		expect(
			screen.queryByTestId("start-container-button"),
		).not.toBeInTheDocument();
		expect(
			screen.queryByTestId("stop-container-button"),
		).not.toBeInTheDocument();
		expect(
			screen.queryByTestId("restart-container-button"),
		).not.toBeInTheDocument();
	});

	it("renders neither the lifecycle actions nor the hint while the host query is pending", async () => {
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
		renderDetailPage();

		await screen.findByTestId("container-detail-name");
		expect(
			screen.queryByTestId("stop-container-button"),
		).not.toBeInTheDocument();
		expect(
			screen.queryByTestId("containers-permission-hint"),
		).not.toBeInTheDocument();

		release();
		await screen.findByTestId("stop-container-button");
	});

	it("keeps the page's existing header actions and does not duplicate them", async () => {
		renderDetailPage();

		await screen.findByTestId("stop-container-button");
		expect(screen.getAllByTestId("edit-configuration-button")).toHaveLength(1);
		expect(screen.getAllByTestId("remove-container-button")).toHaveLength(1);
		expect(screen.getAllByTestId("container-stats-button")).toHaveLength(1);
		expect(screen.getAllByTestId("rename-container-button")).toHaveLength(1);
	});

	it("reports a failed lifecycle action in place, like the row does", async () => {
		server.use(
			http.post("*/api/hosts/m-1/containers/c-1/stop", () =>
				errorResponse(
					409,
					"CONTAINER_STATE_CONFLICT",
					"container web is not running",
					"req-478",
				),
			),
		);
		const user = userEvent.setup();
		renderDetailPage();

		await user.click(await screen.findByTestId("stop-container-button"));

		expect(
			await screen.findByTestId("container-lifecycle-error"),
		).toHaveTextContent("container web is not running");
	});
});
