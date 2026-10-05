import { QuadletList } from "@features/quadlets/components/QuadletList";
import type { QuadletSummary } from "@features/quadlets/services/quadletsApi";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { beforeEach, describe, expect, it } from "vitest";
import { errorResponse } from "@/test/errors";
import { machineFixture, quadletFixture } from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

const web = quadletFixture({ state: "stopped" });

function quadletsHandler(quadlets: () => QuadletSummary[]) {
	return http.get("*/api/machines/m-1/quadlets", () =>
		HttpResponse.json(quadlets()),
	);
}

let lifecycleRequests: URL[] = [];

function resetRequests() {
	lifecycleRequests = [];
}

// The lifecycle endpoints answer 200 with {id, state, unit}; the response
// body itself is not used by the UI (the list refetch tells the story), so
// the handler only records what it saw. A delay keeps a request in flight
// for the pending-state tests.
function captureLifecycle(action: "start" | "stop" | "restart", delayMs = 0) {
	return http.post(
		`*/api/machines/m-1/quadlets/:quadletId/${action}`,
		async ({ request }) => {
			if (delayMs > 0)
				await new Promise((resolve) => setTimeout(resolve, delayMs));
			lifecycleRequests.push(new URL(request.url));
			return HttpResponse.json({
				id: "web",
				state: "running",
				unit: "web.service",
			});
		},
	);
}

function renderQuadletList() {
	renderWithQuery(<QuadletList machine={machineFixture()} />, {
		router: true,
	});
}

async function openOptionsModal(action: "stop" | "restart", name = "web") {
	const user = userEvent.setup();
	renderQuadletList();
	const label = action === "stop" ? "Stop" : "Restart";
	await user.click(
		await screen.findByRole("button", { name: `${label} ${name}` }),
	);
	expect(await screen.findByTestId("modal")).toHaveTextContent(name);
	return user;
}

