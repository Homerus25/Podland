import { createLogSource } from "@shared/streaming/logSource";
import { machineApiPath, machineApiUrl } from "@shared/utils/api";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import { server } from "@/test/server";
import { chunkedStream, streamResponse } from "@/test/stream";

// The factory takes a pre-built machine-scoped pair: the caller keeps the
// machine vocabulary through the builder pair, the kit owns everything else.
const logsPath = machineApiPath("m-1", "containers", "c-1", "logs");
const logsUrl = machineApiUrl("m-1", "containers", "c-1", "logs");

function logEvent(message: string): string {
	return JSON.stringify({
		timestamp: "2026-08-26T10:00:00Z",
		level: "INFO",
		message,
	});
}

describe("createLogSource", () => {
	it("fetches the buffered tail with the tail param and the split, non-empty lines", async () => {
		const request = { tail: null as string | null };
		server.use(
			http.get(
				"*/api/machines/m-1/containers/c-1/logs",
				({ request: sent }) => {
					request.tail = new URL(sent.url).searchParams.get("tail");
					return HttpResponse.json({
						id: "c-1",
						logs: "first line\nsecond line\n",
					});
				},
			),
		);

		const source = createLogSource(logsPath, logsUrl);
		const lines = await source.fetchTail(50);

		expect(request.tail).toBe("50");
		expect(lines).toEqual(["first line", "second line"]);
	});

	it("follows with a URLSearchParams-built tail + follow=true query", async () => {
		const captured: { query: URLSearchParams | null } = { query: null };
		server.use(
			http.get("*/api/machines/m-1/containers/c-1/logs", ({ request }) => {
				captured.query = new URL(request.url).searchParams;
				return streamResponse(chunkedStream([`${logEvent("live line")}\n`]));
			}),
		);

		const source = createLogSource(logsPath, logsUrl);
		const messages: string[] = [];
		await source.follow(
			100,
			(event) => messages.push(event.message),
			new AbortController().signal,
		);

		expect(captured.query?.get("tail")).toBe("100");
		expect(captured.query?.get("follow")).toBe("true");
		expect(messages).toEqual(["live line"]);
	});

	it("fetches the entire log history without a tail param and returns the raw text", async () => {
		const request = { tail: null as string | null };
		server.use(
			http.get(
				"*/api/machines/m-1/containers/c-1/logs",
				({ request: sent }) => {
					request.tail = new URL(sent.url).searchParams.get("tail");
					return HttpResponse.json({
						id: "c-1",
						logs: "first line\nsecond line\n",
					});
				},
			),
		);

		const source = createLogSource(logsPath, logsUrl);
		const text = await source.fetchAll();

		expect(request.tail).toBeNull();
		expect(text).toBe("first line\nsecond line\n");
	});

	it("wraps a failing buffered fetch in the ApiError envelope", async () => {
		server.use(
			http.get("*/api/machines/m-1/containers/c-1/logs", () =>
				HttpResponse.json(
					{
						error: {
							code: "CONTAINER_NOT_FOUND",
							message: "no such container",
						},
					},
					{ status: 404 },
				),
			),
		);

		const source = createLogSource(logsPath, logsUrl);

		await expect(source.fetchTail(50)).rejects.toEqual({
			code: "CONTAINER_NOT_FOUND",
			message: "no such container",
		});
	});
});
