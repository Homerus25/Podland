import {
	api,
	bareApi,
	getApiError,
	getBearerToken,
	handleUnauthorized,
	hostApiPath,
	hostApiUrl,
	refreshSession,
	setBearerToken,
	setSessionExpiredHandler,
} from "@shared/utils/api";
import { downloadFile } from "@shared/utils/download";
import { AxiosError, type AxiosResponse } from "axios";
import { HttpResponse, http } from "msw";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { errorJson, errorResponse } from "@/test/errors";
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

describe("hostApiPath", () => {
	it("encodes every segment, hostId first", () => {
		expect(hostApiPath("m 1", "containers", "c/1")).toBe(
			"/hosts/m%201/containers/c%2F1",
		);
	});

	it("emits the bare host path with no extra segments", () => {
		expect(hostApiPath("m-1")).toBe("/hosts/m-1");
	});
});

describe("hostApiUrl", () => {
	afterEach(() => {
		vi.unstubAllEnvs();
		vi.resetModules();
	});

	it("prefixes the default base onto the encoded path", () => {
		expect(hostApiUrl("m 1", "images", "pull")).toBe(
			"/api/hosts/m%201/images/pull",
		);
	});

	it("strips a trailing slash from a configured base", async () => {
		vi.stubEnv("VITE_API_BASE_URL", "https://backend.example/api/");
		vi.resetModules();
		const { hostApiUrl: withStubbedBase } = await import("@shared/utils/api");
		expect(withStubbedBase("m-1", "volumes", "v", "export")).toBe(
			"https://backend.example/api/hosts/m-1/volumes/v/export",
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
		const { hostApiUrl: withStubbedBase } = await import("@shared/utils/api");
		const requests: URL[] = [];
		server.use(
			http.get("*/api/hosts/m-1/volumes/shared/export", ({ request }) => {
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
		expect(requests[0].pathname).toBe("/api/hosts/m-1/volumes/shared/export");
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
			responseOf(
				401,
				errorJson("INVALID_CREDENTIALS", "Invalid username or password"),
			),
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
			responseOf(
				503,
				errorJson(
					"SERVICE_UNAVAILABLE",
					"Service temporarily unavailable",
					"req-2",
				),
			),
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
			responseOf(
				400,
				errorJson("INTERNAL_ERROR", "Invalid username or password", "req-3"),
			),
		);

		expect(getApiError(error)).toEqual({
			code: "INTERNAL_ERROR",
			message: "An unexpected error occurred",
		});
	});
});

describe("refreshSession", () => {
	let refreshCalls: number;
	let seamCalls: number;

	beforeEach(() => {
		refreshCalls = 0;
		seamCalls = 0;
		// The session-death latch is module state that outlives a test, so
		// every test starts from a live session: installing a token re-arms
		// the latch, whatever a previous test in this file fired.
		setBearerToken("expired-access-token");
	});

	afterEach(() => {
		setBearerToken(null);
		setSessionExpiredHandler(null);
	});

	// The migrated authApi pin: refresh is cookie-based, so the access
	// token must never ride the call even with one installed.
	it("resolves the payload and installs the token without an Authorization header", async () => {
		setBearerToken("stale-access-token");
		let authorization: string | null = null;
		server.use(
			http.post("*/api/auth/refresh", ({ request }) => {
				authorization = request.headers.get("authorization");
				return HttpResponse.json({ token: "fresh-token", username: "ada" });
			}),
		);

		await expect(refreshSession()).resolves.toEqual({
			token: "fresh-token",
			username: "ada",
		});

		expect(authorization).toBeNull();
		expect(getBearerToken()).toBe("fresh-token");
	});

	// Mount asymmetry: a failed refresh at mount means logged-out, not
	// session death — only the 401 policy translates null into the seam.
	it("resolves null on failure without firing the seam; only handleUnauthorized fires it", async () => {
		server.use(
			http.post("*/api/auth/refresh", () =>
				errorResponse(
					401,
					"AUTH_REFRESH_INVALID",
					"Refresh token invalid",
					"req-refresh",
				),
			),
		);
		setSessionExpiredHandler(() => {
			seamCalls++;
		});

		await expect(refreshSession()).resolves.toBeNull();
		expect(seamCalls).toBe(0);

		await expect(handleUnauthorized()).resolves.toBe(false);
		expect(seamCalls).toBe(1);
	});

	// The contract's one invariant: mount-refresh and 401-refresh share a
	// single flight, so both callers together still trigger one POST.
	it("shares one flight between refreshSession and handleUnauthorized", async () => {
		server.use(
			http.post("*/api/auth/refresh", async () => {
				refreshCalls++;
				await new Promise((r) => setTimeout(r, 20));
				return HttpResponse.json({ token: "fresh-token", username: "ada" });
			}),
		);
		setBearerToken("expired-access-token");

		const [payload, replayed] = await Promise.all([
			refreshSession(),
			handleUnauthorized(),
		]);

		expect(payload).toEqual({ token: "fresh-token", username: "ada" });
		expect(replayed).toBe(true);
		expect(refreshCalls).toBe(1);
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
			return errorResponse(
				401,
				"AUTH_REFRESH_INVALID",
				"Refresh token invalid",
				"req-refresh",
			);
		});
	}

	function hostsHandler(respond: (call: number) => Response) {
		let calls = 0;
		return http.get("*/api/hosts", () => respond(++calls));
	}

	function unauthorized(call: number) {
		return errorResponse(
			401,
			"TOKEN_EXPIRED",
			"Token has expired",
			`req-${call}`,
		);
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
		let hostCalls = 0;
		let replayAuthorization: string | null = null;
		server.use(
			refreshHandler(),
			http.get("*/api/hosts", ({ request }) => {
				hostCalls++;
				if (hostCalls === 1) return unauthorized(hostCalls);
				replayAuthorization = request.headers.get("authorization");
				return HttpResponse.json([]);
			}),
		);
		setBearerToken("expired-access-token");

		const response = await api.get("/hosts");

		expect(response.status).toBe(200);
		expect(refreshCalls).toBe(1);
		expect(hostCalls).toBe(2);
		expect(replayAuthorization).toBe("Bearer fresh-access-token");
	});

	it("shares one refresh across concurrent 401s and replays every queued caller", async () => {
		server.use(
			refreshHandler("fresh-access-token", 20),
			hostsHandler((call) =>
				call <= 2 ? unauthorized(call) : HttpResponse.json([]),
			),
		);
		setBearerToken("expired-access-token");

		const [first, second] = await Promise.all([
			api.get("/hosts"),
			api.get("/hosts"),
		]);

		expect(first.status).toBe(200);
		expect(second.status).toBe(200);
		expect(refreshCalls).toBe(1);
	});

	it("fires the seam exactly once and rejects with the original 401 when refresh is rejected", async () => {
		server.use(
			rejectedRefreshHandler(),
			hostsHandler(() => unauthorized(1)),
		);
		setBearerToken("expired-access-token");
		setSessionExpiredHandler(() => {
			seamCalls++;
		});

		let caught: AxiosError | undefined;
		try {
			await api.get("/hosts");
		} catch (error) {
			caught = error as AxiosError;
		}

		expect(caught?.response?.status).toBe(401);
		expect(caught?.response?.data).toEqual(
			errorJson("TOKEN_EXPIRED", "Token has expired", "req-1"),
		);
		expect(seamCalls).toBe(1);
		expect(refreshCalls).toBe(1);
	});

	it("treats a network failure of the refresh call as session death too", async () => {
		server.use(
			http.post("*/api/auth/refresh", () => {
				refreshCalls++;
				return HttpResponse.error();
			}),
			hostsHandler(() => unauthorized(1)),
		);
		setBearerToken("expired-access-token");
		setSessionExpiredHandler(() => {
			seamCalls++;
		});

		await expect(api.get("/hosts")).rejects.toMatchObject({
			response: { status: 401 },
		});
		expect(seamCalls).toBe(1);
	});

	it("goes straight to session death when the replayed request 401s again", async () => {
		server.use(
			refreshHandler(),
			hostsHandler((call) => unauthorized(call)),
		);
		setBearerToken("expired-access-token");
		setSessionExpiredHandler(() => {
			seamCalls++;
		});

		let caught: AxiosError | undefined;
		try {
			await api.get("/hosts");
		} catch (error) {
			caught = error as AxiosError;
		}

		expect(caught?.response?.data).toEqual(
			errorJson("TOKEN_EXPIRED", "Token has expired", "req-2"),
		);
		expect(seamCalls).toBe(1);
		expect(refreshCalls).toBe(1);
	});

	it("does not refresh or fire the seam when a login on the bare transport is rejected", async () => {
		server.use(
			http.post("*/api/auth/login", () =>
				errorResponse(
					401,
					"INVALID_CREDENTIALS",
					"Invalid username or password",
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
			hostsHandler(() => unauthorized(1)),
		);
		setBearerToken("expired-access-token");

		await expect(api.get("/hosts")).rejects.toMatchObject({
			response: { status: 401 },
		});
	});

	it("re-arms the seam once a new token is installed after session death", async () => {
		server.use(
			rejectedRefreshHandler(),
			hostsHandler(() => unauthorized(1)),
		);
		setSessionExpiredHandler(() => {
			seamCalls++;
		});

		setBearerToken("first-session-token");
		await expect(api.get("/hosts")).rejects.toMatchObject({
			response: { status: 401 },
		});
		expect(seamCalls).toBe(1);

		setBearerToken("second-session-token");
		await expect(api.get("/hosts")).rejects.toMatchObject({
			response: { status: 401 },
		});
		expect(seamCalls).toBe(2);
	});
});
