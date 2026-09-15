import {
	api,
	getApiError,
	machineApiPath,
	machineApiUrl,
} from "@shared/utils/api";
import { downloadFile } from "@shared/utils/download";
import { AxiosError, type AxiosResponse } from "axios";
import { HttpResponse, http } from "msw";
import { afterEach, describe, expect, it, vi } from "vitest";
import { server } from "@/test/server";

function responseOf(status: number, data: unknown): AxiosResponse {
	return {
		status,
		data,
		statusText: "",
		headers: {},
		config: {},
	} as AxiosResponse;
}

describe("api client", () => {
	it("returns OK from /api/health through the shared API client", async () => {
		const response = await api.get("/health");

		expect(response.status).toBe(200);
		expect(response.data).toBe("OK");
	});
});

describe("machineApiPath", () => {
	it("encodes every segment, machineId first", () => {
		expect(machineApiPath("m 1", "containers", "c/1")).toBe(
			"/machines/m%201/containers/c%2F1",
		);
	});

	it("emits the bare machine path with no extra segments", () => {
		expect(machineApiPath("m-1")).toBe("/machines/m-1");
	});
});

describe("machineApiUrl", () => {
	afterEach(() => {
		vi.unstubAllEnvs();
		vi.resetModules();
	});

	it("prefixes the default base onto the encoded path", () => {
		expect(machineApiUrl("m 1", "images", "pull")).toBe(
			"/api/machines/m%201/images/pull",
		);
	});

	it("strips a trailing slash from a configured base", async () => {
		vi.stubEnv("VITE_API_BASE_URL", "https://backend.example/api/");
		vi.resetModules();
		const { machineApiUrl: withStubbedBase } = await import(
			"@shared/utils/api"
		);
		expect(withStubbedBase("m-1", "volumes", "v", "export")).toBe(
			"https://backend.example/api/machines/m-1/volumes/v/export",
		);
	});

	it("lands a download fetch on a configured non-default base", async () => {
		Object.defineProperty(URL, "createObjectURL", {
			value: vi.fn(() => "blob:mock"),
			configurable: true,
			writable: true,
		});
		Object.defineProperty(URL, "revokeObjectURL", {
			value: vi.fn(),
			configurable: true,
			writable: true,
		});
		const createElement = document.createElement.bind(document);
		vi.spyOn(document, "createElement").mockImplementation((tag) => {
			if (tag === "a") {
				return { click: vi.fn() } as unknown as HTMLAnchorElement;
			}
			return createElement(tag);
		});
		document.body.appendChild = vi.fn() as typeof document.body.appendChild;
		document.body.removeChild = vi.fn() as typeof document.body.removeChild;

		vi.stubEnv("VITE_API_BASE_URL", "https://backend.example/api");
		vi.resetModules();
		const { machineApiUrl: withStubbedBase } = await import(
			"@shared/utils/api"
		);
		const requests: URL[] = [];
		server.use(
			http.get("*/api/machines/m-1/volumes/shared/export", ({ request }) => {
				requests.push(new URL(request.url));
				return HttpResponse.arrayBuffer(new ArrayBuffer(0), {
					headers: { "Content-Type": "application/x-tar" },
				});
			}),
		);

		await downloadFile(
			withStubbedBase("m-1", "volumes", "shared", "export"),
			"shared.tar",
		);

		expect(requests).toHaveLength(1);
		expect(requests[0].origin).toBe("https://backend.example");
		expect(requests[0].pathname).toBe(
			"/api/machines/m-1/volumes/shared/export",
		);
	});
});

describe("getApiError", () => {
	it("passes through the envelope code and message for client errors", () => {
		const error = new AxiosError(
			"Request failed",
			"ERR_BAD_REQUEST",
			undefined,
			undefined,
			responseOf(401, {
				requestId: "req-1",
				error: {
					code: "INVALID_CREDENTIALS",
					message: "Invalid username or password",
				},
			}),
		);

		expect(getApiError(error)).toEqual({
			code: "INVALID_CREDENTIALS",
			message: "Invalid username or password",
		});
	});

	it("collapses server errors to the generic message regardless of the body", () => {
		const error = new AxiosError(
			"Server error",
			"ERR_BAD_RESPONSE",
			undefined,
			undefined,
			responseOf(503, {
				requestId: "req-2",
				error: {
					code: "SERVICE_UNAVAILABLE",
					message: "Service temporarily unavailable",
				},
			}),
		);

		expect(getApiError(error)).toEqual({
			code: "INTERNAL_ERROR",
			message: "An unexpected error occurred",
		});
	});

	it("collapses network failures without a response to the generic message", () => {
		expect(getApiError(new AxiosError("Network Error"))).toEqual({
			code: "INTERNAL_ERROR",
			message: "An unexpected error occurred",
		});
	});

	it("collapses non-envelope error bodies to the generic message", () => {
		const error = new AxiosError(
			"Bad request",
			"ERR_BAD_REQUEST",
			undefined,
			undefined,
			responseOf(400, "Bad Request"),
		);

		expect(getApiError(error)).toEqual({
			code: "INTERNAL_ERROR",
			message: "An unexpected error occurred",
		});
	});

	it("collapses internal error codes to the generic message even on a 4xx status", () => {
		const error = new AxiosError(
			"Bad request",
			"ERR_BAD_REQUEST",
			undefined,
			undefined,
			responseOf(400, {
				requestId: "req-3",
				error: {
					code: "INTERNAL_ERROR",
					message: "Invalid username or password",
				},
			}),
		);

		expect(getApiError(error)).toEqual({
			code: "INTERNAL_ERROR",
			message: "An unexpected error occurred",
		});
	});
});
