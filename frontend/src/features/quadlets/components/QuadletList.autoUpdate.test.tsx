import { QuadletList } from "@features/quadlets/components/QuadletList";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import { machineFixture, quadletFixture } from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";
import { liveStream, streamResponse } from "@/test/stream";

const web = quadletFixture();

type Permission = "admin" | "write" | "read";

function renderQuadletList(permission: Permission = "admin") {
	server.use(
		http.get("*/api/machines/m-1/quadlets", () => HttpResponse.json([web])),
	);
	renderWithQuery(<QuadletList machine={machineFixture({ permission })} />, {
		router: true,
	});
}

// Auto-update events mirror the backend's AutoUpdateEvent shapes.
type AutoUpdateEvent = Record<string, unknown> & {
	type: string;
	operationId: string;
};

function logEvent(line: string): AutoUpdateEvent {
	return {
		operationId: "op-1",
		type: "log",
		stream: "stdout",
		line,
		machineId: "m-1",
	};
}

function completeEvent(
	containers: Record<string, unknown>[],
	overall = "completed",
): AutoUpdateEvent {
	return {
		operationId: "op-1",
		type: "complete",
		machineId: "m-1",
		report: { containers, overall, exitCode: 0 },
	};
}

function errorEvent(code: string, message: string): AutoUpdateEvent {
	return {
		operationId: "op-1",
		type: "error",
		machineId: "m-1",
		error: { code, message },
	};
}

const item = (overrides: Record<string, unknown>) => ({
	containerName: "web",
	imageName: "nginx:1.27",
	updated: "updated",
	...overrides,
});

// NDJSON framing for the auto-update stream: one JSON event per line,
// newline-terminated.
const frameEvents = (events: AutoUpdateEvent[]) =>
	events.map((event) => `${JSON.stringify(event)}\n`).join("");

let autoUpdateRequests: Request[] = [];

// A gate so tests can hold the stream open and observe live line rendering.
let releaseStream: (() => void) | undefined;
let pendingEvents: AutoUpdateEvent[] = [];

function mockAutoUpdate(events: AutoUpdateEvent[] = [], holdOpen = false) {
	autoUpdateRequests = [];
	pendingEvents = events;
	releaseStream = undefined;
	server.use(
		http.post("*/api/machines/m-1/quadlets/auto-update", ({ request }) => {
			autoUpdateRequests.push(request);
			if (!holdOpen) {
				return new HttpResponse(frameEvents(pendingEvents), {
					status: 200,
					headers: { "Content-Type": "application/json" },
				});
			}
			const encoder = new TextEncoder();
			const stream = new ReadableStream<Uint8Array>({
				async start(controller) {
					for (const event of pendingEvents) {
						await new Promise((resolve) => setTimeout(resolve, 20));
						controller.enqueue(encoder.encode(frameEvents([event])));
					}
					await new Promise<void>((resolve) => {
						releaseStream = resolve;
					});
					controller.close();
				},
			});
			return new HttpResponse(stream, {
				status: 200,
				headers: { "Content-Type": "application/json" },
			});
		}),
	);
}

// Two-run auto-update mock: the first POST (the check's dry run) answers
// immediately with the dry-run events; the second POST (a real run) is
// held until the returned release function fires, so the in-progress state
// is observable mid-run. The request is held, not the body stream: chunks
// enqueued at release time on a held body stream are dropped in this
// environment. The real run's events stream one chunk per event, keeping
// live output observable before the run's report lands.
function mockTwoRunAutoUpdate(
	dryRunEvents: AutoUpdateEvent[],
	realRunEvents: AutoUpdateEvent[],
): () => void {
	autoUpdateRequests = [];
	let releaseRealRun: (() => void) | undefined;
	let runCount = 0;
	server.use(
		http.post(
			"*/api/machines/m-1/quadlets/auto-update",
			async ({ request }) => {
				autoUpdateRequests.push(request);
				runCount += 1;
				if (runCount === 1) {
					return new HttpResponse(frameEvents(dryRunEvents), {
						status: 200,
						headers: { "Content-Type": "application/json" },
					});
				}
				await new Promise<void>((resolve) => {
					releaseRealRun = resolve;
				});
				return streamResponse(
					liveStream(
						realRunEvents.map((event) => frameEvents([event])),
						250,
					),
				);
			},
		),
	);
	return () => releaseRealRun?.();
}

async function clickCheckForUpdates() {
	renderQuadletList();
	const user = userEvent.setup();
	await user.click(await screen.findByTestId("check-updates-button"));
	return user;
}

