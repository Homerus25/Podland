import { REMEMBER_DEVICE_STORAGE_KEY } from "@features/auth/components/LoginForm";
import { api, setBearerToken } from "@shared/utils/api";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { afterEach, describe, expect, it } from "vitest";
import { loginViaForm, renderApp } from "@/test/auth";
import { errorResponse } from "@/test/errors";
import { server } from "@/test/server";

afterEach(() => {
	setBearerToken(null);
	localStorage.removeItem(REMEMBER_DEVICE_STORAGE_KEY);
});

describe("LoginPage", () => {
	it("logs the user in and lands on the placeholder dashboard", async () => {
		renderApp("/login");

		await loginViaForm();

		await waitFor(() => {
			expect(
				screen.getByText("Manage Podman on remote hosts"),
			).toBeInTheDocument();
		});
	});

	it("stores username and memory-only token, and sends the Bearer header on API requests", async () => {
		let authHeader: string | null = null;
		server.use(
			http.get("*/api/hosts", ({ request }) => {
				authHeader = request.headers.get("authorization");
				return HttpResponse.json([]);
			}),
		);
		renderApp("/login");

		await loginViaForm();
		await waitFor(() => {
			expect(screen.getByTestId("auth-user")).toHaveTextContent("alice");
		});

		await api.get("/hosts");

		expect(authHeader).toBe("Bearer test-access-token");

		// Color-mode preference may live in storage, but no auth material may.
		const storedValues = [
			...Object.values(localStorage),
			...Object.values(sessionStorage),
		];
		expect(
			storedValues.some((value) => value.includes("test-access-token")),
		).toBe(false);
		expect(storedValues.some((value) => value.includes("alice"))).toBe(false);
	});

	it("shows the error envelope inline when credentials are rejected", async () => {
		server.use(
			http.post("*/api/auth/login", () =>
				errorResponse(
					401,
					"INVALID_CREDENTIALS",
					"Invalid username or password",
				),
			),
		);
		renderApp("/login");

		await loginViaForm();

		expect(
			await screen.findByText("Invalid username or password"),
		).toBeInTheDocument();
		expect(screen.getByText("INVALID_CREDENTIALS")).toBeInTheDocument();
		expect(
			screen.queryByText("Manage Podman on remote hosts"),
		).not.toBeInTheDocument();
	});

	it("substitutes generic text for internal errors, hiding leaked details", async () => {
		server.use(
			http.post("*/api/auth/login", () =>
				errorResponse(500, "PANIC", "database is on fire"),
			),
		);
		renderApp("/login");

		await loginViaForm();

		expect(
			await screen.findByText("An unexpected error occurred"),
		).toBeInTheDocument();
		expect(screen.getByText("INTERNAL_ERROR")).toBeInTheDocument();
		expect(screen.queryByText("database is on fire")).not.toBeInTheDocument();
	});

	it("shows inline validation errors and never submits when fields are empty", async () => {
		renderApp("/login");

		await userEvent.click(
			await screen.findByRole("button", { name: /log in/i }),
		);

		expect(await screen.findByText("Username is required")).toBeInTheDocument();
		expect(screen.getByText("Password is required")).toBeInTheDocument();
		expect(
			screen.queryByText("Manage Podman on remote hosts"),
		).not.toBeInTheDocument();
	});

	it("treats whitespace-only credentials as empty", async () => {
		renderApp("/login");

		await loginViaForm("   ", "   ");

		expect(await screen.findByText("Username is required")).toBeInTheDocument();
		expect(screen.getByText("Password is required")).toBeInTheDocument();
	});

	it("renders the remember-device checkbox unchecked by default", async () => {
		renderApp("/login");

		const checkbox = await screen.findByRole("checkbox", {
			name: /remember this device/i,
		});

		expect(checkbox).not.toBeChecked();
	});

	it("sends rememberDevice true and persists the choice when the box is checked", async () => {
		let body: unknown;
		server.use(
			http.post("*/api/auth/login", async ({ request }) => {
				body = await request.json();
				return HttpResponse.json({
					token: "test-access-token",
					username: "alice",
					role: "user",
				});
			}),
		);
		renderApp("/login");

		await userEvent.click(
			await screen.findByRole("checkbox", { name: /remember this device/i }),
		);
		await loginViaForm();

		expect(body).toEqual({
			username: "alice",
			password: "wonderland",
			rememberDevice: true,
		});
		expect(localStorage.getItem(REMEMBER_DEVICE_STORAGE_KEY)).toBe("true");
	});

	it("sends rememberDevice false and persists the choice when the box is unchecked", async () => {
		let body: unknown;
		server.use(
			http.post("*/api/auth/login", async ({ request }) => {
				body = await request.json();
				return HttpResponse.json({
					token: "test-access-token",
					username: "alice",
					role: "user",
				});
			}),
		);
		renderApp("/login");

		await loginViaForm();

		expect(body).toEqual({
			username: "alice",
			password: "wonderland",
			rememberDevice: false,
		});
		expect(localStorage.getItem(REMEMBER_DEVICE_STORAGE_KEY)).toBe("false");
	});

	it("does not persist the remember-device choice when login fails", async () => {
		localStorage.setItem(REMEMBER_DEVICE_STORAGE_KEY, "true");
		server.use(
			http.post("*/api/auth/login", () =>
				errorResponse(
					401,
					"INVALID_CREDENTIALS",
					"Invalid username or password",
				),
			),
		);
		renderApp("/login");

		// The box starts checked from the stored choice; the user unchecks
		// it, but the login fails — the stored preference must survive.
		await userEvent.click(
			await screen.findByRole("checkbox", { name: /remember this device/i }),
		);
		await loginViaForm();

		expect(
			await screen.findByText("Invalid username or password"),
		).toBeInTheDocument();
		expect(localStorage.getItem(REMEMBER_DEVICE_STORAGE_KEY)).toBe("true");
	});

	it("pre-fills the remember-device checkbox from the persisted choice", async () => {
		localStorage.setItem(REMEMBER_DEVICE_STORAGE_KEY, "true");
		renderApp("/login");

		const checkbox = await screen.findByRole("checkbox", {
			name: /remember this device/i,
		});

		expect(checkbox).toBeChecked();
	});
});
