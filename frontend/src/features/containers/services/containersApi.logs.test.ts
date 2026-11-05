import { containerLogs } from "@features/containers/services/containersApi";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import { server } from "@/test/server";
import { chunkedStream, streamResponse } from "@/test/stream";

// Adapter-wiring smoke: the builder hands the factory the container logs
// path/url pair (the handler only matches that endpoint, so a captured param
// proves the wiring). The log ritual itself is pinned once in
// @shared/streaming/logSource.test.ts.
describe("containerLogs", () => {
	it("wires both source forms to the container logs endpoint", async () => {
		const captured: {
			tailQuery: URLSearchParams | null;
			followQuery: URLSearchParams | null;
		} = { tailQuery: null, followQuery: null };
		server.use(
			http.get("*/api/hosts/m-1/containers/c-1/logs", ({ request }) => {
				const query = new URL(request.url).searchParams;
				if (query.get("follow") === "true") {
					captured.followQuery = query;
					return streamResponse(chunkedStream([]));
				}
				captured.tailQuery = query;
				return HttpResponse.json({ id: "c-1", logs: "" });
			}),
		);

		const source = containerLogs("m-1", "c-1");
		await source.fetchTail(50);
		await source.follow(100, () => {}, new AbortController().signal);

		expect(captured.tailQuery?.get("tail")).toBe("50");
		expect(captured.followQuery?.get("tail")).toBe("100");
		expect(captured.followQuery?.get("follow")).toBe("true");
	});
});
