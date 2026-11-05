import { containersQueryKey } from "@shared/queries/queryKeys";
import type { User } from "@shared/types/api";
import { setBearerToken } from "@shared/utils/api";
import { QueryClient } from "@tanstack/react-query";
import { act, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { afterEach, describe, expect, it } from "vitest";
import { loginViaForm, renderApp } from "@/test/auth";
import { errorResponse } from "@/test/errors";
import {
	authStatusFixture,
	containerFixture,
	hostFixture,
} from "@/test/factories";
import { server } from "@/test/server";

function deferred<T>() {
	let resolve!: (value: T) => void;
	const promise = new Promise<T>((res) => {
		resolve = res;
	});
	return { promise, resolve };
}

// A fresh instance: the probe answers needs-setup, so every logged-out
// route steers to /setup.
function freshInstanceHandler() {
	return http.get("*/api/auth/status", () =>
		HttpResponse.json(authStatusFixture({ needsSetup: true })),
	);
}

// Drives the setup form the way the first visitor would.
async function fillSetupForm(
	username = "admin",
	password = "first-admin-password",
) {
	await userEvent.type(await screen.findByLabelText("Username"), username);
	await userEvent.type(screen.getByLabelText("Password"), password);
	await userEvent.type(screen.getByLabelText("Confirm password"), password);
}

afterEach(() => {
	setBearerToken(null);
});

describe("app routing guard", () => {
	it("redirects an unauthenticated deep link to /login without rendering protected content", async () => {
		renderApp("/hosts");

		expect(await screen.findByLabelText(/username/i)).toBeInTheDocument();
		expect(
			screen.queryByRole("heading", { name: "Hosts" }),
		).not.toBeInTheDocument();
		expect(screen.getByTestId("location")).toHaveTextContent("/login");
		expect(screen.getByTestId("auth-user")).toHaveTextContent("anonymous");
	});

	it("returns the user to the originally requested page after login", async () => {
		renderApp("/hosts?tab=running");

		await screen.findByLabelText(/username/i);
		await loginViaForm();

		expect(
			await screen.findByRole("heading", { name: "Hosts" }),
		).toBeInTheDocument();
		expect(screen.getByTestId("location")).toHaveTextContent(
			"/hosts?tab=running",
		);
		expect(screen.getByTestId("auth-user")).toHaveTextContent("alice");
	});

	it("shows a full-page spinner while the initial auth state resolves, never the login form", async () => {
		const resolution = deferred<User | null>();
		renderApp("/hosts", { resolveInitialState: () => resolution.promise });

		expect(screen.getByRole("status")).toBeInTheDocument();
		expect(screen.queryByLabelText(/username/i)).not.toBeInTheDocument();
		expect(
			screen.queryByRole("heading", { name: "Hosts" }),
		).not.toBeInTheDocument();

		await act(async () => {
			resolution.resolve(null);
		});

		expect(await screen.findByLabelText(/username/i)).toBeInTheDocument();
		expect(screen.queryByRole("status")).not.toBeInTheDocument();
	});

	describe("session restore on reload", () => {
		it("restores the session from the refresh cookie on reload and stays on the deep link", async () => {
			const authHeaders: (string | null)[] = [];
			server.use(
				http.get("*/api/hosts", ({ request }) => {
					authHeaders.push(request.headers.get("Authorization"));
					return HttpResponse.json([]);
				}),
				http.post("*/api/auth/refresh", () => {
					return HttpResponse.json({
						token: "refreshed-access-token",
						username: "alice",
						role: "user",
					});
				}),
			);
			renderApp("/hosts");

			expect(
				await screen.findByRole("heading", { name: "Hosts" }),
			).toBeInTheDocument();
			expect(screen.getByTestId("location")).toHaveTextContent("/hosts");
			expect(screen.getByTestId("auth-user")).toHaveTextContent("alice");
			expect(authHeaders[0]).toBe("Bearer refreshed-access-token");
		});

		it("redirects to /login when the refresh cookie is rejected", async () => {
			server.use(
				http.post("*/api/auth/refresh", () =>
					errorResponse(401, "AUTH_REFRESH_INVALID", "Refresh token invalid"),
				),
			);
			renderApp("/hosts");

			expect(await screen.findByLabelText(/username/i)).toBeInTheDocument();
			expect(screen.getByTestId("location")).toHaveTextContent("/login");
			expect(screen.getByTestId("auth-user")).toHaveTextContent("anonymous");
		});
	});

	it("logs out: calls POST /api/auth/logout, clears auth state, and lands on /login", async () => {
		let logoutCalls = 0;
		server.use(
			http.post("*/api/auth/logout", () => {
				logoutCalls++;
				return HttpResponse.json({ message: "Logged out successfully" });
			}),
		);
		renderApp("/", {
			resolveInitialState: async () => ({ username: "alice", role: "user" }),
		});

		expect(
			await screen.findByRole("heading", { name: "Dashboard" }),
		).toBeInTheDocument();
		expect(screen.getByTestId("auth-user")).toHaveTextContent("alice");

		// Log out lives in the account menu opened from the header username.
		await userEvent.click(screen.getByTestId("account-menu-button"));
		await userEvent.click(
			await screen.findByRole("menuitem", { name: /log out/i }),
		);

		expect(await screen.findByLabelText(/username/i)).toBeInTheDocument();
		expect(
			screen.queryByRole("heading", { name: "Dashboard" }),
		).not.toBeInTheDocument();
		expect(screen.getByTestId("auth-user")).toHaveTextContent("anonymous");
		expect(logoutCalls).toBe(1);
	});

	it("still clears state and lands on /login when the revocation call fails", async () => {
		server.use(
			http.post("*/api/auth/logout", () =>
				errorResponse(500, "INTERNAL_ERROR", "session store down"),
			),
		);
		renderApp("/", {
			resolveInitialState: async () => ({ username: "alice", role: "user" }),
		});

		expect(
			await screen.findByRole("heading", { name: "Dashboard" }),
		).toBeInTheDocument();

		await userEvent.click(screen.getByTestId("account-menu-button"));
		await userEvent.click(
			await screen.findByRole("menuitem", { name: /log out/i }),
		);

		expect(await screen.findByLabelText(/username/i)).toBeInTheDocument();
		expect(screen.getByTestId("auth-user")).toHaveTextContent("anonymous");
	});

	describe("session death mid-app", () => {
		it("lands on /login with the deep link preserved and no cached data when a query 401s and refresh is rejected", async () => {
			// The live session holds a bearer; installing one also re-arms
			// the policy's once-latch for this module instance.
			setBearerToken("mid-session-token");
			server.use(
				http.get("*/api/hosts", () =>
					errorResponse(401, "TOKEN_EXPIRED", "Token has expired"),
				),
				http.post("*/api/auth/refresh", () =>
					errorResponse(
						401,
						"AUTH_REFRESH_INVALID",
						"Refresh token invalid",
						"req-2",
					),
				),
			);
			// The predecessor's cached list, seeded before mount: session
			// death must clear it, so the next identity never starts from
			// the predecessor's data.
			const queryClient = new QueryClient({
				defaultOptions: { queries: { retry: false } },
			});
			queryClient.setQueryData(containersQueryKey("m-1"), [containerFixture()]);
			renderApp("/hosts", {
				resolveInitialState: async () => ({ username: "alice", role: "user" }),
				queryClient,
			});

			expect(await screen.findByLabelText(/username/i)).toBeInTheDocument();
			expect(screen.getByTestId("location")).toHaveTextContent("/login");
			expect(
				screen.queryByRole("heading", { name: "Hosts" }),
			).not.toBeInTheDocument();
			expect(screen.getByTestId("auth-user")).toHaveTextContent("anonymous");
			// The cache clear can race the dying page's final render, which
			// re-creates its query in the cleared cache before unmounting;
			// what must never survive is predecessor data — a re-created
			// query is data-less and inert.
			expect(
				queryClient
					.getQueryCache()
					.getAll()
					.every((query) => query.state.data === undefined),
			).toBe(true);

			// Logging back in returns to the originally requested page: the
			// session-death redirect attached that location to /login's state.
			server.use(
				http.get("*/api/hosts", () => HttpResponse.json([hostFixture()])),
			);
			await loginViaForm();

			expect(
				await screen.findByRole("heading", { name: "Hosts" }),
			).toBeInTheDocument();
			expect(screen.getByTestId("location")).toHaveTextContent("/hosts");
			expect(screen.getByTestId("auth-user")).toHaveTextContent("alice");
		});
	});

	describe("first-run setup", () => {
		it("lands the root route on setup when the instance is fresh", async () => {
			server.use(freshInstanceHandler());
			renderApp("/");

			expect(
				await screen.findByText(
					"Create the first admin account for this instance.",
				),
			).toBeInTheDocument();
			expect(screen.getByTestId("location")).toHaveTextContent("/setup");
		});

		it("lands a logged-out deep link on setup when the instance is fresh", async () => {
			server.use(freshInstanceHandler());
			renderApp("/hosts?tab=running");

			expect(
				await screen.findByText(
					"Create the first admin account for this instance.",
				),
			).toBeInTheDocument();
			expect(screen.getByTestId("location")).toHaveTextContent("/setup");
			expect(screen.getByTestId("auth-user")).toHaveTextContent("anonymous");
			expect(
				screen.queryByRole("heading", { name: "Hosts" }),
			).not.toBeInTheDocument();
		});

		it("redirects the login page to setup while setup is needed", async () => {
			server.use(freshInstanceHandler());
			renderApp("/login");

			expect(
				await screen.findByLabelText("Confirm password"),
			).toBeInTheDocument();
			expect(screen.getByTestId("location")).toHaveTextContent("/setup");
		});

		it("creates the first admin, adopts the returned session, and opens the dashboard without a second login", async () => {
			let setupBody: unknown;
			let loginCalls = 0;
			server.use(
				freshInstanceHandler(),
				http.post("*/api/setup/user", async ({ request }) => {
					setupBody = await request.json();
					return HttpResponse.json({
						token: "setup-access-token",
						username: "admin",
						role: "admin",
					});
				}),
				http.post("*/api/auth/login", () => {
					loginCalls++;
					return HttpResponse.json({ token: "x", username: "x", role: "user" });
				}),
			);
			renderApp("/setup");

			await fillSetupForm();
			await userEvent.click(
				screen.getByRole("button", { name: /create admin account/i }),
			);

			expect(
				await screen.findByRole("heading", { name: "Dashboard" }),
			).toBeInTheDocument();
			expect(screen.getByTestId("location")).toHaveTextContent("/");
			expect(screen.getByTestId("auth-user")).toHaveTextContent("admin");
			expect(setupBody).toEqual({
				username: "admin",
				password: "first-admin-password",
			});
			expect(loginCalls).toBe(0);
		});

		it("redirects the setup page of a configured instance into the app", async () => {
			renderApp("/setup");

			expect(await screen.findByLabelText(/username/i)).toBeInTheDocument();
			expect(screen.getByTestId("location")).toHaveTextContent("/login");
			expect(
				screen.queryByRole("button", { name: /create admin account/i }),
			).not.toBeInTheDocument();
		});

		it("never shows the setup page to an authenticated user", async () => {
			renderApp("/setup", {
				resolveInitialState: async () => ({
					username: "alice",
					role: "admin",
				}),
			});

			expect(
				await screen.findByRole("heading", { name: "Dashboard" }),
			).toBeInTheDocument();
			expect(screen.getByTestId("location")).toHaveTextContent("/");
			expect(
				screen.queryByRole("button", { name: /create admin account/i }),
			).not.toBeInTheDocument();
		});

		it("sends a parked setup page to login when setup was completed elsewhere", async () => {
			server.use(
				freshInstanceHandler(),
				http.post("*/api/setup/user", () =>
					errorResponse(409, "SETUP_COMPLETED", "Setup already completed"),
				),
			);
			renderApp("/setup");

			await fillSetupForm();
			await userEvent.click(
				screen.getByRole("button", { name: /create admin account/i }),
			);

			// The login form itself must render: the stale needs-setup flag
			// is corrected, so the visitor is not bounced back to the dead
			// setup page.
			expect(await screen.findByLabelText(/username/i)).toBeInTheDocument();
			expect(screen.getByTestId("location")).toHaveTextContent("/login");
			expect(
				screen.queryByRole("button", { name: /create admin account/i }),
			).not.toBeInTheDocument();
		});
	});
});
