import { ContainerList } from "@features/containers/components/ContainerList";
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { Route, Routes, useParams } from "react-router-dom";
import { describe, expect, it } from "vitest";
import {
	containerFixture,
	hostFixture,
	rawInspectFixture,
} from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

const container = containerFixture({ id: "c-1", names: ["/web"] });

function containersHandler() {
	return http.get("*/api/hosts/m-1/containers", () =>
		HttpResponse.json({
			containers: [container],
			total: 1,
			timestamp: "2026-08-26T00:00:00Z",
		}),
	);
}

// The detail page's stand-in: the row click's navigation target, observed
// through the route params the real detail page reads.
function DetailProbe() {
	const { hostId, containerId } = useParams<{
		hostId: string;
		containerId: string;
	}>();
	return (
		<div>
			<span data-testid="probe-host">{hostId}</span>
			<span data-testid="probe-container">{containerId}</span>
		</div>
	);
}

// The logs page's stand-in: the logs button's navigation target, observed
// through the route params the real logs page reads.
function LogsProbe() {
	const { hostId, kind, targetId } = useParams<{
		hostId: string;
		kind: string;
		targetId: string;
	}>();
	return (
		<div>
			<span data-testid="probe-logs-host">{hostId}</span>
			<span data-testid="probe-logs-kind">{kind}</span>
			<span data-testid="probe-logs-target">{targetId}</span>
		</div>
	);
}

function renderContainerList() {
	renderWithQuery(
		<Routes>
			<Route
				path="/hosts/:hostId"
				element={<ContainerList host={hostFixture()} />}
			/>
			<Route
				path="/hosts/:hostId/containers/:containerId"
				element={<DetailProbe />}
			/>
			<Route
				path="/hosts/:hostId/logs/:kind/:targetId"
				element={<LogsProbe />}
			/>
		</Routes>,
		{ router: { initialEntries: ["/hosts/m-1"] } },
	);
}

describe("ContainerList row navigation", () => {
	it("navigates to the container's detail page when the row is clicked", async () => {
		server.use(containersHandler());
		renderContainerList();
		const user = userEvent.setup();

		await user.click(await screen.findByTestId("container-row"));

		expect(screen.getByTestId("probe-host")).toHaveTextContent("m-1");
		expect(screen.getByTestId("probe-container")).toHaveTextContent("c-1");
	});

	it("does not fetch the inspect report when the row is clicked", async () => {
		let inspectRequests = 0;
		server.use(
			containersHandler(),
			http.get("*/api/hosts/m-1/containers/*/inspect", () => {
				inspectRequests++;
				return HttpResponse.json(rawInspectFixture());
			}),
		);
		renderContainerList();
		const user = userEvent.setup();

		await user.click(await screen.findByTestId("container-row"));

		expect(screen.getByTestId("probe-container")).toHaveTextContent("c-1");
		expect(inspectRequests).toBe(0);
	});

	it("keeps the row's action buttons from navigating to the detail page", async () => {
		server.use(
			http.get("*/api/hosts/m-1/containers", () =>
				HttpResponse.json({
					containers: [
						containerFixture({ id: "c-1", names: ["/web"], state: "running" }),
						containerFixture({ id: "c-2", names: ["/db"], state: "exited" }),
					],
					total: 2,
					timestamp: "2026-08-26T00:00:00Z",
				}),
			),
			http.post("*/api/hosts/m-1/containers/*/stop", () =>
				HttpResponse.json({ id: "c-1" }),
			),
			http.post("*/api/hosts/m-1/containers/*/start", () =>
				HttpResponse.json({ id: "c-2" }),
			),
		);
		renderContainerList();
		const user = userEvent.setup();
		await screen.findAllByTestId("container-row");
		// Re-queried per click: the lifecycle mutations refetch the list.
		const rowAt = (index: number) =>
			within(screen.getAllByTestId("container-row")[index]);

		// start/stop, both lifecycle directions
		await user.click(rowAt(0).getByTestId("stop-container-button"));
		await user.click(rowAt(1).getByTestId("start-container-button"));
		expect(screen.queryByTestId("probe-container")).not.toBeInTheDocument();
		expect(screen.getAllByTestId("container-row")).toHaveLength(2);
	});

	it("navigates to the logs page when the logs button is clicked, not the detail page", async () => {
		server.use(containersHandler());
		renderContainerList();
		const user = userEvent.setup();

		const row = await screen.findByTestId("container-row");
		await user.click(within(row).getByRole("button", { name: "Logs web" }));

		// The logs button navigates to the logs page (stopPropagation held:
		// the row click did not fire the detail-page navigation).
		expect(screen.getByTestId("probe-logs-host")).toHaveTextContent("m-1");
		expect(screen.getByTestId("probe-logs-kind")).toHaveTextContent(
			"container",
		);
		expect(screen.getByTestId("probe-logs-target")).toHaveTextContent("c-1");
		expect(screen.queryByTestId("probe-container")).not.toBeInTheDocument();
	});
});
