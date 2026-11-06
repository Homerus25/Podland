import { PodDetailPage } from "@features/pods/components/PodDetailPage";
import { LOG_ROUTE_PATTERN } from "@shared/logs/logRoute";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { Route, Routes, useLocation, useParams } from "react-router-dom";
import { describe, expect, it } from "vitest";
import { hostFixture, podDetailFixture } from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

// The pod detail page's Logs action (spec #477 ticket #479): the row's
// button, page-sized in the header-actions slot, landing on the same logs
// route the row opens. Logs reads at read level, so it renders for every
// caller — a read-only caller keeps it while the lifecycle buttons give way
// to the shared write hint. The logs page's own invariants (tail, query,
// error states) are pinned once in the log kit and viewer suites.

function mockHost(permission: "read" | "write" | "admin" = "admin") {
	server.use(
		http.get("*/api/hosts/m-1", () =>
			HttpResponse.json(
				hostFixture({ permission, isOwner: permission === "admin" }),
			),
		),
	);
}

function mockPodDetail() {
	server.use(
		http.get("*/api/hosts/m-1/pods/p-1", () =>
			HttpResponse.json(podDetailFixture()),
		),
	);
}

// The logs route's stub: the Logs button must land here, with the pod's id
// as the target param.
function LogsRouteProbe() {
	const params = useParams();
	const location = useLocation();
	return (
		<div data-testid="logs-route-probe">
			{location.pathname} {params.kind} {params.targetId}
		</div>
	);
}

function renderPodDetailPage() {
	return renderWithQuery(
		<Routes>
			<Route path="/hosts/:hostId/pods/:podId" element={<PodDetailPage />} />
			<Route path={LOG_ROUTE_PATTERN} element={<LogsRouteProbe />} />
		</Routes>,
		{ router: { initialEntries: ["/hosts/m-1/pods/p-1"] } },
	);
}

describe("PodDetailPage logs action", () => {
	it("navigates to the pod's logs route when Logs is clicked", async () => {
		mockHost();
		mockPodDetail();
		const user = userEvent.setup();
		renderPodDetailPage();

		await screen.findByTestId("pod-detail-name");
		await user.click(screen.getByRole("button", { name: "Logs app-pod" }));

		expect(await screen.findByTestId("logs-route-probe")).toHaveTextContent(
			"/hosts/m-1/logs/pod/p-1",
		);
	});

	it("keeps the Logs action for a read-only caller", async () => {
		// The pod logs endpoint reads at read level, so the Logs button
		// stays visible below the manage capability while the lifecycle
		// affordances give way to the shared write hint — the row's
		// precedent. The hint is the Remove cascade's: the lifecycle
		// actions carry none of their own (the one-hint rule).
		mockHost("read");
		mockPodDetail();
		renderPodDetailPage();

		await screen.findByTestId("pod-detail-name");
		expect(
			await screen.findByTestId("pods-permission-hint"),
		).toBeInTheDocument();
		expect(
			screen.getByRole("button", { name: "Logs app-pod" }),
		).toBeInTheDocument();
		expect(screen.queryByTestId("pause-pod-button")).not.toBeInTheDocument();
	});

	// The header action row's one owner renders an ungated action whatever
	// the gate says, so a pending host query must not take Logs down with
	// the lifecycle buttons it is still deciding about.
	it("keeps the Logs action while the host query is pending", async () => {
		let release: () => void = () => {};
		const gate = new Promise<void>((resolve) => {
			release = resolve;
		});
		server.use(
			http.get("*/api/hosts/m-1", async () => {
				await gate;
				return HttpResponse.json(
					hostFixture({ permission: "admin", isOwner: true }),
				);
			}),
		);
		mockPodDetail();
		renderPodDetailPage();

		await screen.findByTestId("pod-detail-name");
		expect(
			screen.getByRole("button", { name: "Logs app-pod" }),
		).toBeInTheDocument();
		expect(screen.queryByTestId("pause-pod-button")).not.toBeInTheDocument();
		expect(
			screen.queryByTestId("pods-permission-hint"),
		).not.toBeInTheDocument();

		release();
		await screen.findByTestId("pause-pod-button");
	});
});
