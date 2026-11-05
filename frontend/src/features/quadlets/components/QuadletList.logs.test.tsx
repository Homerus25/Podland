import { QuadletList } from "@features/quadlets/components/QuadletList";
import { LOG_ROUTE_PATTERN } from "@shared/logs/logRoute";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { Route, Routes } from "react-router-dom";
import { describe, expect, it } from "vitest";
import { LogsPage } from "@/pages/LogsPage";
import { hostFixture, quadletFixture } from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

function renderQuadletList() {
	renderWithQuery(
		<Routes>
			<Route
				path="/hosts/:hostId"
				element={<QuadletList host={hostFixture()} />}
			/>
			<Route path={LOG_ROUTE_PATTERN} element={<LogsPage />} />
		</Routes>,
		{ router: { initialEntries: ["/hosts/m-1"] } },
	);
}

describe("QuadletList logs", () => {
	it("wires a list row's logs button to the quadlet logs endpoint and renders its history", async () => {
		const requestedPaths: string[] = [];
		server.use(
			http.get("*/api/hosts/m-1/quadlets", () =>
				HttpResponse.json([quadletFixture()]),
			),
			http.get("*/api/hosts/m-1/quadlets/web/logs", ({ request }) => {
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
			expect(path).toBe("/api/hosts/m-1/quadlets/web/logs");
		}
	});

	// Spec #402 ticket #406 acceptance: a .pod entry's logs button wires to
	// the same quadlet logs endpoint — the resource field changes no code
	// path, so this is the verification the ticket asks for.
	it("wires a .pod quadlet row's logs button to the quadlet logs endpoint", async () => {
		const requestedPaths: string[] = [];
		server.use(
			http.get("*/api/hosts/m-1/quadlets", () =>
				HttpResponse.json([
					quadletFixture({
						id: "app-pod",
						name: "app-pod",
						path: "/etc/containers/systemd/app-pod.pod",
						unit: "app-pod.service",
						resource: "pod",
					}),
				]),
			),
			http.get("*/api/hosts/m-1/quadlets/app-pod/logs", ({ request }) => {
				requestedPaths.push(new URL(request.url).pathname);
				return HttpResponse.json({
					id: "app-pod",
					logs: "pod-line-1\npod-line-2",
				});
			}),
		);
		renderQuadletList();

		await screen.findByText("app-pod");
		const user = userEvent.setup();
		await user.click(screen.getByRole("button", { name: /logs.*app-pod/i }));

		expect(
			screen.getByRole("heading", { name: "Logs — app-pod" }),
		).toBeInTheDocument();
		expect(await screen.findAllByTestId("log-viewer-line")).toHaveLength(2);
		for (const path of requestedPaths) {
			expect(path).toBe("/api/hosts/m-1/quadlets/app-pod/logs");
		}
	});
});
