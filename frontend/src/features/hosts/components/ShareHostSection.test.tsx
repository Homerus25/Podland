import { ShareHostSection } from "@features/hosts/components/ShareHostSection";
import type { UserSummary } from "@features/users/services/usersApi";
import type { AccessGrant } from "@shared/types/api";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { beforeEach, describe, expect, it } from "vitest";
import { errorResponse } from "@/test/errors";
import { userFixture } from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

const defaultGrants: AccessGrant[] = [
	{ userId: "u-owner", permission: "admin", grantedAt: "2026-08-01T10:00:00Z" },
	{ userId: "u-bob", permission: "read", grantedAt: "2026-08-03T10:00:00Z" },
];

const defaultUsers: UserSummary[] = [
	userFixture({ id: "u-owner", username: "olivia" }),
	userFixture({ id: "u-bob", username: "bob" }),
	userFixture({ id: "u-carol", username: "carol" }),
	userFixture({ id: "u-dave", username: "dave" }),
];

let grants: AccessGrant[] = defaultGrants;
let users: UserSummary[] = defaultUsers;
let grantPayload: Record<string, unknown> | undefined;
let grantRequests = 0;
let revokedUserId: string | undefined;
let grantsRequests = 0;

function accessHandlers() {
	return [
		http.get("*/api/hosts/m-1/access", () => {
			grantsRequests++;
			return HttpResponse.json(grants);
		}),
		http.get("*/api/users", () => HttpResponse.json(users)),
		http.post("*/api/hosts/m-1/access", async ({ request }) => {
			grantRequests++;
			grantPayload = (await request.json()) as Record<string, unknown>;
			const granted = {
				userId: grantPayload.userId as string,
				permission: grantPayload.permission,
				grantedAt: "2026-08-05T10:00:00Z",
			};
			grants = [...grants, granted as AccessGrant];
			return HttpResponse.json(granted, { status: 201 });
		}),
		http.delete("*/api/hosts/m-1/access/:userId", ({ params }) => {
			revokedUserId = params.userId as string;
			grants = grants.filter((grant) => grant.userId !== revokedUserId);
			return new HttpResponse(null, { status: 204 });
		}),
	];
}

function renderShareHostSection() {
	renderWithQuery(<ShareHostSection hostId="m-1" ownerId="u-owner" />);
}

async function renderWithGrants() {
	server.use(...accessHandlers());
	renderShareHostSection();
	return userEvent.setup();
}

async function offeredUsernames() {
	const user = userEvent.setup();
	await user.click(screen.getByLabelText("User"));
	const list = await screen.findByTestId("searchable-select-list");
	return within(list)
		.getAllByRole("option")
		.map((option) => option.textContent);
}

// Picks by typing the username (the label narrows the list) and clicking
// the option's id-keyed testid — the core keys object-valued options off
// the keyOf key.
async function pickUser(username: string, userId: string) {
	const user = await renderWithGrants();
	await user.type(screen.getByLabelText("User"), username);
	await user.click(
		await screen.findByTestId(`searchable-select-option-${userId}`),
	);
}

