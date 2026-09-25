import type { User } from "@shared/types/api";
import { getBearerToken, setBearerToken } from "@shared/utils/api";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { renderApp } from "@/test/auth";
import { errorResponse } from "@/test/errors";
import { userFixture } from "@/test/factories";
import { server } from "@/test/server";

const adminSession: User = { username: "alice", role: "admin" };
const userSession: User = { username: "alice", role: "user" };

function renderUsersApp(
	at = "/",
	session: User | null = adminSession,
): ReturnType<typeof renderApp> {
	return renderApp(at, {
		resolveInitialState: async () => session,
		usernameProbe: false,
	});
}

afterEach(() => {
	setBearerToken(null);
});

// One handler set for every dialog suite: the directory list plus the
// three admin mutations, each recording the bodies it received so tests
// assert the wire shape. Create and role-change update the served list
// the way the backend would, so a successful dialog's invalidation
// refetch shows the post-mutation directory.
function mockUsersApi(
	initial = [
		userFixture({ id: "u-1", username: "alice", role: "admin" }),
		userFixture({ id: "u-2", username: "bob", role: "user" }),
	],
) {
	let users = initial;
	const createdBodies: unknown[] = [];
	const roleBodies: { id: string; role: string }[] = [];
	const passwordBodies: {
		id: string;
		adminPassword: string;
		newPassword: string;
	}[] = [];
	let listRequests = 0;
	server.use(
		http.get("*/api/users", () => {
			listRequests += 1;
			return HttpResponse.json(users);
		}),
		http.post("*/api/users", async ({ request }) => {
			const body = (await request.json()) as {
				username: string;
				role: string;
			};
			createdBodies.push(body);
			users = [
				...users,
				userFixture({
					id: `u-${users.length + 1}`,
					username: body.username,
					role: body.role as User["role"],
					createdAt: "2026-09-21T12:00:00Z",
				}),
			];
			return HttpResponse.json(users[users.length - 1], { status: 201 });
		}),
		http.put("*/api/users/:id/role", async ({ request, params }) => {
			const body = (await request.json()) as { role: string };
			roleBodies.push({ id: params.id as string, role: body.role });
			users = users.map((user) =>
				user.id === params.id
					? { ...user, role: body.role as User["role"] }
					: user,
			);
			return HttpResponse.json(users.find((user) => user.id === params.id));
		}),
		http.put("*/api/users/:id/password", async ({ request, params }) => {
			const body = (await request.json()) as {
				adminPassword: string;
				newPassword: string;
			};
			passwordBodies.push({ id: params.id as string, ...body });
			return HttpResponse.json(users.find((user) => user.id === params.id));
		}),
	);
	return {
		createdBodies,
		roleBodies,
		passwordBodies,
		listCount: () => listRequests,
	};
}

// Opens a row-scoped dialog: find the user's row by its testid, click the
// row button, wait for the modal.
async function openRowDialog(username: string, buttonTestId: string) {
	const table = within(await screen.findByTestId("users-table"));
	await userEvent.click(
		within(table.getByTestId(`user-row-${username}`)).getByTestId(buttonTestId),
	);
	expect(await screen.findByTestId("modal")).toBeInTheDocument();
}

describe("admin gating", () => {
	it("shows the Users nav entry to an admin", async () => {
		renderUsersApp("/");

		expect(
			await screen.findByRole("heading", { name: "Dashboard" }),
		).toBeInTheDocument();
		expect(screen.getByRole("link", { name: "Users" })).toBeInTheDocument();
	});

	it("hides the Users nav entry from a non-admin", async () => {
		renderUsersApp("/", userSession);

		expect(
			await screen.findByRole("heading", { name: "Dashboard" }),
		).toBeInTheDocument();
		expect(
			screen.queryByRole("link", { name: "Users" }),
		).not.toBeInTheDocument();
	});

	it("redirects a non-admin hitting /users back home without rendering the page", async () => {
		server.use(http.get("*/api/users", () => HttpResponse.json([])));

		renderUsersApp("/users", userSession);

		expect(
			await screen.findByRole("heading", { name: "Dashboard" }),
		).toBeInTheDocument();
		expect(screen.getByTestId("location")).toHaveTextContent("/");
		expect(
			screen.queryByRole("heading", { name: "Users" }),
		).not.toBeInTheDocument();
		expect(screen.queryByTestId("users-table")).not.toBeInTheDocument();
	});
});