async function clickUpdateAllThroughConfirm() {
	renderQuadletList();
	const user = userEvent.setup();
	await user.click(await screen.findByTestId("update-all-button"));
	expect(await screen.findByTestId("modal")).toBeInTheDocument();
	expect(autoUpdateRequests).toHaveLength(0);
	await user.click(screen.getByTestId("confirm-auto-update-button"));
	return user;
}

describe("QuadletList auto-update", () => {
	it("swaps the whole toolbar for the read hint, hiding create with auto-update", async () => {
		renderQuadletList("read");

		const hint = await screen.findByTestId("quadlets-permission-hint");
		expect(hint).toHaveAttribute("data-requirement", "write");
		expect(hint).toHaveTextContent(
			"You have read-only access to this machine. Ask an owner or admin for write access to manage its quadlets.",
		);
		expect(
			screen.queryByTestId("create-quadlet-button"),
		).not.toBeInTheDocument();
		expect(
			screen.queryByTestId("check-updates-button"),
		).not.toBeInTheDocument();
		expect(screen.queryByTestId("update-all-button")).not.toBeInTheDocument();
	});

	it("shows both toolbar buttons to users with write access", async () => {
		renderQuadletList("write");

		expect(
			await screen.findByTestId("check-updates-button"),
		).toBeInTheDocument();
		expect(screen.getByTestId("update-all-button")).toBeInTheDocument();
	});

	it('runs "Check for updates" directly as a dry run without confirmation', async () => {
		mockAutoUpdate([completeEvent([item({ updated: "pending" })], "dry-run")]);
		await clickCheckForUpdates();

		await waitFor(() => expect(autoUpdateRequests).toHaveLength(1));
		expect(new URL(autoUpdateRequests[0].url).searchParams.get("dryRun")).toBe(
			"true",
		);
		expect(await screen.findByTestId("modal")).toBeInTheDocument();
	});

	it('requires confirmation before "Update all" fires, then runs without dryRun', async () => {
		mockAutoUpdate([completeEvent([])]);
		await clickUpdateAllThroughConfirm();

		await waitFor(() => expect(autoUpdateRequests).toHaveLength(1));
		expect(new URL(autoUpdateRequests[0].url).searchParams.has("dryRun")).toBe(
			false,
		);
	});

	it("renders live log lines while the stream is open", async () => {
		mockAutoUpdate([logEvent("Checking web.service")], true);
		await clickCheckForUpdates();

		expect(await screen.findByTestId("modal")).toBeInTheDocument();
		expect(await screen.findByText("Checking web.service")).toBeInTheDocument();
		expect(screen.getByTestId("auto-update-in-progress")).toBeInTheDocument();

		releaseStream?.();
		await waitFor(() =>
			expect(
				screen.queryByTestId("auto-update-in-progress"),
			).not.toBeInTheDocument(),
		);
	});

	it("renders the dry-run report: pending containers with images are distinct from updated", async () => {
		mockAutoUpdate([
			completeEvent(
				[
					item({ containerName: "web", updated: "pending" }),
					item({
						containerName: "db",
						imageName: "postgres:16",
						updated: "up-to-date",
					}),
				],
				"dry-run",
			),
		]);
		await clickCheckForUpdates();

		expect(await screen.findByTestId("modal")).toBeInTheDocument();
		expect(screen.getByTestId("auto-update-dry-run-badge")).toBeInTheDocument();
		expect(
			screen.getByTestId("auto-update-item-pending-web"),
		).toBeInTheDocument();
		expect(
			screen.getByTestId("auto-update-item-pending-web"),
		).toHaveTextContent("nginx:1.27");
		expect(
			screen.getByTestId("auto-update-item-up-to-date-db"),
		).toBeInTheDocument();
	});

	it("renders per-container failures with their errors in the report", async () => {
		mockAutoUpdate([
			completeEvent(
				[
					item({ containerName: "web", updated: "updated" }),
					item({
						containerName: "cache",
						imageName: "redis:7",
						updated: "failed",
						error: "health check timed out",
					}),
				],
				"completed-with-failures",
			),
		]);
		await clickUpdateAllThroughConfirm();

		expect(
			await screen.findByTestId("auto-update-item-updated-web"),
		).toBeInTheDocument();
		const failed = screen.getByTestId("auto-update-item-failed-cache");
		expect(failed).toHaveTextContent("redis:7");
		expect(failed).toHaveTextContent("health check timed out");
	});

	it("surfaces a terminal error event as a connection error, not per-container failures", async () => {
		mockAutoUpdate([errorEvent("SSH_FAILED", "Machine unreachable")]);
		await clickCheckForUpdates();

		const alert = await screen.findByRole("alert");
		expect(alert).toHaveTextContent("SSH_FAILED");
		expect(alert).toHaveTextContent("Machine unreachable");
		expect(
			screen.queryByText(/failed/i, {
				selector: '[data-testid^="auto-update-item"]',
			}),
		).toBeNull();
	});

	it("keeps the footer Update button disabled while the check is running", async () => {
		mockAutoUpdate([logEvent("Checking web.service")], true);
		await clickCheckForUpdates();

		expect(await screen.findByTestId("modal")).toBeInTheDocument();
		expect(await screen.findByText("Checking web.service")).toBeInTheDocument();
		expect(screen.getByTestId("auto-update-in-progress")).toBeInTheDocument();
		const updateButton = screen.getByTestId("auto-update-button");
		expect(updateButton).toBeDisabled();
		expect(updateButton).toHaveTextContent("Update (0)");

		releaseStream?.();
		await waitFor(() =>
			expect(
				screen.queryByTestId("auto-update-in-progress"),
			).not.toBeInTheDocument(),
		);
	});

	it("keeps the footer Update button disabled on a connection error", async () => {
		mockAutoUpdate([errorEvent("SSH_FAILED", "Machine unreachable")]);
		await clickCheckForUpdates();

		expect(await screen.findByRole("alert")).toBeInTheDocument();
		const updateButton = screen.getByTestId("auto-update-button");
		expect(updateButton).toBeDisabled();
		expect(updateButton).toHaveTextContent("Update (0)");
	});

	it("keeps the footer Update button disabled on an unparsable report", async () => {
		mockAutoUpdate([completeEvent([], "unparsable-report")]);
		await clickCheckForUpdates();

		expect(
			await screen.findByText(
				"No auto-update containers found on this machine.",
			),
		).toBeInTheDocument();
		const updateButton = screen.getByTestId("auto-update-button");
		expect(updateButton).toBeDisabled();
		expect(updateButton).toHaveTextContent("Update (0)");
	});

	it("enables the footer Update button with the pending count once the check completes", async () => {
		mockAutoUpdate([
			completeEvent(
				[
					item({ containerName: "web", updated: "pending" }),
					item({
						containerName: "api",
						imageName: "registry/api:2",
						updated: "pending",
					}),
					item({
						containerName: "db",
						imageName: "postgres:16",
						updated: "up-to-date",
					}),
				],
				"dry-run",
			),
		]);
		await clickCheckForUpdates();

		expect(
			await screen.findByTestId("auto-update-item-pending-api"),
		).toBeInTheDocument();
		const updateButton = screen.getByTestId("auto-update-button");
		expect(updateButton).toBeEnabled();
		expect(updateButton).toHaveTextContent("Update (2)");
	});

	it("keeps the footer Update button rendered but disabled at zero pending containers", async () => {
		mockAutoUpdate([
			completeEvent([item({ updated: "up-to-date" })], "dry-run"),
		]);
		await clickCheckForUpdates();

		expect(
			await screen.findByTestId("auto-update-item-up-to-date-web"),
		).toBeInTheDocument();
		const updateButton = screen.getByTestId("auto-update-button");
		expect(updateButton).toBeDisabled();
		expect(updateButton).toHaveTextContent("Update (0)");
	});

	it("renders an inline Update button on every pending report line", async () => {
		mockAutoUpdate([
			completeEvent(
				[
					item({ updated: "pending", unit: "web.service" }),
					item({
						containerName: "api",
						imageName: "registry/api:2",
						updated: "pending",
						unit: "api.service",
					}),
					item({
						containerName: "db",
						imageName: "postgres:16",
						updated: "up-to-date",
					}),
				],
				"dry-run",
			),
		]);
		await clickCheckForUpdates();

		expect(
			await screen.findByTestId("auto-update-item-update-web"),
		).toBeEnabled();
		expect(screen.getByTestId("auto-update-item-update-api")).toBeEnabled();
		// Only pending lines get the button.
		expect(
			screen.queryByTestId("auto-update-item-update-db"),
		).not.toBeInTheDocument();
		expect(screen.getByTestId("auto-update-button")).toHaveTextContent(
			"Update (2)",
		);
	});

	it("renders no inline Update button for a pending line without a unit", async () => {
		mockAutoUpdate([completeEvent([item({ updated: "pending" })], "dry-run")]);
		await clickCheckForUpdates();

		expect(
			await screen.findByTestId("auto-update-item-pending-web"),
		).toBeInTheDocument();
		expect(
			screen.queryByTestId("auto-update-item-update-web"),
		).not.toBeInTheDocument();
	});

	it("runs a real unfiltered update from the check window: applying badge, live output, then the run's report", async () => {
		const releaseRealRun = mockTwoRunAutoUpdate(
			[completeEvent([item({ updated: "pending" })], "dry-run")],
			[
				logEvent("Updating web.service"),
				completeEvent([item({ updated: "updated" })], "completed"),
			],
		);
		const user = await clickCheckForUpdates();

		expect(
			await screen.findByTestId("auto-update-item-pending-web"),
		).toBeInTheDocument();
		const updateButton = screen.getByTestId("auto-update-button");
		expect(updateButton).toHaveTextContent("Update (1)");
		expect(updateButton).toBeEnabled();
		await user.click(updateButton);

		await waitFor(() => expect(autoUpdateRequests).toHaveLength(2));
		expect(new URL(autoUpdateRequests[1].url).searchParams.has("dryRun")).toBe(
			false,
		);
		// The same window narrates the real run: the applying badge, live
		// output, and the button disabled while the run is in progress.
		expect(
			await screen.findByTestId("auto-update-real-run-badge"),
		).toBeInTheDocument();
		expect(screen.queryByTestId("auto-update-dry-run-badge")).toBeNull();
		expect(screen.getByTestId("auto-update-in-progress")).toBeInTheDocument();
		expect(screen.getByTestId("auto-update-button")).toBeDisabled();

		releaseRealRun();
		expect(
			await screen.findByTestId("auto-update-item-updated-web"),
		).toBeInTheDocument();
		expect(screen.getByText("Updating web.service")).toBeInTheDocument();
		// The completed run's report is the end state: no pending containers,
		// so the button stays rendered but disabled.
		expect(screen.getByTestId("auto-update-button")).toBeDisabled();
		expect(screen.getByTestId("auto-update-button")).toHaveTextContent(
			"Update (0)",
		);
	});

	it("runs a real auto-update filtered to the line's unit from the inline button", async () => {
		const releaseFilteredRun = mockTwoRunAutoUpdate(
			[
				completeEvent(
					[
						item({ updated: "pending", unit: "web.service" }),
						item({
							containerName: "api",
							imageName: "registry/api:2",
							updated: "pending",
							unit: "api.service",
						}),
					],
					"dry-run",
				),
			],
			[
				logEvent("Updating web.service"),
				completeEvent(
					[item({ updated: "updated", unit: "web.service" })],
					"completed",
				),
			],
		);
		const user = await clickCheckForUpdates();

		const webUpdate = await screen.findByTestId("auto-update-item-update-web");
		expect(webUpdate).toBeEnabled();
		expect(screen.getByTestId("auto-update-item-update-api")).toBeEnabled();
		await user.click(webUpdate);

		// The second POST is a real run filtered to the line's unit.
		await waitFor(() => expect(autoUpdateRequests).toHaveLength(2));
		const filteredUrl = new URL(autoUpdateRequests[1].url);
		expect(filteredUrl.searchParams.get("filter")).toBe("web.service");
		expect(filteredUrl.searchParams.has("dryRun")).toBe(false);

		// The same window narrates the filtered run: the title names the unit,
		// the applying badge shows, and the update actions are disabled.
		const modal = screen.getByTestId("modal");
		expect(modal).toHaveTextContent("Update web.service");
		expect(within(modal).queryByText("Check for updates")).toBeNull();
		expect(
			await screen.findByTestId("auto-update-real-run-badge"),
		).toBeInTheDocument();
		expect(screen.queryByTestId("auto-update-dry-run-badge")).toBeNull();
		expect(screen.getByTestId("auto-update-in-progress")).toBeInTheDocument();
		expect(screen.getByTestId("auto-update-button")).toBeDisabled();
		expect(screen.queryByTestId("auto-update-item-update-web")).toBeNull();
		expect(screen.queryByTestId("auto-update-item-update-api")).toBeNull();

		releaseFilteredRun();
		// Live output shows while the run is in progress, then the run's
		// report is the end state.
		expect(await screen.findByText("Updating web.service")).toBeInTheDocument();
		expect(screen.getByTestId("auto-update-in-progress")).toBeInTheDocument();
		expect(
			await screen.findByTestId("auto-update-item-updated-web"),
		).toBeInTheDocument();
		expect(
			screen.queryByTestId("auto-update-in-progress"),
		).not.toBeInTheDocument();
		// The title still names the unit (the filtered run is the last run),
		// and the completed report has no pending containers, so the footer
		// button stays rendered but disabled.
		expect(screen.getByTestId("modal")).toHaveTextContent("Update web.service");
		expect(screen.getByTestId("auto-update-button")).toBeDisabled();
		expect(screen.getByTestId("auto-update-button")).toHaveTextContent(
			"Update (0)",
		);
	});

	it("keeps the update actions disabled while any run is in progress", async () => {
		const releaseRealRun = mockTwoRunAutoUpdate(
			[
				completeEvent(
					[
						item({ updated: "pending", unit: "web.service" }),
						item({
							containerName: "api",
							imageName: "registry/api:2",
							updated: "pending",
							unit: "api.service",
						}),
					],
					"dry-run",
				),
			],
			[
				completeEvent(
					[
						item({ updated: "updated", unit: "web.service" }),
						item({
							containerName: "api",
							imageName: "registry/api:2",
							updated: "updated",
							unit: "api.service",
						}),
					],
					"completed",
				),
			],
		);
		const user = await clickCheckForUpdates();

		expect(
			await screen.findByTestId("auto-update-item-update-web"),
		).toBeEnabled();
		expect(screen.getByTestId("auto-update-item-update-api")).toBeEnabled();

		// Any run — here the footer's unfiltered one — takes the window to the
		// applying state: the footer Update button is disabled and the pending
		// report (with its per-service buttons) is replaced by the live output.
		await user.click(screen.getByTestId("auto-update-button"));
		await waitFor(() => expect(autoUpdateRequests).toHaveLength(2));
		expect(screen.getByTestId("auto-update-in-progress")).toBeInTheDocument();
		expect(screen.getByTestId("auto-update-button")).toBeDisabled();
		expect(screen.queryByTestId("auto-update-item-update-web")).toBeNull();
		expect(screen.queryByTestId("auto-update-item-update-api")).toBeNull();

		releaseRealRun();
		expect(
			await screen.findByTestId("auto-update-item-updated-web"),
		).toBeInTheDocument();
		expect(
			screen.queryByTestId("auto-update-in-progress"),
		).not.toBeInTheDocument();
	});

	it("refetches the quadlet list when the filtered run completes", async () => {
		let listRequests = 0;
		server.use(
			http.get("*/api/machines/m-1/quadlets", () => {
				listRequests += 1;
				return HttpResponse.json([web]);
			}),
		);
		let runCount = 0;
		server.use(
			http.post("*/api/machines/m-1/quadlets/auto-update", () => {
				runCount += 1;
				const events =
					runCount === 1
						? [
								completeEvent(
									[item({ updated: "pending", unit: "web.service" })],
									"dry-run",
								),
							]
						: [
								completeEvent(
									[item({ updated: "updated", unit: "web.service" })],
									"completed",
								),
							];
				return new HttpResponse(frameEvents(events), {
					status: 200,
					headers: { "Content-Type": "application/json" },
				});
			}),
		);
		renderWithQuery(<QuadletList machine={machineFixture()} />, {
			router: true,
		});
		const user = userEvent.setup();

		await user.click(await screen.findByTestId("check-updates-button"));
		await user.click(await screen.findByTestId("auto-update-item-update-web"));
		expect(
			await screen.findByTestId("auto-update-item-updated-web"),
		).toBeInTheDocument();
		// The real run invalidates the quadlet query: the list refetches.
		await waitFor(() => expect(listRequests).toBeGreaterThan(1));
	});

	it("never renders either Update button kind when the modal was opened as a real run from the toolbar", async () => {
		mockAutoUpdate([
			completeEvent(
				[item({ updated: "pending", unit: "web.service" })],
				"dry-run",
			),
		]);
		await clickUpdateAllThroughConfirm();

		expect(
			await screen.findByTestId("auto-update-item-pending-web"),
		).toBeInTheDocument();
		expect(screen.queryByTestId("auto-update-button")).not.toBeInTheDocument();
		expect(
			screen.queryByTestId("auto-update-item-update-web"),
		).not.toBeInTheDocument();
	});
});
