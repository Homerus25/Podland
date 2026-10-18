import { detailPath, machineTabPath } from "@shared/routes/detailRoute";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Link, Route, Routes, useLocation, useParams } from "react-router-dom";
import { describe, expect, it } from "vitest";
import { renderWithQuery } from "@/test/render";

// The route kit pins its own contract here: the path shapes, the
// percent-encoding, and the round trip through the router (the router
// decodes params on the far side, so encoding is safe). Feature suites
// keep only their wiring smokes.

describe("detailPath", () => {
	it("builds the machine-scoped detail path in the route pattern's own shape", () => {
		expect(detailPath("m-1", "containers", "c-1")).toBe(
			"/machines/m-1/containers/c-1",
		);
		expect(detailPath("m-1", "volumes", "app-data")).toBe(
			"/machines/m-1/volumes/app-data",
		);
		expect(detailPath("m-1", "images", "sha256:abc")).toBe(
			"/machines/m-1/images/sha256%3Aabc",
		);
		expect(detailPath("m-1", "networks", "app-net")).toBe(
			"/machines/m-1/networks/app-net",
		);
		expect(detailPath("m-1", "secrets", "db-password")).toBe(
			"/machines/m-1/secrets/db-password",
		);
	});

	it("percent-encodes machineId and id so ids with reserved characters still match the route", () => {
		expect(detailPath("m/1", "containers", "a b/c")).toBe(
			"/machines/m%2F1/containers/a%20b%2Fc",
		);
	});
});

describe("machineTabPath", () => {
	it("builds the machine page path with the tab the back button returns to", () => {
		expect(machineTabPath("m-1", "containers")).toBe(
			"/machines/m-1?tab=containers",
		);
		expect(machineTabPath("m-1", "volumes")).toBe("/machines/m-1?tab=volumes");
	});

	it("percent-encodes the machineId", () => {
		expect(machineTabPath("m/1", "networks")).toBe(
			"/machines/m%2F1?tab=networks",
		);
	});
});

// The router decodes params on the far side, so encoding in detailPath must
// round trip: the route still matches for ids containing reserved
// characters and the params come back as the originals.
function DetailProbe() {
	const { machineId, containerId } = useParams<{
		machineId: string;
		containerId: string;
	}>();
	return (
		<div>
			<span data-testid="probe-machine">{machineId}</span>
			<span data-testid="probe-id">{containerId}</span>
		</div>
	);
}

function TabProbe() {
	const { machineId } = useParams<{ machineId: string }>();
	const location = useLocation();
	return (
		<div>
			<span data-testid="probe-machine">{machineId}</span>
			<span data-testid="probe-search">{location.search}</span>
		</div>
	);
}

describe("detailPath round trip", () => {
	it("the mounted route pattern matches the encoded path and decodes the params", async () => {
		renderWithQuery(
			<Routes>
				<Route
					path="*"
					element={
						<Link to={detailPath("m/1", "containers", "a b/c")}>open</Link>
					}
				/>
				<Route
					path="/machines/:machineId/containers/:containerId"
					element={<DetailProbe />}
				/>
			</Routes>,
			{ router: { initialEntries: ["/somewhere"] } },
		);

		const user = userEvent.setup();
		await user.click(screen.getByRole("link", { name: "open" }));

		expect(screen.getByTestId("probe-machine")).toHaveTextContent("m/1");
		expect(screen.getByTestId("probe-id")).toHaveTextContent("a b/c");
	});
});

describe("machineTabPath round trip", () => {
	it("the machine route matches the encoded path and decodes the machineId", async () => {
		renderWithQuery(
			<Routes>
				<Route
					path="*"
					element={<Link to={machineTabPath("m/1", "containers")}>back</Link>}
				/>
				<Route path="/machines/:machineId" element={<TabProbe />} />
			</Routes>,
			{ router: { initialEntries: ["/somewhere"] } },
		);

		const user = userEvent.setup();
		await user.click(screen.getByRole("link", { name: "back" }));

		expect(screen.getByTestId("probe-machine")).toHaveTextContent("m/1");
		expect(screen.getByTestId("probe-search")).toHaveTextContent(
			"?tab=containers",
		);
	});
});
