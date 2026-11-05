import { useQuadletAutoUpdate } from "@features/quadlets/hooks/useQuadletAutoUpdate";
import {
	containersQueryKey,
	imagesQueryKey,
	quadletsQueryKey,
} from "@shared/queries/queryKeys";
import { act, waitFor } from "@testing-library/react";
import { type DefaultBodyType, type HttpResponse, http } from "msw";
import { afterEach, describe, expect, it, vi } from "vitest";
import { renderHookWithQuery } from "@/test/render";
import { server } from "@/test/server";
import { ndjsonResponse } from "@/test/stream";

type AutoUpdateResponse = HttpResponse<DefaultBodyType>;
type AutoUpdateHandler = (
	request: Request,
) => AutoUpdateResponse | Promise<AutoUpdateResponse>;

// The auto-update stream's event shapes mirror the backend's
// AutoUpdateEvent (backend/internal/domain/quadlets/auto_update.go).
type AutoUpdateEvent = Record<string, unknown> & {
	type: string;
	operationId: string;
};

function logEvent(line: string, stream = "stdout"): AutoUpdateEvent {
	return { operationId: "op-1", type: "log", stream, line, hostId: "m-1" };
}

function report(overrides: Record<string, unknown> = {}): AutoUpdateEvent {
	return {
		operationId: "op-1",
		type: "complete",
		hostId: "m-1",
		report: {
			containers: [],
			overall: "completed",
			exitCode: 0,
			...overrides,
		},
	};
}

function errorEvent(code: string, message: string): AutoUpdateEvent {
	return {
		operationId: "op-1",
		type: "error",
		hostId: "m-1",
		error: { code, message },
	};
}

function planEvent(entries: Record<string, unknown>[]): AutoUpdateEvent {
	return { operationId: "op-1", type: "plan", hostId: "m-1", plan: entries };
}

function pullingEvent(unit: string, image?: string): AutoUpdateEvent {
	return {
		operationId: "op-1",
		type: "pulling",
		hostId: "m-1",
		unit,
		...(image !== undefined ? { image } : {}),
	};
}

const autoUpdateUrl = "*/api/hosts/:hostId/quadlets/auto-update";

let requests: Request[] = [];

function mockAutoUpdate(handler: AutoUpdateHandler) {
	requests = [];
	server.use(
		http.post(autoUpdateUrl, ({ request }) => {
			requests.push(request);
			return handler(request);
		}),
	);
}

// The backend events use their own "log" type, so the generic NDJSON test
// builder can't type-check them; they travel as raw framed lines instead.
function streamResponse(events: AutoUpdateEvent[]) {
	return ndjsonResponse(
		[],
		events.map((event) => `${JSON.stringify(event)}\n`),
	);
}

function renderAutoUpdateHook() {
	return renderHookWithQuery(() => useQuadletAutoUpdate("m-1"));
}

afterEach(() => {
	vi.restoreAllMocks();
});