describe("users list", () => {
	it("keeps the toolbar above the loading state while the directory fetches", async () => {
		let resolveUsers!: (value: unknown) => void;
		server.use(
			http.get("*/api/users", () =>
				new Promise((resolve) => {
					resolveUsers = resolve;
				}).then(() => HttpResponse.json([])),
			),
		);
		renderUsersApp("/users");

		expect(await screen.findByTestId("users-loading")).toBeInTheDocument();
		// The triad is inline: the Add user control stays reachable while
		// loading, on error, and when empty.
		expect(screen.getByTestId("add-user-button")).toBeInTheDocument();

		resolveUsers(HttpResponse.json([]));
		expect(await screen.findByTestId("users-empty")).toHaveTextContent(
			"No users yet.",
		);
	});

	it("lists all users with username, role, and created", async () => {
		server.use(
			http.get("*/api/users", () =>
				HttpResponse.json([
					userFixture({
						id: "u-1",
						username: "alice",
						role: "admin",
						createdAt: "2026-08-20T10:00:00Z",
					}),
					userFixture({
						id: "u-2",
						username: "bob",
						role: "user",
						createdAt: "2026-09-20T10:00:00Z",
					}),
				]),
			),
		);
		renderUsersApp("/users");

		expect(
			await screen.findByRole("heading", { name: "Users" }),
		).toBeInTheDocument();
		const table = within(await screen.findByTestId("users-table"));

		expect(
			within(table.getByTestId("user-row-alice")).getByTestId("user-role"),
		).toHaveTextContent("admin");
		const bobRow = table.getByTestId("user-row-bob");
		expect(within(bobRow).getByTestId("user-role")).toHaveTextContent("user");
		expect(within(bobRow).getAllByRole("cell")).toHaveLength(4);
	});
});

describe("add-user dialog", () => {
	async function openAddUserDialog() {
		await userEvent.click(await screen.findByTestId("add-user-button"));
		expect(await screen.findByTestId("modal")).toBeInTheDocument();
	}

	it("creates a user with the chosen role and refreshes the list", async () => {
		const { createdBodies } = mockUsersApi();
		renderUsersApp("/users");
		await openAddUserDialog();

		// The role select defaults to "user" — promotion is the deliberate act.
		expect(screen.getByTestId("user-role-select")).toHaveValue("user");
		await userEvent.type(screen.getByLabelText(/^username/i), "carol");
		await userEvent.type(screen.getByLabelText(/^password/i), "carol-secret");
		await userEvent.click(screen.getByTestId("user-form-submit"));

		expect(createdBodies).toEqual([
			{ username: "carol", password: "carol-secret", role: "user" },
		]);
		// Close-on-success, then the invalidated users query refetches and
		// the created user joins the table.
		expect(await screen.findByText("carol")).toBeInTheDocument();
		expect(screen.queryByTestId("modal")).not.toBeInTheDocument();
	});

	it("sends role admin when the select is changed", async () => {
		const { createdBodies } = mockUsersApi();
		renderUsersApp("/users");
		await openAddUserDialog();

		await userEvent.type(screen.getByLabelText(/^username/i), "root");
		await userEvent.type(screen.getByLabelText(/^password/i), "root-secret-1");
		await userEvent.selectOptions(
			screen.getByTestId("user-role-select"),
			"admin",
		);
		await userEvent.click(screen.getByTestId("user-form-submit"));

		expect(createdBodies).toEqual([
			{ username: "root", password: "root-secret-1", role: "admin" },
		]);
	});

	it("rejects a too-short password client-side without sending a request", async () => {
		const { createdBodies } = mockUsersApi();
		renderUsersApp("/users");
		await openAddUserDialog();

		await userEvent.type(screen.getByLabelText(/^username/i), "carol");
		await userEvent.type(screen.getByLabelText(/^password/i), "short");
		expect(
			screen.getByText("Password must be at least 8 characters"),
		).toBeInTheDocument();
		expect(screen.getByTestId("user-form-submit")).toBeDisabled();
		expect(createdBodies).toHaveLength(0);
	});

	it("surfaces a username-taken rejection inside the dialog and stays open", async () => {
		mockUsersApi();
		server.use(
			http.post("*/api/users", () =>
				errorResponse(409, "USERNAME_TAKEN", "Username already exists"),
			),
		);
		renderUsersApp("/users");
		await openAddUserDialog();

		await userEvent.type(screen.getByLabelText(/^username/i), "alice");
		await userEvent.type(screen.getByLabelText(/^password/i), "alice-secret");
		await userEvent.click(screen.getByTestId("user-form-submit"));

		expect(
			await screen.findByTestId("mutation-dialog-error"),
		).toHaveTextContent("Username already exists");
		expect(screen.getByTestId("modal")).toBeInTheDocument();
	});
});

