import { authApi } from "@features/auth/services/authApi";
import { setBearerToken } from "@shared/utils/api";
import { HttpResponse, http } from "msw";
import { afterEach, describe, expect, it } from "vitest";
import { server } from "@/test/server";

// A wrong password is itself a 401 INVALID_CREDENTIALS — exactly the
// response that must never be read as session death. Auth calls ride the
// bare transport, so the no-Authorization pin runs with a token
// deliberately installed: the shared request interceptor must not touch
// these calls.
describe("authApi", () => {
	afterEach(() => {
		setBearerToken(null);
	});

	it("logs in without an Authorization header and returns the token payload", async () => {
		setBearerToken("stale-access-token");
		let authorization: string | null = null;
		let body: unknown;
		server.use(
			http.post("*/api/auth/login", async ({ request }) => {
				authorization = request.headers.get("authorization");
				body = await request.json();
				return HttpResponse.json({ token: "fresh-token", username: "ada" });
			}),
		);

		const response = await authApi.login({
			username: "ada",
			password: "secret",
		});

		expect(response).toEqual({ token: "fresh-token", username: "ada" });
		expect(body).toEqual({ username: "ada", password: "secret" });
		expect(authorization).toBeNull();
	});

	it("logs out without an Authorization header", async () => {
		setBearerToken("stale-access-token");
		let authorization: string | null = null;
		server.use(
			http.post("*/api/auth/logout", ({ request }) => {
				authorization = request.headers.get("authorization");
				return HttpResponse.json({ message: "Logged out successfully" });
			}),
		);

		await authApi.logout();

		expect(authorization).toBeNull();
	});
});
