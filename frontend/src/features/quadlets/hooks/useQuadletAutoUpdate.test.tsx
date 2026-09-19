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
	return { operationId: "op-1", type: "log", stream, line, machineId: "m-1" };
}

function report(overrides: Record<string, unknown> = {}): AutoUpdateEvent {
	return {
		operationId: "op-1",
		type: "complete",
		machineId: "m-1",
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
		machineId: "m-1",
		error: { code, message },
	};
}

const autoUpdateUrl = "*/api/machines/:machineId/quadlets/auto-update";

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
			"/api/machines/m-1/quadlets/auto-update",
		);
		expect(new URL(requests[0].url).searchParams.get("dryRun")).toBe("true");
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
		await waitFor(() => expect(result.current.isRunning).toBe(false));
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

	it("surfaces a terminal error event as a connection error, not a report", async () => {
		mockAutoUpdate(() =>
			streamResponse([errorEvent("SSH_FAILED", "Machine unreachable")]),
		);
		const { result } = renderAutoUpdateHook();

		await act(async () => {
			result.current.run(false);
		});

		await waitFor(() => expect(result.current.isRunning).toBe(false));
		expect(result.current.connectionError).toEqual({
			code: "SSH_FAILED",
			message: "Machine unreachable",
		});
		expect(result.current.report).toBeNull();
	});
});