describe("change-role dialog", () => {
	it("opens with the current role selected, changes it, and refreshes the list", async () => {
		const { roleBodies } = mockUsersApi();
		renderUsersApp("/users");
		await openRowDialog("bob", "change-role-button");

		// The select starts at the target's current role; the dialog is named
		// for the target user.
		expect(
			screen.getByRole("dialog", { name: "Change role — bob" }),
		).toBeInTheDocument();
		expect(screen.getByTestId("change-role-select")).toHaveValue("user");
		await userEvent.selectOptions(
			screen.getByTestId("change-role-select"),
			"admin",
		);
		await userEvent.click(screen.getByTestId("change-role-submit"));

		// Close-on-success, then the invalidated users query refetches and
		// bob's row shows the promoted role. Awaiting the refetch first also
		// proves the PUT completed before the body assertion below.
		const bobRow = screen.getByTestId("user-row-bob");
		expect(await within(bobRow).findByTestId("user-role")).toHaveTextContent(
			"admin",
		);
		expect(screen.queryByTestId("modal")).not.toBeInTheDocument();
		expect(roleBodies).toEqual([{ id: "u-2", role: "admin" }]);
	});

	it("surfaces the own-role rejection inside the dialog with its message", async () => {
		mockUsersApi();
		server.use(
			http.put("*/api/users/:id/role", () =>
				errorResponse(400, "SELF_ROLE_CHANGE", "Cannot change your own role"),
			),
		);
		renderUsersApp("/users");
		await openRowDialog("alice", "change-role-button");

		await userEvent.click(screen.getByTestId("change-role-submit"));

		expect(
			await screen.findByTestId("mutation-dialog-error"),
		).toHaveTextContent("Cannot change your own role");
		expect(screen.getByTestId("modal")).toBeInTheDocument();
	});
});

