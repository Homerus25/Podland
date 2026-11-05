import { LOG_ROUTE_PATTERN, logsPath } from "@shared/logs/logRoute";
import { screen } from "@testing-library/react";
import { Route, Routes } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";
import { LogsPage } from "@/pages/LogsPage";
import { renderWithQuery } from "@/test/render";

// The viewer's plumbing is pinned in its own suite; here it is a stub so
// the page suite needs no streaming endpoints.
vi.mock("@shared/components/LogViewer", () => ({
	LogViewer: () => <div data-testid="log-viewer-stub" />,
}));

function renderLogsPage() {
	renderWithQuery(
		<Routes>
			<Route path={LOG_ROUTE_PATTERN} element={<LogsPage />} />
		</Routes>,
		{ router: { initialEntries: [logsPath("m-1", "container", "c-1")] } },
	);
}

describe("LogsPage", () => {
	it("shows the target's name with a back button and mounts the viewer", () => {
		renderLogsPage();

		expect(
			screen.getByRole("heading", { name: "Logs — c-1" }),
		).toBeInTheDocument();
		expect(screen.getByTestId("logs-back-button")).toBeInTheDocument();
		expect(screen.getByTestId("log-viewer-stub")).toBeInTheDocument();
	});

	// jsdom proves no real 360px layout; this pins the style props that
	// produce it (spec #393's testing decision): an unbounded name in the
	// heading is the one overflow risk on this page. The clamp engages only
	// when the name overflows, so desktop headings that fit are unchanged.
	it("clamps the heading to one line so a long name cannot force page-level horizontal scroll", () => {
		renderLogsPage();

		const heading = screen.getByRole("heading", { name: "Logs — c-1" });
		expect(heading).toHaveStyle("overflow: hidden");
		expect(heading).toHaveStyle("min-width: 0px");
		expect(
			window.getComputedStyle(heading).getPropertyValue("--chakra-line-clamp"),
		).toBe("1");
	});
});