describe("ShareHostSection", () => {
	beforeEach(() => {
		grants = defaultGrants;
		users = defaultUsers;
		grantPayload = undefined;
		grantRequests = 0;
		revokedUserId = undefined;
		grantsRequests = 0;
	});

	it("shows the derived loading copy while the grants request is in flight", async () => {
		server.use(
			http.get("*/api/hosts/m-1/access", async () => {
				await new Promise((resolve) => setTimeout(resolve, 50));
				return HttpResponse.json(defaultGrants);
			}),
			http.get("*/api/users", () => HttpResponse.json(users)),
		);
		renderShareHostSection();

		expect(screen.getByTestId("grants-loading")).toHaveTextContent(
			"Loading grants…",
		);
		expect(await screen.findByTestId("grant-row-u-bob")).toBeInTheDocument();
	});

	it("renders the owner-facing empty state when the host has no grants", async () => {
		grants = [];
		await renderWithGrants();

		const empty = await screen.findByTestId("grants-empty");
		expect(empty).toHaveTextContent(
			"No one else has access to this host yet. Use the form below to grant access.",
		);
		expect(screen.queryByTestId("grant-row-u-bob")).not.toBeInTheDocument();
	});

	it("shows usernames resolved from the users query, not raw user ids", async () => {
		await renderWithGrants();

		const row = await screen.findByTestId("grant-row-u-bob");
		// The directory fetch starts only once the grants are in, so the
		// username swap needs its own wait; exact matching keeps "bob" from
		// passing via the "u-bob" fallback text.
		await waitFor(() =>
			expect(within(row).getByText("bob")).toBeInTheDocument(),
		);
		expect(within(row).queryByText("u-bob")).not.toBeInTheDocument();
		expect(
			within(screen.getByTestId("grant-row-u-owner")).getByText("olivia"),
		).toBeInTheDocument();
	});

	it("falls back to the raw user id when the directory does not know the grant's user", async () => {
		grants = [
			...defaultGrants,
			{
				userId: "u-gone",
				permission: "write",
				grantedAt: "2026-08-04T10:00:00Z",
			},
		];
		await renderWithGrants();

		expect(await screen.findByTestId("grant-row-u-gone")).toHaveTextContent(
			"u-gone",
		);
	});

	it("offers only users who are neither the owner nor already granted", async () => {
		await renderWithGrants();

		// olivia owns the host, bob already holds a grant; only carol
		// and dave are grantable.
		expect(await offeredUsernames()).toEqual(["carol", "dave"]);
	});

	it("excludes the owner even when the access list carries no owner row", async () => {
		grants = [
			{
				userId: "u-bob",
				permission: "read",
				grantedAt: "2026-08-03T10:00:00Z",
			},
		];
		await renderWithGrants();

		expect(await offeredUsernames()).toEqual(["carol", "dave"]);
	});

	it("grants access with the picked user's id and the selected permission, then refetches the grants", async () => {
		await pickUser("carol", "u-carol");
		const user = userEvent.setup();
		await user.selectOptions(screen.getByLabelText("Permission"), "write");
		const requestsBefore = grantsRequests;

		await user.click(screen.getByTestId("grant-access-button"));

		expect(await screen.findByTestId("grant-row-u-carol")).toBeInTheDocument();
		expect(grantPayload).toMatchObject({
			userId: "u-carol",
			permission: "write",
		});
		await waitFor(() => expect(grantsRequests).toBeGreaterThan(requestsBefore));
	});

	it("picks with the keyboard without submitting the surrounding form", async () => {
		const user = await renderWithGrants();
		await screen.findByTestId("grant-row-u-bob");

		const input = screen.getByLabelText("User");
		await user.type(input, "carol");
		await screen.findByTestId("searchable-select-list");
		await user.keyboard("{ArrowDown}{Enter}");

		// The selection landed but no grant was sent: Enter belongs to the
		// picker, not to the form. The chip is the selection; the input is
		// only the query.
		expect(
			screen.getByTestId("searchable-select-chip-u-carol"),
		).toHaveTextContent("carol");
		expect(input).toHaveValue("");
		expect(grantRequests).toBe(0);

		await user.click(screen.getByTestId("grant-access-button"));
		await waitFor(() => expect(grantRequests).toBe(1));
		expect(grantPayload).toMatchObject({ userId: "u-carol" });
	});

	it("clears the selection when the picked user's chip is removed", async () => {
		await pickUser("carol", "u-carol");
		expect(
			screen.getByTestId("searchable-select-chip-u-carol"),
		).toBeInTheDocument();
		expect(screen.getByTestId("grant-access-button")).toBeEnabled();

		const user = userEvent.setup();
		await user.click(screen.getByTestId("searchable-select-remove-u-carol"));

		expect(
			screen.queryByTestId("searchable-select-chip-u-carol"),
		).not.toBeInTheDocument();
		expect(screen.getByTestId("grant-access-button")).toBeDisabled();
	});

	it("clears the picker after a successful grant and hides the new grantee from the choices", async () => {
		await pickUser("carol", "u-carol");
		const user = userEvent.setup();

		await user.click(screen.getByTestId("grant-access-button"));
		await screen.findByTestId("grant-row-u-carol");

		expect(screen.getByLabelText("User")).toHaveValue("");
		expect(
			screen.queryByTestId("searchable-select-chip-u-carol"),
		).not.toBeInTheDocument();
		expect(await offeredUsernames()).toEqual(["dave"]);
	});

	it("renders the error envelope when a grant fails, keeping the pending state during the request", async () => {
		await pickUser("carol", "u-carol");
		const user = userEvent.setup();
		server.use(
			http.post("*/api/hosts/m-1/access", async () => {
				await new Promise((resolve) => setTimeout(resolve, 50));
				return errorResponse(404, "USER_NOT_FOUND", "User not found", "req-6");
			}),
		);

		await user.click(screen.getByTestId("grant-access-button"));

		expect(screen.getByTestId("grant-access-button")).toBeDisabled();
		expect(await screen.findByRole("alert")).toBeInTheDocument();
		expect(screen.getByText("USER_NOT_FOUND")).toBeInTheDocument();
	});

	it("revokes a user, removing their row and refetching grants", async () => {
		const user = await renderWithGrants();
		await screen.findByTestId("grant-row-u-bob");
		const requestsBefore = grantsRequests;

		await user.click(screen.getByTestId("revoke-access-u-bob"));

		await waitFor(() =>
			expect(screen.queryByTestId("grant-row-u-bob")).not.toBeInTheDocument(),
		);
		expect(revokedUserId).toBe("u-bob");
		await waitFor(() => expect(grantsRequests).toBeGreaterThan(requestsBefore));
	});

	// The adapter-wiring smoke for the shared row-scoping projection: the
	// revoke mutation is shared by every grant row, and only the clicked
	// row's button shows pending while the revoke runs. The projection's
	// own contract is pinned once in shared/queries/rowScopedMutation.test.ts.
	it("marks only the clicked row's revoke as pending while the revoke runs", async () => {
		const user = await renderWithGrants();
		await screen.findByTestId("grant-row-u-bob");
		server.use(
			http.delete("*/api/hosts/m-1/access/:userId", async () => {
				await new Promise((resolve) => setTimeout(resolve, 100));
				return new HttpResponse(null, { status: 204 });
			}),
		);

		await user.click(screen.getByTestId("revoke-access-u-bob"));

		const bobButton = screen.getByTestId("revoke-access-u-bob");
		expect(bobButton).toBeDisabled();
		expect(bobButton).toHaveAttribute("data-loading");
		const ownerButton = screen.getByTestId("revoke-access-u-owner");
		expect(ownerButton).toBeEnabled();
		expect(ownerButton).not.toHaveAttribute("data-loading");
	});

	it("renders the error envelope when a revoke fails", async () => {
		const user = await renderWithGrants();
		await screen.findByTestId("grant-row-u-bob");
		server.use(
			http.delete("*/api/hosts/m-1/access/:userId", () =>
				errorResponse(403, "ACCESS_DENIED", "Not allowed", "req-7"),
			),
		);

		await user.click(screen.getByTestId("revoke-access-u-bob"));

		expect(await screen.findByRole("alert")).toBeInTheDocument();
		expect(screen.getByText("ACCESS_DENIED")).toBeInTheDocument();
		expect(screen.getByTestId("grant-row-u-bob")).toBeInTheDocument();
	});
});
