import { QuadletList } from "@features/quadlets/components/QuadletList";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { Route, Routes } from "react-router-dom";
import { describe, expect, it } from "vitest";
import { LogsPage } from "@/pages/LogsPage";
import { machineFixture, quadletFixture } from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

function renderQuadletList() {
	renderWithQuery(
		<Routes>
			<Route
				path="/machines/:machineId"
				element={<QuadletList machine={machineFixture()} />}
			/>
			<Route
				path="/machines/:machineId/logs/:kind/:targetId"
				element={<LogsPage />}
			/>
		</Routes>,
		{ router: { initialEntries: ["/machines/m-1"] } },
	);
}

describe("QuadletList logs", () => {
	it("wires a list row's logs button to the quadlet logs endpoint and renders its history", async () => {
		const requestedPaths: string[] = [];
		server.use(
			http.get("*/api/machines/m-1/quadlets", () =>
				HttpResponse.json([quadletFixture()]),
			),
			http.get("*/api/machines/m-1/quadlets/web/logs", ({ request }) => {
				requestedPaths.push(new URL(request.url).pathname);
				return HttpResponse.json({
					id: "web",
					logs: "hist-line-1\nhist-line-2",
				});
			}),
		);
		renderQuadletList();

		await screen.findByText("web");
		const user = userEvent.setup();
		await user.click(screen.getByRole("button", { name: /logs.*web/i }));

		expect(
			screen.getByRole("heading", { name: "Logs — web" }),
		).toBeInTheDocument();
		expect(await screen.findAllByTestId("log-viewer-line")).toHaveLength(2);
		expect(screen.getByText(/hist-line-1/)).toBeInTheDocument();
		expect(screen.getByText(/hist-line-2/)).toBeInTheDocument();
		expect(requestedPaths.length).toBeGreaterThan(0);
		for (const path of requestedPaths) {
			expect(path).toBe("/api/machines/m-1/quadlets/web/logs");
		}
	});
});
