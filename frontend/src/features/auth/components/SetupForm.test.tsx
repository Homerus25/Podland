import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http, type JsonBodyType } from "msw";
import { afterEach, describe, expect, it } from "vitest";
import { renderApp } from "@/test/auth";
import { errorResponse } from "@/test/errors";
import { authStatusFixture } from "@/test/factories";
import { server } from "@/test/server";

// The setup form rides the auth context (adoptSession) and the router
// (navigate), so the suite renders the real app on a fresh instance
// instead of stubbing providers; only the form's endpoint is overridden.
// The redirect matrix around the form lives in App.test.tsx.

function renderFreshSetup() {
	server.use(
		http.get("*/api/auth/status", () =>
			HttpResponse.json(authStatusFixture({ needsSetup: true })),
		),
	);
	return renderApp("/setup");
}

let setupRequests: Array<Record<string, unknown>> = [];

function countSetupRequests() {
	setupRequests = [];
	server.use(
		http.post("*/api/setup/user", async ({ request }) => {
			setupRequests.push((await request.json()) as Record<string, unknown>);
			return HttpResponse.json({
				token: "setup-access-token",
				username: "admin",
				role: "admin",
			}) satisfies HttpResponse<JsonBodyType>;
		}),
	);
}

async function submit() {
	await userEvent.click(
		screen.getByRole("button", { name: /create admin account/i }),
	);
}

afterEach(() => {
	setupRequests = [];
});

describe("SetupForm", () => {
	it("shows the minimum-length hint before any input", async () => {
		renderFreshSetup();

		expect(
			await screen.findByText("At least 8 characters"),
		).toBeInTheDocument();
		expect(setupRequests).toHaveLength(0);
	});

	it("validates inline and sends nothing until the form is valid", async () => {
		renderFreshSetup();
		countSetupRequests();
		await screen.findByLabelText("Username");

		// Empty fields.
		await submit();
		expect(screen.getByText("Username is required")).toBeInTheDocument();
		expect(screen.getByText("Password is required")).toBeInTheDocument();
		expect(
			screen.getByText("Password confirmation is required"),
		).toBeInTheDocument();
		expect(setupRequests).toHaveLength(0);

		// Password below the minimum.
		await userEvent.type(screen.getByLabelText("Username"), "admin");
		await userEvent.type(screen.getByLabelText("Password"), "short");
		await userEvent.type(screen.getByLabelText("Confirm password"), "short");
		await submit();
		expect(
			screen.getByText("Password must be at least 8 characters"),
		).toBeInTheDocument();
		expect(setupRequests).toHaveLength(0);

		// Mismatched confirmation.
		await userEvent.clear(screen.getByLabelText("Password"));
		await userEvent.type(screen.getByLabelText("Password"), "admin-password");
		await userEvent.clear(screen.getByLabelText("Confirm password"));
		await userEvent.type(
			screen.getByLabelText("Confirm password"),
			"different-password",
		);
		await submit();
		expect(screen.getByText("Passwords do not match")).toBeInTheDocument();
		expect(setupRequests).toHaveLength(0);

		// The corrected form goes out exactly once, confirmation excluded.
		await userEvent.clear(screen.getByLabelText("Confirm password"));
		await userEvent.type(
			screen.getByLabelText("Confirm password"),
			"admin-password",
		);
		await submit();
		expect(
			await screen.findByRole("heading", { name: "Dashboard" }),
		).toBeInTheDocument();
		expect(setupRequests).toEqual([
			{ username: "admin", password: "admin-password" },
		]);
	});

	it("surfaces a server validation error inline and stays on the form", async () => {
		renderFreshSetup();
		server.use(
			http.post("*/api/setup/user", () =>
				errorResponse(
					400,
					"PASSWORD_TOO_SHORT",
					"Password must be at least 8 characters",
				),
			),
		);
		await screen.findByLabelText("Username");

		await userEvent.type(screen.getByLabelText("Username"), "admin");
		await userEvent.type(screen.getByLabelText("Password"), "admin-password");
		await userEvent.type(
			screen.getByLabelText("Confirm password"),
			"admin-password",
		);
		await submit();

		expect(await screen.findByRole("alert")).toBeInTheDocument();
		expect(
			screen.getByText("Password must be at least 8 characters"),
		).toBeInTheDocument();
		expect(screen.getByLabelText("Username")).toHaveValue("admin");
		expect(screen.getByTestId("location")).toHaveTextContent("/setup");
	});
});
