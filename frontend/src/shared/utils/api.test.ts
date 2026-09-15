import {
	api,
	apiBaseUrl,
	bareApi,
	getApiError,
	setBearerToken,
} from "@shared/utils/api";
import { AxiosError, type AxiosResponse } from "axios";
import { HttpResponse, http } from "msw";
import { afterEach, describe, expect, it } from "vitest";
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

describe("bare transport", () => {
	afterEach(() => {
		setBearerToken(null);
	});

	it("shares the configured base URL with the shared instance", () => {
		expect(bareApi.defaults.baseURL).toBe(apiBaseUrl);
		expect(api.defaults.baseURL).toBe(apiBaseUrl);
	});

	// The structural guarantee of this slice: nothing intercepts the bare
	// instance, so auth calls stay outside any request/response policy.
	it("has no request or response interceptors attached", () => {
		expect(bareApi.interceptors.request.handlers).toHaveLength(0);
		expect(bareApi.interceptors.response.handlers).toHaveLength(0);
	});

	// Auth calls and the future 401 policy's refresh ride this instance
	// precisely so no interceptor (and later, no 401 policy) can touch
	// them; a missing bearer header is the observable pin for that.
	it("carries no Authorization header even with a bearer token installed", async () => {
		setBearerToken("stale-access-token");
		let authorization: string | null = null;
		server.use(
			http.get("*/api/health", ({ request }) => {
				authorization = request.headers.get("authorization");
				return new HttpResponse("OK", { status: 200 });
			}),
		);

		const response = await bareApi.get("/health");

		expect(response.status).toBe(200);
		expect(authorization).toBeNull();
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
