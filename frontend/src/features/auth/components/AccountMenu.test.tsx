import { getBearerToken, setBearerToken } from "@shared/utils/api";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http, type JsonBodyType } from "msw";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { renderApp } from "@/test/auth";
import { errorResponse } from "@/test/errors";
import { server } from "@/test/server";

// The account menu, its change-password dialog, and the session adoption
// are one flow over the real app shell, so the suite renders the full app
// with a restored session instead of stubbing providers. The dashboard
// route keeps every fetch in the flow auth-related.

function renderSignedIn(username = "alice") {
	return renderApp("/", {
		resolveInitialState: async () => ({ username, role: "user" }),
		usernameProbe: false,
	});
}

async function openAccountMenu() {
	await userEvent.click(await screen.findByTestId("account-menu-button"));
	await screen.findByRole("menuitem", { name: "Change password" });
}

let passwordRequests: Array<{
	authorization: string | null;
	body: Record<string, unknown>;
}> = [];

function mockChangePassword(respond: () => HttpResponse<JsonBodyType>): void {
	passwordRequests = [];
	server.use(
		http.post("*/api/auth/password", async ({ request }) => {
			passwordRequests.push({
				authorization: request.headers.get("authorization"),
				body: (await request.json()) as Record<string, unknown>,
			});
			return respond();
		}),
	);
}

async function fillAndSubmit(
	currentPassword: string,
	newPassword: string,
): Promise<void> {
	// Regex prefixes: the required indicator (an appended asterisk) is part
	// of the label text.
	await userEvent.type(
		screen.getByLabelText(/^Current password/),
		currentPassword,
	);
	await userEvent.type(screen.getByLabelText(/^New password/), newPassword);
	await userEvent.click(screen.getByTestId("change-password-submit"));
}

beforeEach(() => {
	setBearerToken("current-access-token");
});

afterEach(() => {
	setBearerToken(null);
});

describe("account menu", () => {
	it("opens from the header username with the change-password and logout entries", async () => {
		renderSignedIn();

		await openAccountMenu();

		expect(
			screen.getByRole("menuitem", { name: "Log out" }),
		).toBeInTheDocument();
		expect(
			screen.queryByRole("button", { name: "Log out" }),
		).not.toBeInTheDocument();
	});

	it("logs out from the menu and lands on the login page", async () => {
		renderSignedIn();

		await openAccountMenu();
		await userEvent.click(screen.getByRole("menuitem", { name: "Log out" }));

		expect(await screen.findByLabelText(/username/i)).toBeInTheDocument();
		expect(screen.getByTestId("location")).toHaveTextContent("/login");
	});

	it("opens the change-password dialog with current and new password fields", async () => {
		renderSignedIn();

		await openAccountMenu();
		await userEvent.click(
			screen.getByRole("menuitem", { name: "Change password" }),
		);

		expect(await screen.findByTestId("modal")).toBeInTheDocument();
		expect(screen.getByLabelText(/^Current password/)).toBeInTheDocument();
		expect(screen.getByLabelText(/^New password/)).toBeInTheDocument();
	});
});

describe("change-password dialog", () => {
	it("rejects a short new password client-side without sending a request", async () => {
		renderSignedIn();
		await openAccountMenu();
		await userEvent.click(
			screen.getByRole("menuitem", { name: "Change password" }),
		);
		await screen.findByTestId("modal");
		mockChangePassword(() =>
			HttpResponse.json({ token: "x", username: "a", role: "user" }),
		);

		await userEvent.type(
			screen.getByLabelText(/^Current password/),
			"old-secret",
		);
		await userEvent.type(screen.getByLabelText(/^New password/), "short");

		expect(
			screen.getByText("Password must be at least 8 characters"),
		).toBeInTheDocument();
		expect(screen.getByTestId("change-password-submit")).toBeDisabled();
		expect(passwordRequests).toHaveLength(0);
	});

	it("sends the current and new password with the session bearer and adopts the re-issued session", async () => {
		renderSignedIn();
		await openAccountMenu();
		await userEvent.click(
			screen.getByRole("menuitem", { name: "Change password" }),
		);
		await screen.findByTestId("modal");
		mockChangePassword(() =>
			HttpResponse.json({
				token: "re-issued-access-token",
				username: "alice",
				role: "user",
			}),
		);

		await fillAndSubmit("old-secret", "new-secret-1");

		await waitFor(() =>
			expect(screen.queryByTestId("modal")).not.toBeInTheDocument(),
		);
		expect(passwordRequests).toHaveLength(1);
		expect(passwordRequests[0]).toEqual({
			authorization: "Bearer current-access-token",
			body: { currentPassword: "old-secret", newPassword: "new-secret-1" },
		});
		// The re-issued session is adopted: the fresh token is installed and
		// the user stays on the page — no forced re-login on this device.
		expect(getBearerToken()).toBe("re-issued-access-token");
		expect(screen.getByTestId("location")).toHaveTextContent("/");
		expect(screen.getByTestId("account-menu-button")).toBeInTheDocument();
	});

	it("surfaces a wrong current password in the dialog and keeps the session", async () => {
		renderSignedIn();
		await openAccountMenu();
		await userEvent.click(
			screen.getByRole("menuitem", { name: "Change password" }),
		);
		await screen.findByTestId("modal");
		mockChangePassword(() =>
			errorResponse(
				401,
				"INVALID_CREDENTIALS",
				"Current password is incorrect",
			),
		);

		await fillAndSubmit("wrong-secret", "new-secret-1");

		expect(await screen.findByRole("alert")).toBeInTheDocument();
		expect(
			screen.getByText("Current password is incorrect"),
		).toBeInTheDocument();
		// The failed change is an input error, not session death: the dialog
		// stays open and the app does not navigate to the login page.
		expect(screen.getByTestId("modal")).toBeInTheDocument();
		expect(screen.getByTestId("location")).toHaveTextContent("/");
		expect(screen.getByTestId("account-menu-button")).toBeInTheDocument();
	});
});
