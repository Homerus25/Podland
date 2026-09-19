import { useAuth } from "@features/auth/hooks/useAuth";
import { AuthProvider } from "@shared/context/AuthContext";
import { machinesQueryKey } from "@shared/queries/queryKeys";
import type { User } from "@shared/types/api";
import { handleUnauthorized, setBearerToken } from "@shared/utils/api";
import { act, waitFor } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import { afterEach, describe, expect, it } from "vitest";
import { errorResponse } from "@/test/errors";
import { machineFixture } from "@/test/factories";
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
			resolveInitialState: async () => ({ username: "alice" }),
		});
		queryClient.setQueryData(machinesQueryKey, [
			machineFixture({ name: "alice-machine" }),
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
		queryClient.setQueryData(machinesQueryKey, [machineFixture()]);
		await waitFor(() => expect(result.current.isResolving).toBe(false));
		expect(queryClient.getQueryCache().getAll()).toHaveLength(1);

		await act(async () => {
			await result.current.login({ username: "bob", password: "builder" });
		});

		expect(result.current.user?.username).toBe("bob");
		expect(queryClient.getQueryCache().getAll()).toHaveLength(1);
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
			resolveInitialState: async () => ({ username: "alice" }),
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
			resolveInitialState: async () => ({ username: "alice" }),
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
