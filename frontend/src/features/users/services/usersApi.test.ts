import {
	changeUserRole,
	listUsers,
	resetUserPassword,
} from "@features/users/services/usersApi";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import { userFixture } from "@/test/factories";
import { server } from "@/test/server";

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

	it("puts the new password to the user's password endpoint", async () => {
		let path = "";
		let body: unknown;
		const updated = userFixture({ id: "u-2", username: "bob" });
		server.use(
			http.put("*/api/users/:id/password", async ({ request }) => {
				path = new URL(request.url).pathname;
				body = await request.json();
				return HttpResponse.json(updated);
			}),
		);

		await expect(
			resetUserPassword({ id: "u-2", newPassword: "fresh-secret-1" }),
		).resolves.toEqual(updated);
		expect(path).toBe("/api/users/u-2/password");
		expect(body).toEqual({ newPassword: "fresh-secret-1" });
	});
});
