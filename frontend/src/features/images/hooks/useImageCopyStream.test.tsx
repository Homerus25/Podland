import { useImageCopyStream } from "@features/images/hooks/useImageCopyStream";
import { act, waitFor } from "@testing-library/react";
import { type DefaultBodyType, type HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import {
	type ImageCopyStreamEvent,
	imageCopyCompleteEventFixture,
	imageCopyEventFixture,
	imageCopyResultFixture,
} from "@/test/factories";
import { renderHookWithQuery } from "@/test/render";
import { server } from "@/test/server";
import { ndjsonResponse } from "@/test/stream";

// The image copy stream's domain assertions: the request shape (the
// surface's URL builder and copy body) and the single-value progress fold.
// The ritual — the both-hosts invalidation, the terminal handling, the
// malformed-event skip — is pinned once in the transfer-hooks module's
// suite (shared/transfers/transferHooks.test.tsx).

const imageId =
	"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa";

type CopyResponse = HttpResponse<DefaultBodyType>;
type CopyHandler = (request: Request) => CopyResponse | Promise<CopyResponse>;

// The copy stream's events mirror the backend's ImageCopyEvent
// (backend/internal/domain/images/copy.go, issue 375); the shared
// image-copy event fixtures build them.

function streamResponse(events: ImageCopyStreamEvent[]) {
	return ndjsonResponse(
		[],
		events.map((event) => `${JSON.stringify(event)}\n`),
	);
}

const copyUrl = `*/api/hosts/m-1/images/${encodeURIComponent(imageId)}/copy`;

let requests: Request[] = [];

function mockCopyStream(handler: CopyHandler) {
	requests = [];
	server.use(
		http.post(copyUrl, ({ request }) => {
			requests.push(request);
			return handler(request);
		}),
	);
}

const copyInput = { targetHostId: "m-2" };

function renderCopyStreamHook() {
	return renderHookWithQuery(() => useImageCopyStream("m-1"));
}

describe("useImageCopyStream", () => {
	it("POSTs the copy body to the copy endpoint as an NDJSON operation", async () => {
		mockCopyStream(() =>
			streamResponse([imageCopyCompleteEventFixture(imageCopyResultFixture())]),
		);
		const { result } = renderCopyStreamHook();

		await act(async () => {
			result.current.run(imageId, copyInput);
		});

		expect(requests).toHaveLength(1);
		expect(requests[0].method).toBe("POST");
		expect(new URL(requests[0].url).pathname).toBe(
			`/api/hosts/m-1/images/${encodeURIComponent(imageId)}/copy`,
		);
		expect(requests[0].headers.get("Content-Type")).toBe("application/json");
		await waitFor(() => expect(result.current.isRunning).toBe(false));
		expect(await requests[0].json()).toEqual(copyInput);
		expect(result.current.result?.status).toBe("copied");
		expect(result.current.error).toBeNull();
	});

	it("folds progress events in place, the latest phase winning", async () => {
		mockCopyStream(() =>
			streamResponse([
				imageCopyEventFixture({ phase: "start" }),
				imageCopyEventFixture({ phase: "bytes", bytes: 1048576 }),
				imageCopyEventFixture({ phase: "bytes", bytes: 2097152 }),
				imageCopyEventFixture({ phase: "done", bytes: 2097152 }),
				imageCopyCompleteEventFixture(imageCopyResultFixture()),
			]),
		);
		const { result } = renderCopyStreamHook();

		await act(async () => {
			result.current.run(imageId, copyInput);
		});

		await waitFor(() => expect(result.current.isRunning).toBe(false));
		expect(result.current.progress).toEqual({
			phase: "done",
			bytes: 2097152,
		});
		expect(result.current.result?.bytes).toBe(2097152);
	});
});
