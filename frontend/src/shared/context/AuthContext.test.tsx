import { useAuth } from "@features/auth/hooks/useAuth";
import { AuthProvider } from "@shared/context/AuthContext";
import { hostsQueryKey } from "@shared/queries/queryKeys";
import type { User } from "@shared/types/api";
import {
	getBearerToken,
	handleUnauthorized,
	setBearerToken,
} from "@shared/utils/api";
import { act, waitFor } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import { afterEach, describe, expect, it } from "vitest";
import { errorResponse } from "@/test/errors";
import { authStatusFixture, hostFixture } from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

// AuthProvider must sit above the hook, and renderHookWithQuery's wrapper
// is query-only, so the hook value surfaces through a probe under the
// provider while the harness owns the QueryClientProvider.
function renderAuth(
	options: { resolveInitialState?: () => Promise<User | null> } = {},
) {
	let current: ReturnType<typeof useAuth>;
	function AuthProbe() {
		current = useAuth();
		return null;
	}
	const utils = renderWithQuery(
		<AuthProvider
			resolveInitialState={options.resolveInitialState ?? (async () => null)}
		>
			<AuthProbe />
		</AuthProvider>,
	);
	return {
		result: {
			get current() {
				return current;
			},
		},
		queryClient: utils.queryClient,
		unmount: utils.unmount,
	};
}

afterEach(() => {
	setBearerToken(null);
});

describe("logout and the query cache", () => {
	it("clears the query cache alongside the local state drop", async () => {
		const { result, queryClient } = renderAuth({
			resolveInitialState: async () => ({ username: "alice", role: "user" }),
		});
		queryClient.setQueryData(hostsQueryKey, [
			hostFixture({ name: "alice-host" }),
		]);
		await waitFor(() => expect(result.current.isAuthenticated).toBe(true));
		expect(queryClient.getQueryCache().getAll()).toHaveLength(1);

		await act(async () => {
			await result.current.logout();
		});

		expect(result.current.user).toBeNull();
		expect(queryClient.getQueryCache().getAll()).toHaveLength(0);
	});

	it("does not clear the query cache on login", async () => {
		const { result, queryClient } = renderAuth();
		queryClient.setQueryData(hostsQueryKey, [hostFixture()]);
		await waitFor(() => expect(result.current.isResolving).toBe(false));
		expect(queryClient.getQueryCache().getAll()).toHaveLength(1);

		await act(async () => {
			await result.current.login({ username: "bob", password: "builder" });
		});

		expect(result.current.user?.username).toBe("bob");
		expect(queryClient.getQueryCache().getAll()).toHaveLength(1);
	});
});

describe("adoptSession", () => {
	it("installs the re-issued token and user without any request or cache clear", async () => {
		let refreshCalled = false;
		server.use(
			http.post("*/api/auth/refresh", () => {
				refreshCalled = true;
				return HttpResponse.json({ token: "x", username: "ada" });
			}),
		);
		const { result, queryClient } = renderAuth({
			resolveInitialState: async () => ({ username: "alice", role: "user" }),
		});
		queryClient.setQueryData(hostsQueryKey, [hostFixture()]);
		await waitFor(() => expect(result.current.isAuthenticated).toBe(true));
		expect(queryClient.getQueryCache().getAll()).toHaveLength(1);

		await act(async () => {
			result.current.adoptSession({
				token: "re-issued-access-token",
				username: "alice",
				role: "user",
			});
		});

		// The re-issued login is adopted in place: fresh token installed,
		// user taken from the payload, query cache untouched, no request.
		expect(result.current.user?.username).toBe("alice");
		expect(getBearerToken()).toBe("re-issued-access-token");
		expect(queryClient.getQueryCache().getAll()).toHaveLength(1);
		expect(refreshCalled).toBe(false);
	});
});