describe("useQuadletAutoUpdate", () => {
	it("POSTs to the auto-update endpoint with the requested dry-run flag", async () => {
		mockAutoUpdate(() => streamResponse([report()]));
		const { result } = renderAutoUpdateHook();

		await act(async () => {
			result.current.run(true);
		});

		expect(requests).toHaveLength(1);
		expect(requests[0].method).toBe("POST");
		expect(new URL(requests[0].url).pathname).toBe(
			"/api/hosts/m-1/quadlets/auto-update",
		);
		expect(new URL(requests[0].url).searchParams.get("dryRun")).toBe("true");
		expect(new URL(requests[0].url).searchParams.has("filter")).toBe(false);
		await waitFor(() => expect(result.current.isRunning).toBe(false));
		expect(result.current.connectionError).toBeNull();
	});

	it("omits the dry-run flag for a real run", async () => {
		mockAutoUpdate(() => streamResponse([report()]));
		const { result } = renderAutoUpdateHook();

		await act(async () => {
			result.current.run(false);
		});

		expect(new URL(requests[0].url).searchParams.has("dryRun")).toBe(false);
		expect(new URL(requests[0].url).searchParams.has("filter")).toBe(false);
		await waitFor(() => expect(result.current.isRunning).toBe(false));
	});

	it("POSTs with the filter argument for a filtered run and exposes the filter state", async () => {
		mockAutoUpdate(() => streamResponse([report()]));
		const { result } = renderAutoUpdateHook();

		await act(async () => {
			result.current.run(false, "web.service");
		});

		expect(requests).toHaveLength(1);
		expect(new URL(requests[0].url).searchParams.get("filter")).toBe(
			"web.service",
		);
		expect(new URL(requests[0].url).searchParams.has("dryRun")).toBe(false);
		await waitFor(() => expect(result.current.isRunning).toBe(false));
		expect(result.current.filter).toBe("web.service");
	});

	it("clears the filter state when a later run is unfiltered", async () => {
		mockAutoUpdate(() => streamResponse([report()]));
		const { result } = renderAutoUpdateHook();

		await act(async () => {
			result.current.run(false, "web.service");
		});
		await waitFor(() => expect(result.current.isRunning).toBe(false));
		expect(result.current.filter).toBe("web.service");

		await act(async () => {
			result.current.run(true);
		});

		expect(requests).toHaveLength(2);
		expect(new URL(requests[1].url).searchParams.has("filter")).toBe(false);
		expect(new URL(requests[1].url).searchParams.get("dryRun")).toBe("true");
		await waitFor(() => expect(result.current.isRunning).toBe(false));
		expect(result.current.filter).toBeNull();
	});

	it("collects log lines as they arrive and exposes the terminal report", async () => {
		mockAutoUpdate(() =>
			streamResponse([
				logEvent("Loading registries"),
				logEvent("Checking web.service", "stderr"),
				report({
					containers: [
						{
							containerName: "web",
							imageName: "nginx:1.27",
							updated: "updated",
						},
					],
					overall: "completed",
					exitCode: 0,
				}),
			]),
		);
		const { result } = renderAutoUpdateHook();

		await act(async () => {
			result.current.run(false);
		});

		await waitFor(() => expect(result.current.isRunning).toBe(false));
		expect(result.current.lines).toEqual([
			expect.objectContaining({ line: "Loading registries", stream: "stdout" }),
			expect.objectContaining({
				line: "Checking web.service",
				stream: "stderr",
			}),
		]);
		expect(result.current.report?.containers).toEqual([
			{ containerName: "web", imageName: "nginx:1.27", updated: "updated" },
		]);
		expect(result.current.report?.overall).toBe("completed");
		expect(result.current.connectionError).toBeNull();
	});

	it("invalidates quadlet, container, and image queries when a real run completes", async () => {
		mockAutoUpdate(() => streamResponse([report()]));
		const { result, queryClient } = renderAutoUpdateHook();
		const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

		await act(async () => {
			result.current.run(false);
		});

		await waitFor(() => expect(result.current.isRunning).toBe(false));
		expect(invalidateSpy).toHaveBeenCalledWith({
			queryKey: quadletsQueryKey("m-1"),
		});
		expect(invalidateSpy).toHaveBeenCalledWith({
			queryKey: containersQueryKey("m-1"),
		});
		expect(invalidateSpy).toHaveBeenCalledWith({
			queryKey: imagesQueryKey("m-1"),
		});
	});

	it("does not invalidate queries when a dry run completes", async () => {
		mockAutoUpdate(() => streamResponse([report()]));
		const { result, queryClient } = renderAutoUpdateHook();
		const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

		await act(async () => {
			result.current.run(true);
		});

		await waitFor(() => expect(result.current.isRunning).toBe(false));
		expect(invalidateSpy).not.toHaveBeenCalled();
	});

	it("normalizes a null container list in the terminal report to an empty array", async () => {
		// The backend serializes an empty or unparsable report's container
		// list as null (a Go nil slice; AutoUpdateReport.Containers has no
		// omitempty). The hook owns the report shape and normalizes it at
		// the event seam, so consumers never dereference null.
		mockAutoUpdate(() =>
			streamResponse([
				report({ containers: null, overall: "unparsable-report" }),
			]),
		);
		const { result } = renderAutoUpdateHook();

		await act(async () => {
			result.current.run(false);
		});

		await waitFor(() => expect(result.current.isRunning).toBe(false));
		expect(result.current.report).toEqual({
			containers: [],
			overall: "unparsable-report",
			exitCode: 0,
		});
	});

	it("seeds the row list from the plan event in plan order", async () => {
		mockAutoUpdate(() =>
			streamResponse([
				planEvent([
					{ unit: "web.service", image: "docker.io/library/web:1" },
					// A degraded plan entry: no image (unreadable file or
					// unparsable Image=) still gets its row.
					{ unit: "db.service" },
				]),
				report(),
			]),
		);
		const { result } = renderAutoUpdateHook();

		await act(async () => {
			result.current.run(false);
		});

		await waitFor(() => expect(result.current.isRunning).toBe(false));
		expect(result.current.rows).toEqual([
			{
				unit: "web.service",
				image: "docker.io/library/web:1",
				state: "waiting",
			},
			{ unit: "db.service", state: "waiting" },
		]);
	});

	it("flips the matching row to pulling and exposes the pulling unit", async () => {
		mockAutoUpdate(() =>
			streamResponse([
				planEvent([
					{ unit: "web.service", image: "docker.io/library/web:1" },
					{ unit: "db.service" },
				]),
				pullingEvent("web.service", "docker.io/library/web:1"),
				report(),
			]),
		);
		const { result } = renderAutoUpdateHook();

		await act(async () => {
			result.current.run(false);
		});

		await waitFor(() => expect(result.current.isRunning).toBe(false));
		expect(result.current.rows).toEqual([
			{
				unit: "web.service",
				image: "docker.io/library/web:1",
				state: "pulling",
			},
			{ unit: "db.service", state: "waiting" },
		]);
		expect(result.current.pullingUnit).toBe("web.service");
	});

	it("appends a pulling row for a unit the plan did not name", async () => {
		mockAutoUpdate(() =>
			streamResponse([
				planEvent([{ unit: "web.service" }]),
				pullingEvent("other.service"),
				report(),
			]),
		);
		const { result } = renderAutoUpdateHook();

		await act(async () => {
			result.current.run(false);
		});

		await waitFor(() => expect(result.current.isRunning).toBe(false));
		expect(result.current.rows).toEqual([
			{ unit: "web.service", state: "waiting" },
			{ unit: "other.service", state: "pulling" },
		]);
		expect(result.current.pullingUnit).toBe("other.service");
	});

	it("resolves rows by unit when the report lands, appending report-only units", async () => {
		mockAutoUpdate(() =>
			streamResponse([
				planEvent([
					{ unit: "web.service", image: "docker.io/library/web:1" },
					{ unit: "db.service" },
				]),
				pullingEvent("web.service"),
				report({
					containers: [
						{
							unit: "web.service",
							containerName: "systemd-web",
							imageName: "docker.io/library/web:2",
							updated: "updated",
						},
						{
							containerName: "cache",
							imageName: "redis:7",
							updated: "failed",
							error: "health check timed out",
						},
					],
					overall: "completed-with-failures",
					exitCode: 1,
				}),
			]),
		);
		const { result } = renderAutoUpdateHook();

		await act(async () => {
			result.current.run(false);
		});

		await waitFor(() => expect(result.current.isRunning).toBe(false));
		expect(result.current.rows).toEqual([
			{
				unit: "web.service",
				image: "docker.io/library/web:1",
				state: "updated",
				item: {
					unit: "web.service",
					containerName: "systemd-web",
					imageName: "docker.io/library/web:2",
					updated: "updated",
				},
			},
			// Planned but never reported: no outcome to show, last state kept.
			{ unit: "db.service", state: "waiting" },
			// In the report but not the plan: appended as a resolved row.
			{
				unit: null,
				image: "redis:7",
				state: "failed",
				item: {
					containerName: "cache",
					imageName: "redis:7",
					updated: "failed",
					error: "health check timed out",
				},
			},
		]);
	});

	it("skips malformed plan and pulling events without breaking the fold", async () => {
		mockAutoUpdate(() =>
			streamResponse([
				// A plan event whose plan is not an array, and a pulling event
				// without a string unit: both must be skipped, not crash the
				// fold or poison the row list.
				{ operationId: "op-1", type: "plan", hostId: "m-1", plan: "nope" },
				{ operationId: "op-1", type: "pulling", hostId: "m-1", unit: 42 },
				planEvent([{ unit: "web.service" }, { unit: 7 }]),
				report(),
			]),
		);
		const { result } = renderAutoUpdateHook();

		await act(async () => {
			result.current.run(false);
		});

		await waitFor(() => expect(result.current.isRunning).toBe(false));
		// The well-formed plan seeds the rows; its malformed entry (unit not
		// a string) is dropped.
		expect(result.current.rows).toEqual([
			{ unit: "web.service", state: "waiting" },
		]);
		expect(result.current.pullingUnit).toBeNull();
	});

	it("skips a report item with an unknown state, keeping the row's last observed state", async () => {
		// The report's outcome vocabulary is the backend's closed set; an
		// item outside it is malformed content and must not fold a
		// nonsense state onto its row — the row keeps what the stream
		// last observed, the way the malformed-event guard skips whole
		// events.
		mockAutoUpdate(() =>
			streamResponse([
				planEvent([{ unit: "web.service", image: "docker.io/library/web:1" }]),
				pullingEvent("web.service"),
				report({
					containers: [
						{
							unit: "web.service",
							containerName: "web",
							imageName: "docker.io/library/web:2",
							updated: "not-a-known-state",
						},
						{
							containerName: "cache",
							imageName: "redis:7",
							updated: "failed",
							error: "health check timed out",
						},
					],
					overall: "completed-with-failures",
					exitCode: 1,
				}),
			]),
		);
		const { result } = renderAutoUpdateHook();

		await act(async () => {
			result.current.run(false);
		});

		await waitFor(() => expect(result.current.isRunning).toBe(false));
		expect(result.current.rows).toEqual([
			// The garbage-state item is skipped: the row keeps its last
			// observed state instead of folding the unknown value.
			{
				unit: "web.service",
				image: "docker.io/library/web:1",
				state: "pulling",
			},
			// A well-formed item in the same report still resolves.
			{
				unit: null,
				image: "redis:7",
				state: "failed",
				item: {
					containerName: "cache",
					imageName: "redis:7",
					updated: "failed",
					error: "health check timed out",
				},
			},
		]);
	});

	it("resets the row list and pulling unit when a new run starts", async () => {
		let releaseSecondRun: (() => void) | undefined;
		requests = [];
		server.use(
			http.post(autoUpdateUrl, async ({ request }) => {
				requests.push(request);
				if (requests.length === 1) {
					return streamResponse([
						planEvent([{ unit: "web.service", image: "img:1" }]),
						pullingEvent("web.service"),
						report(),
					]);
				}
				await new Promise<void>((resolve) => {
					releaseSecondRun = resolve;
				});
				return streamResponse([
					planEvent([{ unit: "api.service", image: "api:2" }]),
					report(),
				]);
			}),
		);
		const { result } = renderAutoUpdateHook();

		await act(async () => {
			result.current.run(false);
		});
		await waitFor(() => expect(result.current.isRunning).toBe(false));
		expect(result.current.rows).toEqual([
			{ unit: "web.service", image: "img:1", state: "pulling" },
		]);
		expect(result.current.pullingUnit).toBe("web.service");

		await act(async () => {
			result.current.run(true);
		});

		// The new run starts from a clean slate: no rows, no pulling unit.
		expect(result.current.rows).toEqual([]);
		expect(result.current.pullingUnit).toBeNull();

		await act(async () => {
			releaseSecondRun?.();
		});
		await waitFor(() => expect(result.current.isRunning).toBe(false));
		expect(result.current.rows).toEqual([
			{ unit: "api.service", image: "api:2", state: "waiting" },
		]);
	});

	it("surfaces a terminal error event as a connection error, not a report", async () => {
		mockAutoUpdate(() =>
			streamResponse([errorEvent("SSH_FAILED", "Host unreachable")]),
		);
		const { result } = renderAutoUpdateHook();

		await act(async () => {
			result.current.run(false);
		});

		await waitFor(() => expect(result.current.isRunning).toBe(false));
		expect(result.current.connectionError).toEqual({
			code: "SSH_FAILED",
			message: "Host unreachable",
		});
		expect(result.current.report).toBeNull();
	});

	it("switches from a completed dry run to a real run with fresh state and invalidation", async () => {
		let releaseRealRun: (() => void) | undefined;
		requests = [];
		server.use(
			http.post(autoUpdateUrl, async ({ request }) => {
				requests.push(request);
				if (requests.length === 1) {
					return streamResponse([
						report({
							containers: [
								{
									containerName: "web",
									imageName: "nginx:1.27",
									updated: "pending",
								},
							],
							overall: "dry-run",
						}),
					]);
				}
				// The real run's response is held back until released, so the
				// post-switch state (reset lines/report, flipped dryRun) is
				// observable mid-run, exactly as the check window's Update
				// button finds it.
				await new Promise<void>((resolve) => {
					releaseRealRun = resolve;
				});
				return streamResponse([
					logEvent("Updating web"),
					report({
						containers: [
							{
								containerName: "web",
								imageName: "nginx:1.28",
								updated: "updated",
							},
						],
						overall: "completed",
					}),
				]);
			}),
		);
		const { result, queryClient } = renderAutoUpdateHook();
		const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

		await act(async () => {
			result.current.run(true);
		});
		await waitFor(() => expect(result.current.isRunning).toBe(false));
		expect(result.current.dryRun).toBe(true);
		expect(result.current.report?.containers).toEqual([
			{ containerName: "web", imageName: "nginx:1.27", updated: "pending" },
		]);
		expect(invalidateSpy).not.toHaveBeenCalled();

		await act(async () => {
			result.current.run(false);
		});

		expect(requests).toHaveLength(2);
		expect(new URL(requests[1].url).searchParams.has("dryRun")).toBe(false);
		expect(result.current.dryRun).toBe(false);
		expect(result.current.lines).toEqual([]);
		expect(result.current.report).toBeNull();
		expect(result.current.isRunning).toBe(true);

		await act(async () => {
			releaseRealRun?.();
		});
		await waitFor(() => expect(result.current.isRunning).toBe(false));
		expect(result.current.lines).toEqual([
			expect.objectContaining({ line: "Updating web" }),
		]);
		expect(result.current.report?.containers).toEqual([
			{ containerName: "web", imageName: "nginx:1.28", updated: "updated" },
		]);
		expect(invalidateSpy).toHaveBeenCalledWith({
			queryKey: quadletsQueryKey("m-1"),
		});
		expect(invalidateSpy).toHaveBeenCalledWith({
			queryKey: containersQueryKey("m-1"),
		});
		expect(invalidateSpy).toHaveBeenCalledWith({
			queryKey: imagesQueryKey("m-1"),
		});
	});
});
