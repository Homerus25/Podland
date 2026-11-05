import {
	containerCreatePath,
	detailPath,
	hostTabPath,
} from "@shared/routes/detailRoute";
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
	it("builds the host-scoped detail path in the route pattern's own shape", () => {
		expect(detailPath("m-1", "containers", "c-1")).toBe(
			"/hosts/m-1/containers/c-1",
		);
		expect(detailPath("m-1", "volumes", "app-data")).toBe(
			"/hosts/m-1/volumes/app-data",
		);
		expect(detailPath("m-1", "images", "sha256:abc")).toBe(
			"/hosts/m-1/images/sha256%3Aabc",
		);
		expect(detailPath("m-1", "networks", "app-net")).toBe(
			"/hosts/m-1/networks/app-net",
		);
		expect(detailPath("m-1", "secrets", "db-password")).toBe(
			"/hosts/m-1/secrets/db-password",
		);
	});

	it("percent-encodes hostId and id so ids with reserved characters still match the route", () => {
		expect(detailPath("m/1", "containers", "a b/c")).toBe(
			"/hosts/m%2F1/containers/a%20b%2Fc",
		);
	});
});

describe("hostTabPath", () => {
	it("builds the host page path with the tab the back button returns to", () => {
		expect(hostTabPath("m-1", "containers")).toBe("/hosts/m-1?tab=containers");
		expect(hostTabPath("m-1", "volumes")).toBe("/hosts/m-1?tab=volumes");
	});

	it("percent-encodes the hostId", () => {
		expect(hostTabPath("m/1", "networks")).toBe("/hosts/m%2F1?tab=networks");
	});
});

describe("containerCreatePath", () => {
	it("builds the create-page path in the containers area's route shape", () => {
		expect(containerCreatePath("m-1")).toBe("/hosts/m-1/containers/new");
	});

	it("percent-encodes the hostId", () => {
		expect(containerCreatePath("m/1")).toBe("/hosts/m%2F1/containers/new");
	});
});

// The router decodes params on the far side, so encoding in detailPath must
// round trip: the route still matches for ids containing reserved
// characters and the params come back as the originals.
function DetailProbe() {
	const { hostId, containerId } = useParams<{
		hostId: string;
		containerId: string;
	}>();
	return (
		<div>
			<span data-testid="probe-host">{hostId}</span>
			<span data-testid="probe-id">{containerId}</span>
		</div>
	);
}

function TabProbe() {
	const { hostId } = useParams<{ hostId: string }>();
	const location = useLocation();
	return (
		<div>
			<span data-testid="probe-host">{hostId}</span>
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
					path="/hosts/:hostId/containers/:containerId"
					element={<DetailProbe />}
				/>
			</Routes>,
			{ router: { initialEntries: ["/somewhere"] } },
		);

		const user = userEvent.setup();
		await user.click(screen.getByRole("link", { name: "open" }));

		expect(screen.getByTestId("probe-host")).toHaveTextContent("m/1");
		expect(screen.getByTestId("probe-id")).toHaveTextContent("a b/c");
	});
});

describe("hostTabPath round trip", () => {
	it("the host route matches the encoded path and decodes the hostId", async () => {
		renderWithQuery(
			<Routes>
				<Route
					path="*"
					element={<Link to={hostTabPath("m/1", "containers")}>back</Link>}
				/>
				<Route path="/hosts/:hostId" element={<TabProbe />} />
			</Routes>,
			{ router: { initialEntries: ["/somewhere"] } },
		);

		const user = userEvent.setup();
		await user.click(screen.getByRole("link", { name: "back" }));

		expect(screen.getByTestId("probe-host")).toHaveTextContent("m/1");
		expect(screen.getByTestId("probe-search")).toHaveTextContent(
			"?tab=containers",
		);
	});
});
