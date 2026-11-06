import { ContainerDetailPage } from "@features/containers/components/ContainerDetailPage";
import { LOG_ROUTE_PATTERN } from "@shared/logs/logRoute";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { Route, Routes, useLocation, useParams } from "react-router-dom";
import { beforeEach, describe, expect, it } from "vitest";
import { containerDetailFixture, hostFixture } from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

// The logs route's stub: the Logs action must land here, with the container's
// id as the target param and the container kind bound by the page.
function LogsRouteProbe() {
	const params = useParams();
	const location = useLocation();
	return (
		<div data-testid="logs-route-probe">
			{location.pathname} {params.kind} {params.targetId}
		</div>
	);
}

function mockHost(permission: "read" | "write" | "admin" = "write") {
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
				path="/hosts/:hostId/containers/:containerId"
				element={<ContainerDetailPage />}
			/>
			<Route path={LOG_ROUTE_PATTERN} element={<LogsRouteProbe />} />
		</Routes>,
		{ router: { initialEntries: ["/hosts/m-1/containers/c-1"] } },
	);
}

describe("ContainerDetailPage logs", () => {
	beforeEach(() => {
		server.use(
			http.get("*/api/hosts/m-1/containers/c-1", () =>
				HttpResponse.json(containerDetailFixture()),
			),
		);
	});

	it("navigates to the container's logs route", async () => {
		mockHost("write");
		const user = userEvent.setup();
		renderDetailPage();

		await user.click(await screen.findByTestId("logs-container-button"));

		expect(await screen.findByTestId("logs-route-probe")).toHaveTextContent(
			"/hosts/m-1/logs/container/c-1",
		);
	});

	// Logs reads at read level, so it is the one action that survives the
	// page's write gates.
	it("renders Logs for a read-only caller while the write actions stay hidden", async () => {
		mockHost("read");
		renderDetailPage();

		expect(
			await screen.findByTestId("logs-container-button"),
		).toHaveTextContent("Logs");
		expect(
			screen.queryByTestId("stop-container-button"),
		).not.toBeInTheDocument();
		expect(
			screen.queryByTestId("connect-container-button"),
		).not.toBeInTheDocument();
	});

	// The header action row's one owner renders an ungated action whatever
	// the gate says, so a pending host query must not take Logs down with
	// the write actions it is still deciding about.
	it("renders Logs while the host query is pending", async () => {
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

		expect(
			await screen.findByTestId("logs-container-button"),
		).toHaveTextContent("Logs");
		expect(
			screen.queryByTestId("stop-container-button"),
		).not.toBeInTheDocument();
		expect(
			screen.queryByTestId("containers-permission-hint"),
		).not.toBeInTheDocument();

		release();
		await screen.findByTestId("stop-container-button");
	});
});