describe("boot probe", () => {
	function statusProbe(needsSetup: boolean, calls: string[] = []) {
		return http.get("*/api/auth/status", ({ request }) => {
			calls.push(request.headers.get("authorization") ?? "no-header");
			return HttpResponse.json(authStatusFixture({ needsSetup }));
		});
	}

	it("asks the status endpoint once when the restore lands logged-out and reports the flag", async () => {
		const statusCalls: string[] = [];
		server.use(statusProbe(true, statusCalls));
		const { result } = renderAuth();

		await waitFor(() => expect(result.current.isResolving).toBe(false));

		expect(result.current.needsSetup).toBe(true);
		expect(statusCalls).toEqual(["no-header"]);
	});

	it("reads a configured instance as not needing setup", async () => {
		const statusCalls: string[] = [];
		server.use(statusProbe(false, statusCalls));
		const { result } = renderAuth();

		await waitFor(() => expect(result.current.isResolving).toBe(false));

		expect(result.current.needsSetup).toBe(false);
		expect(statusCalls).toHaveLength(1);
	});

	it("skips the probe when the restore resolves a session", async () => {
		const statusCalls: string[] = [];
		server.use(statusProbe(true, statusCalls));
		const { result } = renderAuth({
			resolveInitialState: async () => ({ username: "alice", role: "user" }),
		});

		await waitFor(() => expect(result.current.isResolving).toBe(false));

		expect(result.current.needsSetup).toBe(false);
		expect(statusCalls).toHaveLength(0);
	});

	it("treats a failed probe as a configured instance, not an error", async () => {
		server.use(
			http.get("*/api/auth/status", () =>
				errorResponse(500, "INTERNAL_ERROR", "database unreachable"),
			),
		);
		const { result } = renderAuth();

		await waitFor(() => expect(result.current.isResolving).toBe(false));

		expect(result.current.needsSetup).toBe(false);
		expect(result.current.isAuthenticated).toBe(false);
	});

	it("clears the flag when a session is adopted", async () => {
		server.use(statusProbe(true));
		const { result } = renderAuth();
		await waitFor(() => expect(result.current.needsSetup).toBe(true));

		await act(async () => {
			result.current.adoptSession({
				token: "setup-access-token",
				username: "admin",
				role: "admin",
			});
		});

		expect(result.current.needsSetup).toBe(false);
		expect(result.current.isAuthenticated).toBe(true);
	});

	it("clears the flag when the parked form's 409 reports setup completed", async () => {
		server.use(statusProbe(true));
		const { result } = renderAuth();
		await waitFor(() => expect(result.current.needsSetup).toBe(true));

		act(() => {
			result.current.markConfigured();
		});

		expect(result.current.needsSetup).toBe(false);
	});
});

describe("session-expired seam", () => {
	function logoutCountingHandler(logoutCalls: string[]) {
		return http.post("*/api/auth/logout", () => {
			logoutCalls.push("logout");
			return HttpResponse.json({ message: "Logged out successfully" });
		});
	}

	function rejectedRefreshHandler() {
		return http.post("*/api/auth/refresh", () =>
			errorResponse(
				401,
				"AUTH_REFRESH_INVALID",
				"Refresh token invalid",
				"req-refresh",
			),
		);
	}

	it("runs the existing logout when the policy declares the session dead", async () => {
		const logoutCalls: string[] = [];
		server.use(logoutCountingHandler(logoutCalls), rejectedRefreshHandler());
		const { result } = renderAuth({
			resolveInitialState: async () => ({ username: "alice", role: "user" }),
		});
		await waitFor(() => expect(result.current.isAuthenticated).toBe(true));
		// A live session holds a token; installing one also re-arms the
		// policy's once-latch for this module instance.
		setBearerToken("mid-session-token");

		let replayed: boolean | undefined;
		await act(async () => {
			replayed = await handleUnauthorized();
		});

		expect(replayed).toBe(false);
		expect(logoutCalls).toHaveLength(1);
		await waitFor(() => expect(result.current.user).toBeNull());
	});

	it("stops running logout once the provider unmounts", async () => {
		const logoutCalls: string[] = [];
		server.use(logoutCountingHandler(logoutCalls), rejectedRefreshHandler());
		const { result, unmount } = renderAuth({
			resolveInitialState: async () => ({ username: "alice", role: "user" }),
		});
		await waitFor(() => expect(result.current.isAuthenticated).toBe(true));
		unmount();
		setBearerToken("mid-session-token");

		await act(async () => {
			await handleUnauthorized();
		});

		expect(logoutCalls).toHaveLength(0);
	});
});
