import { useVolumeMoveStream } from "@features/volumes/hooks/useVolumeMoveStream";
import type { VolumeMoveResult } from "@features/volumes/services/volumesApi";
import { act, waitFor } from "@testing-library/react";
import { type DefaultBodyType, type HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import { volumeMoveResultFixture } from "@/test/factories";
import { renderHookWithQuery } from "@/test/render";
import { server } from "@/test/server";
import { ndjsonResponse } from "@/test/stream";

// The volume move stream's domain assertions: the request shape (the
// surface's URL builder and transfer body) and the single-value progress
// fold. The ritual — the both-machines invalidation, the terminal handling,
// the malformed-event skip — is pinned once in the transfer-hooks module's
// suite (shared/transfers/transferHooks.test.tsx).

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
});
