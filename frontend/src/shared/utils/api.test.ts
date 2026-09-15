import {
	api,
	bareApi,
	getApiError,
	handleUnauthorized,
	machineApiPath,
	machineApiUrl,
	setBearerToken,
	setSessionExpiredHandler,
} from "@shared/utils/api";
import { downloadFile } from "@shared/utils/download";
import { AxiosError, type AxiosResponse } from "axios";
import { HttpResponse, http } from "msw";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { server } from "@/test/server";

const tokenExpiredEnvelope = (requestId: string) => ({
	requestId,
	error: { code: "TOKEN_EXPIRED", message: "Token has expired" },
});

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

describe("bare transport", () => {
	afterEach(() => {
		setBearerToken(null);
	});

	it("shares the configured base URL with the shared instance", () => {
		expect(bareApi.defaults.baseURL).toBe(api.defaults.baseURL);
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

describe("401 policy", () => {
	let refreshCalls: number;
	let seamCalls: number;

	beforeEach(() => {
		refreshCalls = 0;
		seamCalls = 0;
	});

	afterEach(() => {
		setBearerToken(null);
		setSessionExpiredHandler(null);
	});

	function refreshHandler(token = "fresh-access-token", delayMs = 0) {
		return http.post("*/api/auth/refresh", async () => {
			refreshCalls++;
			if (delayMs > 0) await new Promise((r) => setTimeout(r, delayMs));
			return HttpResponse.json({ token, username: "alice" });
		});
	}

	function rejectedRefreshHandler() {
		return http.post("*/api/auth/refresh", () => {
			refreshCalls++;
			return HttpResponse.json(
				{
					requestId: "req-refresh",
					error: {
						code: "AUTH_REFRESH_INVALID",
						message: "Refresh token invalid",
					},
				},
				{ status: 401 },
			);
		});
	}

	function machinesHandler(respond: (call: number) => Response) {
		let calls = 0;
		return http.get("*/api/machines", () => respond(++calls));
	}

	function unauthorized(call: number) {
		return HttpResponse.json(tokenExpiredEnvelope(`req-${call}`), {
			status: 401,
		});
	}

	it("resolves true only when a fresh token was installed", async () => {
		server.use(refreshHandler());
		setBearerToken("expired-access-token");

		await expect(handleUnauthorized()).resolves.toBe(true);
		expect(refreshCalls).toBe(1);

		server.use(rejectedRefreshHandler());
		await expect(handleUnauthorized()).resolves.toBe(false);
	});

	it("refreshes once and replays the 401ing request so it resolves normally", async () => {
		let machineCalls = 0;
		let replayAuthorization: string | null = null;
		server.use(
			refreshHandler(),
			http.get("*/api/machines", ({ request }) => {
				machineCalls++;
				if (machineCalls === 1) return unauthorized(machineCalls);
				replayAuthorization = request.headers.get("authorization");
				return HttpResponse.json([]);
			}),
		);
		setBearerToken("expired-access-token");

		const response = await api.get("/machines");

		expect(response.status).toBe(200);
		expect(refreshCalls).toBe(1);
		expect(machineCalls).toBe(2);
		expect(replayAuthorization).toBe("Bearer fresh-access-token");
	});

	it("shares one refresh across concurrent 401s and replays every queued caller", async () => {
		server.use(
			refreshHandler("fresh-access-token", 20),
			machinesHandler((call) =>
				call <= 2 ? unauthorized(call) : HttpResponse.json([]),
			),
		);
		setBearerToken("expired-access-token");

		const [first, second] = await Promise.all([
			api.get("/machines"),
			api.get("/machines"),
		]);

		expect(first.status).toBe(200);
		expect(second.status).toBe(200);
		expect(refreshCalls).toBe(1);
	});

	it("fires the seam exactly once and rejects with the original 401 when refresh is rejected", async () => {
		server.use(
			rejectedRefreshHandler(),
			machinesHandler(() => unauthorized(1)),
		);
		setBearerToken("expired-access-token");
		setSessionExpiredHandler(() => {
			seamCalls++;
		});

		let caught: AxiosError | undefined;
		try {
			await api.get("/machines");
		} catch (error) {
			caught = error as AxiosError;
		}

		expect(caught?.response?.status).toBe(401);
		expect(caught?.response?.data).toEqual(tokenExpiredEnvelope("req-1"));
		expect(seamCalls).toBe(1);
		expect(refreshCalls).toBe(1);
	});

	it("treats a network failure of the refresh call as session death too", async () => {
		server.use(
			http.post("*/api/auth/refresh", () => {
				refreshCalls++;
				return HttpResponse.error();
			}),
			machinesHandler(() => unauthorized(1)),
		);
		setBearerToken("expired-access-token");
		setSessionExpiredHandler(() => {
			seamCalls++;
		});

		await expect(api.get("/machines")).rejects.toMatchObject({
			response: { status: 401 },
		});
		expect(seamCalls).toBe(1);
	});

	it("goes straight to session death when the replayed request 401s again", async () => {
		server.use(
			refreshHandler(),
			machinesHandler((call) => unauthorized(call)),
		);
		setBearerToken("expired-access-token");
		setSessionExpiredHandler(() => {
			seamCalls++;
		});

		let caught: AxiosError | undefined;
		try {
			await api.get("/machines");
		} catch (error) {
			caught = error as AxiosError;
		}

		expect(caught?.response?.data).toEqual(tokenExpiredEnvelope("req-2"));
		expect(seamCalls).toBe(1);
		expect(refreshCalls).toBe(1);
	});

	it("does not refresh or fire the seam when a login on the bare transport is rejected", async () => {
		server.use(
			http.post("*/api/auth/login", () =>
				HttpResponse.json(
					{
						requestId: "req-1",
						error: {
							code: "INVALID_CREDENTIALS",
							message: "Invalid username or password",
						},
					},
					{ status: 401 },
				),
			),
			refreshHandler(),
		);
		setSessionExpiredHandler(() => {
			seamCalls++;
		});

		await expect(
			bareApi.post("/auth/login", { username: "alice", password: "wrong" }),
		).rejects.toMatchObject({ response: { status: 401 } });

		expect(refreshCalls).toBe(0);
		expect(seamCalls).toBe(0);
	});

	it("treats seam firing with no handler registered as a no-op", async () => {
		server.use(
			rejectedRefreshHandler(),
			machinesHandler(() => unauthorized(1)),
		);
		setBearerToken("expired-access-token");

		await expect(api.get("/machines")).rejects.toMatchObject({
			response: { status: 401 },
		});
	});

	it("re-arms the seam once a new token is installed after session death", async () => {
		server.use(
			rejectedRefreshHandler(),
			machinesHandler(() => unauthorized(1)),
		);
		setSessionExpiredHandler(() => {
			seamCalls++;
		});

		setBearerToken("first-session-token");
		await expect(api.get("/machines")).rejects.toMatchObject({
			response: { status: 401 },
		});
		expect(seamCalls).toBe(1);

		setBearerToken("second-session-token");
		await expect(api.get("/machines")).rejects.toMatchObject({
			response: { status: 401 },
		});
		expect(seamCalls).toBe(2);
	});
});
