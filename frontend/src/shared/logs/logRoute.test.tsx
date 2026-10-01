import {
	isLogKind,
	LOG_ROUTE_PATTERN,
	type LogKind,
	type LogsPageState,
	logsPath,
	useOpenLogs,
} from "@shared/logs/logRoute";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Route, Routes, useLocation, useParams } from "react-router-dom";
import { describe, expect, it } from "vitest";
import { renderWithQuery } from "@/test/render";

// The route kit pins its own contract here: the path shape, the
// percent-encoding round trip through the router, the kind guard, and the
// typed navigation state. Feature logs suites keep only their wiring smokes.

describe("logsPath", () => {
	it("builds the machine-scoped logs path in the route pattern's own shape", () => {
		expect(logsPath("m-1", "container", "c-1")).toBe(
			"/machines/m-1/logs/container/c-1",
		);
		expect(logsPath("m-1", "quadlet", "web.service")).toBe(
			"/machines/m-1/logs/quadlet/web.service",
		);
	});

	it("percent-encodes machineId and targetId so ids with reserved characters still match the route", () => {
		expect(logsPath("m/1", "container", "a b/c")).toBe(
			"/machines/m%2F1/logs/container/a%20b%2Fc",
		);
	});
});

describe("isLogKind", () => {
	it("accepts exactly the LogKind members", () => {
		expect(isLogKind("container")).toBe(true);
		expect(isLogKind("quadlet")).toBe(true);
		expect(isLogKind("pod")).toBe(false);
		expect(isLogKind(undefined)).toBe(false);
	});
});

// The router decodes params on the far side, so encoding in logsPath must
// round trip: the route still matches for ids containing reserved characters
// and the params come back as the originals. The navigation state arrives
// typed, carrying the friendly display name.
function OpenProbe({ machineId, kind }: { machineId: string; kind: LogKind }) {
	const open = useOpenLogs(machineId, kind);
	return (
		<button
			onClick={() => open({ targetId: "a b/c", name: "Display Name" })}
			type="button"
		>
			open
		</button>
	);
}

function RouteProbe() {
	const { machineId, kind, targetId } = useParams<{
		machineId: string;
		kind: string;
		targetId: string;
	}>();
	const location = useLocation();
	const state = location.state as LogsPageState | null;
	return (
		<div>
			<span data-testid="probe-machine">{machineId}</span>
			<span data-testid="probe-kind">{kind}</span>
			<span data-testid="probe-target">{targetId}</span>
			<span data-testid="probe-name">{state?.name}</span>
		</div>
	);
}

describe("useOpenLogs", () => {
	it("navigates the encoded path; the mounted pattern matches it and decodes params and state", async () => {
		renderWithQuery(
			<Routes>
				<Route
					path="*"
					element={<OpenProbe machineId="m/1" kind="container" />}
				/>
				<Route path={LOG_ROUTE_PATTERN} element={<RouteProbe />} />
			</Routes>,
			{ router: { initialEntries: ["/somewhere"] } },
		);

		const user = userEvent.setup();
		await user.click(screen.getByRole("button", { name: "open" }));

		expect(screen.getByTestId("probe-machine")).toHaveTextContent("m/1");
		expect(screen.getByTestId("probe-kind")).toHaveTextContent("container");
		expect(screen.getByTestId("probe-target")).toHaveTextContent("a b/c");
		expect(screen.getByTestId("probe-name")).toHaveTextContent("Display Name");
	});
});
