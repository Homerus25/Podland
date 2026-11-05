import { ContainerList } from "@features/containers/components/ContainerList";
import { LOG_ROUTE_PATTERN } from "@shared/logs/logRoute";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { Route, Routes } from "react-router-dom";
import { describe, expect, it } from "vitest";
import { LogsPage } from "@/pages/LogsPage";
import { containerFixture, hostFixture } from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

function renderContainerList() {
	renderWithQuery(
		<Routes>
			<Route
				path="/hosts/:hostId"
				element={<ContainerList host={hostFixture()} />}
			/>
			<Route path={LOG_ROUTE_PATTERN} element={<LogsPage />} />
		</Routes>,
		{ router: { initialEntries: ["/hosts/m-1"] } },
	);
}

describe("ContainerList logs", () => {
	it("wires a list row's logs button to the container logs endpoint and renders its history", async () => {
		const requestedPaths: string[] = [];
		server.use(
			http.get("*/api/hosts/m-1/containers", () =>
				HttpResponse.json({
					containers: [containerFixture()],
					total: 1,
					timestamp: "2026-08-26T00:00:00Z",
				}),
			),
			http.get("*/api/hosts/m-1/containers/c-1/logs", ({ request }) => {
				requestedPaths.push(new URL(request.url).pathname);
				return HttpResponse.json({
					id: "c-1",
					logs: "hist-line-1\nhist-line-2",
				});
			}),
		);
		renderContainerList();

		await screen.findByText("my-container");
		const user = userEvent.setup();
		await user.click(
			screen.getByRole("button", { name: /logs.*my-container/i }),
		);

		expect(
			screen.getByRole("heading", { name: "Logs — my-container" }),
		).toBeInTheDocument();
		expect(await screen.findAllByTestId("log-viewer-line")).toHaveLength(2);
		expect(screen.getByText(/hist-line-1/)).toBeInTheDocument();
		expect(screen.getByText(/hist-line-2/)).toBeInTheDocument();
		expect(requestedPaths.length).toBeGreaterThan(0);
		for (const path of requestedPaths) {
			expect(path).toBe("/api/hosts/m-1/containers/c-1/logs");
		}
	});
});
