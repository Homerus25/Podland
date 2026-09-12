import {
	followLogStream,
	isLogEvent,
	parseLogErrorEvent,
} from "@shared/streaming/logStream";
import { http } from "msw";
import { describe, expect, it } from "vitest";
import { server } from "@/test/server";
import { chunkedStream, streamResponse } from "@/test/stream";

function logEvent(
	overrides: Partial<{
		timestamp: string;
		level: string;
		message: string;
	}> = {},
) {
	return JSON.stringify({
		timestamp: "2026-08-26T10:00:00Z",
		level: "INFO",
		message: "server started",
		...overrides,
	});
}

describe("isLogEvent", () => {
	it("accepts the backend log event shape", () => {
		expect(
			isLogEvent({
				timestamp: "2026-08-26T10:00:00Z",
				level: "INFO",
				message: "hi",
			}),
		).toBe(true);
	});

	it("rejects other shapes", () => {
		expect(isLogEvent(null)).toBe(false);
		expect(isLogEvent({ operationId: "op", type: "progress" })).toBe(false);
		expect(isLogEvent({ timestamp: "t", level: "INFO" })).toBe(false);
	});
});

describe("parseLogErrorEvent", () => {
	it("splits an ERROR-level event into code and message", () => {
		expect(
			parseLogErrorEvent({
				timestamp: "t",
				level: "ERROR",
				message: "CONTAINER_NOT_FOUND: no such container",
			}),
		).toEqual({ code: "CONTAINER_NOT_FOUND", message: "no such container" });
	});

	it("parses quadlet terminal errors into an ApiError", () => {
		expect(
			parseLogErrorEvent({
				timestamp: "t",
				level: "ERROR",
				message: "QUADLET_NOT_FOUND: Quadlet not found",
			}),
		).toEqual({ code: "QUADLET_NOT_FOUND", message: "Quadlet not found" });
		expect(
			parseLogErrorEvent({
				timestamp: "t",
				level: "ERROR",
				message: "CONNECTION_FAILED: Failed to connect to machine",
			}),
		).toEqual({
			code: "CONNECTION_FAILED",
			message: "Failed to connect to machine",
		});
	});

	it("returns null for non-error events", () => {
		expect(
			parseLogErrorEvent({ timestamp: "t", level: "INFO", message: "fine" }),
		).toBeNull();
		expect(
			parseLogErrorEvent({
				timestamp: "t",
				level: "ERROR",
				message: "no code prefix",
			}),
		).toBeNull();
	});

	it("returns null for stderr lines that merely look like coded errors", () => {
		expect(
			parseLogErrorEvent({
				timestamp: "t",
				level: "ERROR",
				message: "FATAL: could not bind port",
			}),
		).toBeNull();
		expect(
			parseLogErrorEvent({
				timestamp: "t",
				level: "ERROR",
				message: "ERROR: socket closed",
			}),
		).toBeNull();
	});
});

describe("followLogStream", () => {
	it("emits each log event and skips non-log values", async () => {
		const events: { message: string }[] = [];
		server.use(
			http.get("*/api/logs", () =>
				streamResponse(
					chunkedStream([
						`${logEvent()}\n`,
						`${JSON.stringify({ operationId: "op", type: "progress" })}\n`,
						`${logEvent({ message: "second" })}\n`,
					]),
				),
			),
		);

		await followLogStream(
			"/api/logs",
			(event) => events.push({ message: event.message }),
			new AbortController().signal,
		);

		expect(events.map((e) => e.message)).toEqual(["server started", "second"]);
	});
});
