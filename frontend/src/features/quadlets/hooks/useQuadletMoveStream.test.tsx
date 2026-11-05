import { useQuadletMoveStream } from "@features/quadlets/hooks/useQuadletMoveStream";
import type { QuadletMoveResult } from "@features/quadlets/services/quadletsApi";
import { act, waitFor } from "@testing-library/react";
import { type DefaultBodyType, type HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import { quadletMoveResultFixture } from "@/test/factories";
import { renderHookWithQuery } from "@/test/render";
import { server } from "@/test/server";
import { ndjsonResponse } from "@/test/stream";

// The quadlet move stream's domain assertions: the request shape (the
// surface's URL builder and transfer body) and the first-seen-order progress
// fold. The ritual — the both-hosts invalidation, the terminal handling,
// the malformed-event skip — is pinned once in the transfer-hooks module's
// suite (shared/transfers/transferHooks.test.tsx).

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
		hostId: "m-1",
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
		hostId: "m-1",
		result,
	};
}

function streamResponse(events: MoveStreamEvent[]) {
	return ndjsonResponse(
		[],
		events.map((event) => `${JSON.stringify(event)}\n`),
	);
}

const moveUrl = "*/api/hosts/m-1/quadlets/web/move";

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
	targetHostId: "m-2",
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
			"/api/hosts/m-1/quadlets/web/move",
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
});