describe("QuadletList lifecycle", () => {
	beforeEach(resetRequests);

	it("shows Start only for non-running quadlets and Stop only for running ones", async () => {
		server.use(
			quadletsHandler(() => [
				web,
				quadletFixture({ id: "db", name: "db", unit: "db.service" }),
			]),
		);
		renderQuadletList();

		const rows = await screen.findAllByTestId("quadlet-row");
		const stoppedRow = rows.find((row) => row.textContent?.includes("web"));
		const runningRow = rows.find((row) => row.textContent?.includes("db"));

		expect(stoppedRow).not.toBeNull();
		expect(runningRow).not.toBeNull();
		expect(
			within(stoppedRow!).getByTestId("start-quadlet-button"),
		).toBeInTheDocument();
		expect(
			within(stoppedRow!).queryByTestId("stop-quadlet-button"),
		).not.toBeInTheDocument();
		expect(
			within(runningRow!).getByTestId("stop-quadlet-button"),
		).toBeInTheDocument();
		expect(
			within(runningRow!).queryByTestId("start-quadlet-button"),
		).not.toBeInTheDocument();
	});

	it("posts to the start endpoint with no options on Start click", async () => {
		server.use(
			quadletsHandler(() => [web]),
			captureLifecycle("start"),
		);
		renderQuadletList();

		await userEvent.click(await screen.findByTestId("start-quadlet-button"));

		await waitFor(() => expect(lifecycleRequests).toHaveLength(1));
		expect(lifecycleRequests[0].pathname).toBe(
			"/api/machines/m-1/quadlets/web/start",
		);
		expect(lifecycleRequests[0].search).toBe("");
	});

	it("opens the stop options modal naming the quadlet without sending a request", async () => {
		server.use(
			quadletsHandler(() => [quadletFixture()]),
			captureLifecycle("stop"),
		);
		await openOptionsModal("stop");

		// No force checkbox — the param left the contract (issue 205).
		expect(
			screen.queryByRole("checkbox", { name: /force/i }),
		).not.toBeInTheDocument();
		expect(screen.getByRole("spinbutton", { name: /timeout/i })).toHaveValue(
			null,
		);
		expect(lifecycleRequests).toHaveLength(0);
	});

	it("confirming stop with the defaults sends no query parameters", async () => {
		server.use(
			quadletsHandler(() => [quadletFixture()]),
			captureLifecycle("stop"),
		);
		const user = await openOptionsModal("stop");

		await user.click(screen.getByTestId("confirm-lifecycle-button"));

		await waitFor(() => expect(lifecycleRequests).toHaveLength(1));
		expect(lifecycleRequests[0].pathname).toBe(
			"/api/machines/m-1/quadlets/web/stop",
		);
		expect(lifecycleRequests[0].search).toBe("");
	});

	it("maps the chosen timeout onto the timeout query parameter", async () => {
		server.use(
			quadletsHandler(() => [quadletFixture()]),
			captureLifecycle("stop"),
		);
		const user = await openOptionsModal("stop");

		await user.type(screen.getByRole("spinbutton", { name: /timeout/i }), "30");
		await user.click(screen.getByTestId("confirm-lifecycle-button"));

		await waitFor(() => expect(lifecycleRequests).toHaveLength(1));
		expect(lifecycleRequests[0].searchParams.has("force")).toBe(false);
		expect(lifecycleRequests[0].searchParams.get("timeout")).toBe("30");
	});

	it("restarts through the options modal onto the restart endpoint", async () => {
		server.use(
			quadletsHandler(() => [quadletFixture()]),
			captureLifecycle("restart"),
		);
		const user = await openOptionsModal("restart");

		expect(screen.getByTestId("modal")).toHaveTextContent("Restart");
		await user.type(screen.getByRole("spinbutton", { name: /timeout/i }), "45");
		await user.click(screen.getByTestId("confirm-lifecycle-button"));

		await waitFor(() => expect(lifecycleRequests).toHaveLength(1));
		expect(lifecycleRequests[0].pathname).toBe(
			"/api/machines/m-1/quadlets/web/restart",
		);
		expect(lifecycleRequests[0].searchParams.get("timeout")).toBe("45");
	});

	it("confirming restart with defaults sends no query parameters", async () => {
		server.use(
			quadletsHandler(() => [quadletFixture()]),
			captureLifecycle("restart"),
		);
		const user = await openOptionsModal("restart");

		await user.click(screen.getByTestId("confirm-lifecycle-button"));

		await waitFor(() => expect(lifecycleRequests).toHaveLength(1));
		expect(lifecycleRequests[0].pathname).toBe(
			"/api/machines/m-1/quadlets/web/restart",
		);
		expect(lifecycleRequests[0].search).toBe("");
	});

	it("disables the row lifecycle actions while a start runs and resists double clicks", async () => {
		server.use(
			quadletsHandler(() => [
				web,
				quadletFixture({ id: "db", name: "db", unit: "db.service" }),
			]),
			captureLifecycle("start", 500),
		);
		const user = userEvent.setup();
		renderQuadletList();

		const startButtons = await screen.findAllByTestId("start-quadlet-button");
		expect(startButtons).toHaveLength(1);
		await user.click(startButtons[0]);
		await user.click(startButtons[0]);

		await waitFor(() => {
			expect(startButtons[0]).toBeDisabled();
			expect(startButtons[0]).toHaveAttribute("data-loading");
		});
		expect(screen.getByTestId("stop-quadlet-button")).toBeEnabled();

		await waitFor(() => expect(lifecycleRequests).toHaveLength(1));
	});

	it("refetches the list on success so the state badge shows the new state", async () => {
		let stopped = false;
		server.use(
			quadletsHandler(() => [
				quadletFixture({ state: stopped ? "stopped" : "running" }),
			]),
			http.post("*/api/machines/m-1/quadlets/:quadletId/stop", () => {
				stopped = true;
				return HttpResponse.json({
					id: "web",
					state: "stopped",
					unit: "web.service",
				});
			}),
		);
		const user = userEvent.setup();
		renderQuadletList();

		expect(await screen.findByTestId("state-badge")).toHaveTextContent(
			"running",
		);
		await user.click(await screen.findByTestId("stop-quadlet-button"));
		await user.click(await screen.findByTestId("confirm-lifecycle-button"));

		await waitFor(() =>
			expect(screen.getByTestId("state-badge")).toHaveTextContent("stopped"),
		);
	});

	it("shows the error envelope under the row when starting fails", async () => {
		server.use(
			quadletsHandler(() => [quadletFixture({ state: "stopped" })]),
			http.post("*/api/machines/m-1/quadlets/:quadletId/start", () =>
				errorResponse(409, "QUADLET_START_FAILED", "Unit is masked", "req-61"),
			),
		);
		const user = userEvent.setup();
		renderQuadletList();

		const startButton = await screen.findByTestId("start-quadlet-button");
		await user.click(startButton);

		expect(await screen.findByRole("alert")).toBeInTheDocument();
		expect(screen.getByText("QUADLET_START_FAILED")).toBeInTheDocument();
		expect(screen.getByText("Unit is masked")).toBeInTheDocument();
		await waitFor(() => expect(startButton).toBeEnabled());
	});

	it("keeps the options modal open with the error envelope when stopping fails", async () => {
		server.use(
			quadletsHandler(() => [quadletFixture()]),
			http.post("*/api/machines/m-1/quadlets/:quadletId/stop", () =>
				errorResponse(404, "QUADLET_NOT_FOUND", "Quadlet not found", "req-62"),
			),
		);
		const user = await openOptionsModal("stop");

		await user.click(screen.getByTestId("confirm-lifecycle-button"));

		// The envelope renders inside the still-open modal, in the module's
		// error region above the body; stay-open and button state are the
		// module suite's pins.
		const modal = screen.getByTestId("modal");
		expect(
			await within(modal).findByTestId("mutation-dialog-error"),
		).toBeInTheDocument();
		expect(within(modal).getByText("QUADLET_NOT_FOUND")).toBeInTheDocument();
		expect(within(modal).getByText("Quadlet not found")).toBeInTheDocument();
	});
});
