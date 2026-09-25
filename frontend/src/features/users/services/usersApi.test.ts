import {
	changeUserRole,
	listUsers,
	resetUserPassword,
} from "@features/users/services/usersApi";
import { setBearerToken } from "@shared/utils/api";
import { HttpResponse, http } from "msw";
import { afterEach, describe, expect, it } from "vitest";
import { userFixture } from "@/test/factories";
import { server } from "@/test/server";

afterEach(() => {
	setBearerToken(null);
});

describe("usersApi", () => {
	it("lists the users from the app-wide endpoint as a bare array", async () => {
		const users = [userFixture(), userFixture({ id: "u-2", username: "bob" })];
		server.use(http.get("*/api/users", () => HttpResponse.json(users)));

		await expect(listUsers()).resolves.toEqual(users);
	});

	it("puts the chosen role to the user's role endpoint", async () => {
		let path = "";
		let body: unknown;
		const updated = userFixture({
			id: "u-2",
			username: "bob",
			role: "admin",
		});
		server.use(
			http.put("*/api/users/:id/role", async ({ request }) => {
				path = new URL(request.url).pathname;
				body = await request.json();
				return HttpResponse.json(updated);
			}),
		);

		await expect(changeUserRole({ id: "u-2", role: "admin" })).resolves.toEqual(
			updated,
		);
		expect(path).toBe("/api/users/u-2/role");
		expect(body).toEqual({ role: "admin" });
	});

	// The reset is authenticated with re-authentication: the session bearer
	// rides along explicitly and both passwords travel in the body. Like the
	// change-password call it rides the bare transport, so a wrong admin
	// password (401) surfaces as an ordinary rejection instead of a refresh.
	it("puts the admin and new passwords to the user's password endpoint with the bearer header", async () => {
		setBearerToken("session-access-token");
		let path = "";
		let authorization: string | null = null;
		let body: unknown;
		const updated = userFixture({ id: "u-2", username: "bob" });
		server.use(
			http.put("*/api/users/:id/password", async ({ request }) => {
				path = new URL(request.url).pathname;
				authorization = request.headers.get("authorization");
				body = await request.json();
				return HttpResponse.json(updated);
			}),
		);

		await expect(
			resetUserPassword({
				id: "u-2",
				adminPassword: "admin-secret",
				newPassword: "fresh-secret-1",
			}),
		).resolves.toEqual(updated);
		expect(path).toBe("/api/users/u-2/password");
		expect(authorization).toBe("Bearer session-access-token");
		expect(body).toEqual({
			adminPassword: "admin-secret",
			newPassword: "fresh-secret-1",
		});
	});
});
