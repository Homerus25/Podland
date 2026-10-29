import { ContainerDetailPage } from "@features/containers/components/ContainerDetailPage";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { Route, Routes } from "react-router-dom";
import { describe, expect, it } from "vitest";
import {
	containerDetailFixture,
	containerStatsFixture,
} from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

function mockDetail() {
	server.use(
		http.get("*/api/machines/m-1/containers/c-1", () =>
			HttpResponse.json(containerDetailFixture()),
		),
	);
}

function mockContainerStats() {
	let requests = 0;
	server.use(
		http.get("*/api/machines/m-1/containers/c-1/stats", () => {
			requests++;
			return HttpResponse.json(containerStatsFixture());
		}),
	);
	return {
		requestCount: () => requests,
	};
}

function renderDetailPage() {
	return renderWithQuery(
		<Routes>
			<Route
				path="/machines/:machineId/containers/:containerId"
				element={<ContainerDetailPage />}
			/>
		</Routes>,
		{ router: { initialEntries: ["/machines/m-1/containers/c-1"] } },
	);
}

describe("ContainerDetailPage stats dialog wiring", () => {
	it("opens the per-container stats dialog from the content area's Stats button", async () => {
		mockDetail();
		const stats = mockContainerStats();
		renderDetailPage();

		expect(stats.requestCount()).toBe(0);
		const statsButton = await screen.findByTestId("container-stats-button");
		// The button sits at the top of the content area, above the overview.
		expect(screen.getByTestId("container-detail-overview")).toBeInTheDocument();
		expect(
			statsButton.compareDocumentPosition(
				screen.getByTestId("container-detail-overview"),
			),
		).toBe(Node.DOCUMENT_POSITION_FOLLOWING);

		await userEvent.click(statsButton);
		expect(
			await screen.findByTestId("container-stats-table"),
		).toBeInTheDocument();
		expect(stats.requestCount()).toBe(1);
		expect(screen.getByTestId("container-stats-row")).toHaveTextContent("web");
	});
});
