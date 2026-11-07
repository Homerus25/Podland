import { QuadletList } from "@features/quadlets/components/QuadletList";
import type { QuadletSummary } from "@features/quadlets/services/quadletsApi";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { Route, Routes, useParams } from "react-router-dom";
import { describe, expect, it } from "vitest";
import { hostFixture, quadletFixture } from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

function mockQuadlets(quadlets: QuadletSummary[]) {
	server.use(
		http.get("*/api/hosts/m-1/quadlets", () => HttpResponse.json(quadlets)),
	);
}

// The detail route's stub: the row click must land here, with the quadlet's
// id as the route param.
function QuadletDetailStub() {
	const { quadletId } = useParams<{ quadletId: string }>();
	return <div data-testid="quadlet-detail-page">{quadletId}</div>;
}

function renderQuadletList(permission: "read" | "write" | "admin" = "admin") {
	return renderWithQuery(
		<Routes>
			<Route
				path="/hosts/:hostId"
				element={
					<QuadletList
						host={hostFixture({
							permission,
							isOwner: permission === "admin",
						})}
					/>
				}
			/>
			<Route
				path="/hosts/:hostId/quadlets/:quadletId"
				element={<QuadletDetailStub />}
			/>
		</Routes>,
		{ router: { initialEntries: ["/hosts/m-1"] } },
	);
}

describe("QuadletList row navigation", () => {
	it("navigates to the quadlet's detail page when the row is clicked", async () => {
		mockQuadlets([quadletFixture()]);
		renderQuadletList();
		const user = userEvent.setup();

		await user.click(await screen.findByTestId("quadlet-row"));

		expect(await screen.findByTestId("quadlet-detail-page")).toHaveTextContent(
			"web",
		);
		expect(screen.queryByTestId("quadlet-row")).not.toBeInTheDocument();
	});

	it("navigates for read-only users too", async () => {
		mockQuadlets([quadletFixture()]);
		renderQuadletList("read");
		const user = userEvent.setup();

		await user.click(await screen.findByTestId("quadlet-row"));

		expect(await screen.findByTestId("quadlet-detail-page")).toHaveTextContent(
			"web",
		);
	});

	it("does not navigate when a row action button is clicked", async () => {
		mockQuadlets([quadletFixture({ state: "stopped" })]);
		server.use(
			http.post("*/api/hosts/m-1/quadlets/web/start", () =>
				HttpResponse.json({ id: "web", state: "running" }),
			),
		);
		renderQuadletList();
		const user = userEvent.setup();

		await user.click(await screen.findByRole("button", { name: "Start web" }));

		expect(screen.getByTestId("quadlet-row")).toBeInTheDocument();
		expect(screen.queryByTestId("quadlet-detail-page")).not.toBeInTheDocument();
	});

	it("does not navigate when a row action button is clicked", async () => {
		mockQuadlets([quadletFixture()]);
		renderQuadletList();
		const user = userEvent.setup();

		await user.click(await screen.findByRole("button", { name: "Edit web" }));

		expect(await screen.findByTestId("modal")).toBeInTheDocument();
		expect(screen.queryByTestId("quadlet-detail-page")).not.toBeInTheDocument();
		expect(screen.getByTestId("quadlet-row")).toBeInTheDocument();
	});
});