describe("reset-password dialog", () => {
	// The reset re-authenticates the acting admin, so the dialog tests run
	// with a live session bearer: a wrong admin password (401) must read as
	// an input error, and only a fired session-death seam would clear it.
	beforeEach(() => {
		setBearerToken("current-access-token");
	});

	async function fillResetForm(adminPassword: string, newPassword: string) {
		// Regex prefixes: the required indicator (an appended asterisk) is
		// part of the label text.
		await userEvent.type(
			screen.getByLabelText(/^Your password/),
			adminPassword,
		);
		await userEvent.type(screen.getByLabelText(/^New password/), newPassword);
	}

	it("resets the target's password, refreshes the list, and toasts the forced re-login", async () => {
		const { passwordBodies, listCount } = mockUsersApi();
		renderUsersApp("/users");
		await openRowDialog("bob", "reset-password-button");

		// The dialog is named for the target user and both fields start
		// empty — the admin's own password above the target's new one.
		expect(
			screen.getByRole("dialog", { name: "Reset password — bob" }),
		).toBeInTheDocument();
		const adminField = screen.getByLabelText(/^Your password/);
		expect(adminField).toHaveValue("");
		expect(screen.getByLabelText(/^new password/i)).toHaveValue("");
		expect(
			adminField.compareDocumentPosition(
				screen.getByLabelText(/^New password/),
			) & Node.DOCUMENT_POSITION_FOLLOWING,
		).not.toBe(0);
		await fillResetForm("alice-admin-secret", "bob-fresh-secret");
		const requestsBefore = listCount();
		await userEvent.click(screen.getByTestId("reset-password-submit"));

		// Close-on-success, then the invalidated users query refetches; the
		// toast names the target so the admin knows to hand over credentials.
		expect(await screen.findByText(/bob must sign in/i)).toBeInTheDocument();
		expect(screen.queryByTestId("modal")).not.toBeInTheDocument();
		await waitFor(() => expect(listCount()).toBeGreaterThan(requestsBefore));
		expect(passwordBodies).toEqual([
			{
				id: "u-2",
				adminPassword: "alice-admin-secret",
				newPassword: "bob-fresh-secret",
			},
		]);
	});

	it("rejects a too-short password client-side without sending a request", async () => {
		const { passwordBodies } = mockUsersApi();
		renderUsersApp("/users");
		await openRowDialog("bob", "reset-password-button");

		await fillResetForm("alice-admin-secret", "short");
		expect(
			screen.getByText("Password must be at least 8 characters"),
		).toBeInTheDocument();
		expect(screen.getByTestId("reset-password-submit")).toBeDisabled();
		expect(passwordBodies).toHaveLength(0);
	});

	it("surfaces a wrong admin password in the dialog and keeps the session", async () => {
		mockUsersApi();
		server.use(
			http.put("*/api/users/:id/password", () =>
				errorResponse(
					401,
					"INVALID_CREDENTIALS",
					"Current password is incorrect",
				),
			),
		);
		renderUsersApp("/users");
		await openRowDialog("bob", "reset-password-button");

		await fillResetForm("wrong-admin-secret", "bob-fresh-secret");
		await userEvent.click(screen.getByTestId("reset-password-submit"));

		expect(
			await screen.findByTestId("mutation-dialog-error"),
		).toHaveTextContent("Current password is incorrect");
		expect(screen.getByTestId("modal")).toBeInTheDocument();
		// The failed re-auth is an input error, not session death: the app
		// stays on the page with the session holder untouched.
		expect(screen.getByTestId("location")).toHaveTextContent("/users");
		expect(screen.getByTestId("account-menu-button")).toBeInTheDocument();
		expect(getBearerToken()).toBe("current-access-token");
	});

	it("surfaces the spent failure budget in the dialog error region", async () => {
		mockUsersApi();
		server.use(
			http.put("*/api/users/:id/password", () =>
				errorResponse(429, "RATE_LIMIT_EXCEEDED", "Too many requests"),
			),
		);
		renderUsersApp("/users");
		await openRowDialog("bob", "reset-password-button");

		await fillResetForm("alice-admin-secret", "bob-fresh-secret");
		await userEvent.click(screen.getByTestId("reset-password-submit"));

		expect(
			await screen.findByTestId("mutation-dialog-error"),
		).toHaveTextContent("Too many requests");
		expect(screen.getByTestId("modal")).toBeInTheDocument();
	});

	it("surfaces the self-reset guard in the dialog error region", async () => {
		mockUsersApi();
		server.use(
			http.put("*/api/users/:id/password", () =>
				errorResponse(
					400,
					"SELF_PASSWORD_RESET",
					"Cannot reset your own password",
				),
			),
		);
		renderUsersApp("/users");
		await openRowDialog("alice", "reset-password-button");

		await fillResetForm("alice-admin-secret", "another-secret-1");
		await userEvent.click(screen.getByTestId("reset-password-submit"));

		expect(
			await screen.findByTestId("mutation-dialog-error"),
		).toHaveTextContent("Cannot reset your own password");
		expect(screen.getByTestId("modal")).toBeInTheDocument();
	});

	it("surfaces a reset rejection inside the dialog and stays open", async () => {
		mockUsersApi();
		server.use(
			http.put("*/api/users/:id/password", () =>
				errorResponse(404, "USER_NOT_FOUND", "User not found"),
			),
		);
		renderUsersApp("/users");
		await openRowDialog("bob", "reset-password-button");

		await fillResetForm("alice-admin-secret", "bob-fresh-secret");
		await userEvent.click(screen.getByTestId("reset-password-submit"));

		expect(
			await screen.findByTestId("mutation-dialog-error"),
		).toHaveTextContent("User not found");
		expect(screen.getByTestId("modal")).toBeInTheDocument();
	});
});
