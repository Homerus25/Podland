import { ContainerList } from "@features/containers/components/ContainerList";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { describe, expect, it } from "vitest";
import { LogsPage } from "@/pages/LogsPage";
import { containerFixture } from "@/test/factories";
import { server } from "@/test/server";

describe("ContainerList logs", () => {
	it("wires a list row's logs button to the container logs endpoint and renders its history", async () => {
		const requestedPaths: string[] = [];
		server.use(
			http.get("*/api/machines/m-1/containers", () =>
				HttpResponse.json({
					containers: [containerFixture()],
					total: 1,
					timestamp: "2026-08-26T00:00:00Z",
				}),
			),
			http.get("*/api/machines/m-1/containers/c-1/logs", ({ request }) => {
				requestedPaths.push(new URL(request.url).pathname);
				return HttpResponse.json({
					id: "c-1",
					logs: "hist-line-1\nhist-line-2",
				});
			}),
		);
		render(
			<QueryClientProvider
				client={
					new QueryClient({
						defaultOptions: { queries: { retry: false } },
					})
				}
			>
				<MemoryRouter initialEntries={["/machines/m-1"]}>
					<Routes>
						<Route
							path="/machines/:machineId"
							element={<ContainerList machineId="m-1" />}
						/>
						<Route
							path="/machines/:machineId/logs/:kind/:targetId"
							element={<LogsPage />}
						/>
					</Routes>
				</MemoryRouter>
			</QueryClientProvider>,
		);

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
			expect(path).toBe("/api/machines/m-1/containers/c-1/logs");
		}
	});
});
