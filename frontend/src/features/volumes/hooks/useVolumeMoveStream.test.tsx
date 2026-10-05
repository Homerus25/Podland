import { useVolumeMoveStream } from "@features/volumes/hooks/useVolumeMoveStream";
import type { VolumeMoveResult } from "@features/volumes/services/volumesApi";
import { volumesQueryKey } from "@shared/queries/queryKeys";
import { act, waitFor } from "@testing-library/react";
import { type DefaultBodyType, type HttpResponse, http } from "msw";
import { describe, expect, it, vi } from "vitest";
import { volumeMoveResultFixture } from "@/test/factories";
import { renderHookWithQuery } from "@/test/render";
import { server } from "@/test/server";
import { ndjsonResponse } from "@/test/stream";

type MoveResponse = HttpResponse<DefaultBodyType>;
type MoveHandler = (request: Request) => MoveResponse | Promise<MoveResponse>;

// The move stream's events mirror the backend's VolumeMoveEvent
// (backend/internal/domain/volumes/dto.go). The generic NDJSON test builder
// types pull-stream events, so these travel as raw framed lines.
type MoveStreamEvent = Record<string, unknown> & {
	type: string;
	operationId: string;
};

function volumeEvent(overrides: Record<string, unknown> = {}): MoveStreamEvent {
	return {
		operationId: "op-1",
		type: "volume",
		machineId: "m-1",
		volume: "app-data",
		phase: "start",
		...overrides,
	};
}

function completeEvent(result: VolumeMoveResult): MoveStreamEvent {
	return {
		operationId: "op-1",
		type: "complete",
		machineId: "m-1",
		result,
	};
}

function errorEvent(code: string, message: string): MoveStreamEvent {
	return {
		operationId: "op-1",
		type: "error",
		machineId: "m-1",
		error: { code, message },
	};
}

function streamResponse(events: MoveStreamEvent[]) {
	return ndjsonResponse(
		[],
		events.map((event) => `${JSON.stringify(event)}\n`),
	);
}

const moveUrl = "*/api/machines/m-1/volumes/app-data/move";

let requests: Request[] = [];

function mockMoveStream(handler: MoveHandler) {
	requests = [];
	server.use(
		http.post(moveUrl, ({ request }) => {
			requests.push(request);
			return handler(request);
		}),
	);
}

const transferInput = {
	targetMachineId: "m-2",
	mode: "move" as const,
	force: false,
};

function renderMoveStreamHook() {
	return renderHookWithQuery(() => useVolumeMoveStream("m-1"));
}

describe("useVolumeMoveStream", () => {
	it("POSTs the transfer body to the move endpoint as an NDJSON operation", async () => {
		mockMoveStream(() =>
			streamResponse([completeEvent(volumeMoveResultFixture())]),
		);
		const { result } = renderMoveStreamHook();

		await act(async () => {
			result.current.run("app-data", transferInput);
		});

		expect(requests).toHaveLength(1);
		expect(requests[0].method).toBe("POST");
		expect(new URL(requests[0].url).pathname).toBe(
			"/api/machines/m-1/volumes/app-data/move",
		);
		expect(requests[0].headers.get("Content-Type")).toBe("application/json");
		await waitFor(() => expect(result.current.isRunning).toBe(false));
		expect(await requests[0].json()).toEqual(transferInput);
		expect(result.current.result?.status).toBe("moved");
		expect(result.current.error).toBeNull();
	});

	it("folds progress events in place, the latest phase winning", async () => {
		mockMoveStream(() =>
			streamResponse([
				volumeEvent({ phase: "start" }),
				volumeEvent({ phase: "bytes", bytes: 1048576 }),
				volumeEvent({ phase: "bytes", bytes: 2097152 }),
				volumeEvent({ phase: "done", bytes: 2097152 }),
				completeEvent(volumeMoveResultFixture()),
			]),
		);
		const { result } = renderMoveStreamHook();

		await act(async () => {
			result.current.run("app-data", transferInput);
		});

		await waitFor(() => expect(result.current.isRunning).toBe(false));
		expect(result.current.progress).toEqual({
			phase: "done",
			bytes: 2097152,
		});
		expect(result.current.result?.bytes).toBe(2097152);
	});

	it("invalidates the volume queries of both machines on the terminal complete event", async () => {
		mockMoveStream(() =>
			streamResponse([completeEvent(volumeMoveResultFixture())]),
		);
		const { result, queryClient } = renderMoveStreamHook();
		const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

		await act(async () => {
			result.current.run("app-data", transferInput);
		});

		await waitFor(() => expect(result.current.isRunning).toBe(false));
		expect(invalidateSpy).toHaveBeenCalledWith({
			queryKey: volumesQueryKey("m-1"),
		});
		expect(invalidateSpy).toHaveBeenCalledWith({
			queryKey: volumesQueryKey("m-2"),
		});
	});

	it("invalidates both machines on a partial result too — the data crossed", async () => {
		mockMoveStream(() =>
			streamResponse([
				completeEvent(
					volumeMoveResultFixture({
						status: "partial",
						error: "failed to remove volume: volume is in use",
					}),
				),
			]),
		);
		const { result, queryClient } = renderMoveStreamHook();
		const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

		await act(async () => {
			result.current.run("app-data", transferInput);
		});

		await waitFor(() => expect(result.current.isRunning).toBe(false));
		expect(invalidateSpy).toHaveBeenCalledWith({
			queryKey: volumesQueryKey("m-1"),
		});
		expect(invalidateSpy).toHaveBeenCalledWith({
			queryKey: volumesQueryKey("m-2"),
		});
	});

	it("surfaces a terminal error event with its code and message, without a result", async () => {
		mockMoveStream(() =>
			streamResponse([
				volumeEvent({ phase: "start" }),
				errorEvent(
					"VOLUME_ALREADY_EXISTS",
					"Volume app-data already exists on the target machine",
				),
			]),
		);
		const { result } = renderMoveStreamHook();

		await act(async () => {
			result.current.run("app-data", transferInput);
		});

		await waitFor(() => expect(result.current.isRunning).toBe(false));
		expect(result.current.error).toEqual({
			code: "VOLUME_ALREADY_EXISTS",
			message: "Volume app-data already exists on the target machine",
		});
		expect(result.current.result).toBeNull();
	});
});
