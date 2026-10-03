import { useQuadletMoveStream } from "@features/quadlets/hooks/useQuadletMoveStream";
import type { QuadletMoveResult } from "@features/quadlets/services/quadletsApi";
import { quadletsQueryKey } from "@shared/queries/queryKeys";
import { act, waitFor } from "@testing-library/react";
import { type DefaultBodyType, type HttpResponse, http } from "msw";
import { describe, expect, it, vi } from "vitest";
import { quadletMoveResultFixture } from "@/test/factories";
import { renderHookWithQuery } from "@/test/render";
import { server } from "@/test/server";
import { ndjsonResponse } from "@/test/stream";

type MoveResponse = HttpResponse<DefaultBodyType>;
type MoveHandler = (request: Request) => MoveResponse | Promise<MoveResponse>;

// The move stream's events mirror the backend's QuadletMoveEvent
// (backend/internal/domain/quadlets/dto.go). The generic NDJSON test builder
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
		ref: "pgdata",
		podmanVolume: "pgdata",
		phase: "start",
		...overrides,
	};
}

function completeEvent(result: QuadletMoveResult): MoveStreamEvent {
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

const moveUrl = "*/api/machines/m-1/quadlets/web/move";

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
	stop: true,
	migrateVolumeData: true,
};

function renderMoveStreamHook() {
	return renderHookWithQuery(() => useQuadletMoveStream("m-1"));
}

describe("useQuadletMoveStream", () => {
	it("POSTs the transfer body to the move endpoint as an NDJSON operation", async () => {
		mockMoveStream(() =>
			streamResponse([completeEvent(quadletMoveResultFixture())]),
		);
		const { result } = renderMoveStreamHook();

		await act(async () => {
			result.current.run("web", transferInput);
		});

		expect(requests).toHaveLength(1);
		expect(requests[0].method).toBe("POST");
		expect(new URL(requests[0].url).pathname).toBe(
			"/api/machines/m-1/quadlets/web/move",
		);
		expect(requests[0].headers.get("Content-Type")).toBe("application/json");
		await waitFor(() => expect(result.current.isRunning).toBe(false));
		expect(await requests[0].json()).toEqual(transferInput);
		expect(result.current.result?.status).toBe("moved");
		expect(result.current.error).toBeNull();
	});

	it("folds per-volume progress events in place, latest phase per volume winning", async () => {
		mockMoveStream(() =>
			streamResponse([
				volumeEvent({ phase: "start" }),
				volumeEvent({ phase: "bytes", bytes: 1048576 }),
				volumeEvent({ phase: "bytes", bytes: 2097152 }),
				volumeEvent({
					ref: "cache.volume",
					podmanVolume: "systemd-cache",
					phase: "skip",
					reason: "source volume does not exist",
				}),
				volumeEvent({ phase: "done", bytes: 2097152 }),
				completeEvent(
					quadletMoveResultFixture({
						status: "copied",
						volumeMigration: { volumes: [] },
					}),
				),
			]),
		);
		const { result } = renderMoveStreamHook();

		await act(async () => {
			result.current.run("web", transferInput);
		});

		await waitFor(() => expect(result.current.isRunning).toBe(false));
		expect(result.current.progress).toEqual([
			{ ref: "pgdata", podmanVolume: "pgdata", phase: "done", bytes: 2097152 },
			{
				ref: "cache.volume",
				podmanVolume: "systemd-cache",
				phase: "skip",
				reason: "source volume does not exist",
			},
		]);
		expect(result.current.result?.status).toBe("copied");
	});

	it("invalidates the quadlet queries of both machines on the terminal complete event", async () => {
		mockMoveStream(() =>
			streamResponse([
				completeEvent(
					quadletMoveResultFixture({
						volumeMigration: { volumes: [] },
					}),
				),
			]),
		);
		const { result, queryClient } = renderMoveStreamHook();
		const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

		await act(async () => {
			result.current.run("web", transferInput);
		});

		await waitFor(() => expect(result.current.isRunning).toBe(false));
		expect(invalidateSpy).toHaveBeenCalledWith({
			queryKey: quadletsQueryKey("m-1"),
		});
		expect(invalidateSpy).toHaveBeenCalledWith({
			queryKey: quadletsQueryKey("m-2"),
		});
	});

	it("surfaces a terminal error event with its code and message, without a result", async () => {
		mockMoveStream(() =>
			streamResponse([
				volumeEvent({ phase: "start" }),
				errorEvent(
					"VOLUME_MIGRATION_TARGET_NOT_EMPTY",
					"target volume pgdata is not empty",
				),
			]),
		);
		const { result } = renderMoveStreamHook();

		await act(async () => {
			result.current.run("web", transferInput);
		});

		await waitFor(() => expect(result.current.isRunning).toBe(false));
		expect(result.current.error).toEqual({
			code: "VOLUME_MIGRATION_TARGET_NOT_EMPTY",
			message: "target volume pgdata is not empty",
		});
		expect(result.current.result).toBeNull();
	});
});
