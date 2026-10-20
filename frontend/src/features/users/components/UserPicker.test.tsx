import { UserPicker } from "@features/users/components/UserPicker";
import type { UserSummary } from "@features/users/services/usersApi";
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { useState } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { errorResponse } from "@/test/errors";
import { userFixture } from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

const defaultUsers: UserSummary[] = [
	userFixture({ id: "u-1", username: "alice" }),
	userFixture({ id: "u-2", username: "bob" }),
	userFixture({ id: "u-3", username: "carol" }),
];

let users: UserSummary[] = defaultUsers;
let usersRequests = 0;

function usersHandlers() {
	return [
		http.get("*/api/users", () => {
			usersRequests++;
			return HttpResponse.json(users);
		}),
	];
}

// The picker is controlled, so the harness holds the value state the way
// a real consumer would while spying on the same onChange.
function renderUserPicker(
	props: { value?: UserSummary | null; excludeUserIds?: string[] } = {},
) {
	const onChange = vi.fn();
	function Harness() {
		const [selected, setSelected] = useState<UserSummary | null>(
			props.value ?? null,
		);
		return (
			<UserPicker
				value={selected}
				onChange={(user) => {
					onChange(user);
					setSelected(user);
				}}
				excludeUserIds={props.excludeUserIds}
			/>
		);
	}
	renderWithQuery(<Harness />, { chakra: true });
	return onChange;
}

// Opens the list through the form label, pinning the label association
// the share form relies on.
async function openList() {
	const user = userEvent.setup();
	await user.click(screen.getByLabelText("User"));
	const list = await screen.findByTestId("searchable-select-list");
	return { user, list };
}

// Adapter concerns only: the directory fetch, the exclusion pre-filter,
// the loading copy, and the onChange shape. Keyboard and ARIA behavior is
// pinned once in the core's suite (SearchableSelect.test.tsx).
describe("UserPicker", () => {
	beforeEach(() => {
		users = defaultUsers;
		usersRequests = 0;
		server.use(...usersHandlers());
	});

	it("fetches the directory once and lists it when opened", async () => {
		renderUserPicker();

		const { list } = await openList();

		expect(
			within(list)
				.getAllByRole("option")
				.map((option) => option.textContent),
		).toEqual(["alice", "bob", "carol"]);
		expect(usersRequests).toBe(1);
	});

	it("narrows the offered users while typing without refetching the directory", async () => {
		renderUserPicker();
		const user = userEvent.setup();

		await user.click(screen.getByLabelText("User"));
		await screen.findByTestId("searchable-select-list");
		await user.type(screen.getByLabelText("User"), "ca");

		const list = screen.getByTestId("searchable-select-list");
		expect(
			within(list)
				.getAllByRole("option")
				.map((option) => option.textContent),
		).toEqual(["carol"]);
		// One directory request for the whole interaction: narrowing happens
		// in memory, not on the network.
		expect(usersRequests).toBe(1);
	});

	it("never offers excluded users", async () => {
		renderUserPicker({ excludeUserIds: ["u-1", "u-3"] });

		const { list } = await openList();

		expect(
			within(list)
				.getAllByRole("option")
				.map((option) => option.textContent),
		).toEqual(["bob"]);
	});

	it("shows the loading copy while the directory is in flight", async () => {
		// Deferred rather than timed: the request stays in flight until the
		// test releases it, so the loading assertion cannot race the fetch.
		let resolveUsers: () => void = () => {};
		server.use(
			http.get("*/api/users", async () => {
				await new Promise<void>((resolve) => {
					resolveUsers = resolve;
				});
				return HttpResponse.json(users);
			}),
		);
		renderUserPicker();

		await openList();

		expect(screen.getByTestId("searchable-select-empty")).toHaveTextContent(
			"Loading users…",
		);
		// The directory lands and the options appear without a second
		// interaction.
		resolveUsers();
		expect(
			await screen.findByTestId("searchable-select-option-u-1"),
		).toBeInTheDocument();
	});

	it("reports no match through the core's empty state when the query filters everything out", async () => {
		renderUserPicker();
		const user = userEvent.setup();

		await user.type(screen.getByLabelText("User"), "zzz");

		expect(screen.getByTestId("searchable-select-empty")).toHaveTextContent(
			"No matching users",
		);
	});

	it("hands the full user summary to the caller and renders it as a removable chip", async () => {
		const onChange = renderUserPicker();

		const { user } = await openList();
		await user.click(screen.getByTestId("searchable-select-option-u-2"));

		expect(onChange).toHaveBeenCalledWith(
			expect.objectContaining({ id: "u-2", username: "bob" }),
		);
		expect(screen.getByTestId("searchable-select-chip-u-2")).toHaveTextContent(
			"bob",
		);
		expect(
			screen.queryByTestId("searchable-select-list"),
		).not.toBeInTheDocument();
	});

	it("clears the selection when the chip is removed", async () => {
		const onChange = renderUserPicker();

		const { user } = await openList();
		await user.click(screen.getByTestId("searchable-select-option-u-2"));
		expect(
			screen.getByTestId("searchable-select-chip-u-2"),
		).toBeInTheDocument();

		await user.click(screen.getByTestId("searchable-select-remove-u-2"));

		expect(onChange).toHaveBeenLastCalledWith(null);
		expect(
			screen.queryByTestId("searchable-select-chip-u-2"),
		).not.toBeInTheDocument();
	});

	it("keeps a held selection when the directory refetch rebuilds the option objects", async () => {
		// The caller holds a summary object from the previous fetch; a
		// background refetch rebuilds every option object. The selection
		// compares by id, so the chip survives, the user is not
		// re-offered, and onChange never fires.
		const staleBob = userFixture({ id: "u-2", username: "bob" });
		const onChange = renderUserPicker({ value: staleBob });

		const { list } = await openList();

		expect(screen.getByTestId("searchable-select-chip-u-2")).toHaveTextContent(
			"bob",
		);
		expect(
			within(list)
				.getAllByRole("option")
				.map((option) => option.textContent),
		).toEqual(["alice", "carol"]);
		expect(onChange).not.toHaveBeenCalled();
	});

	it("renders the error state when the directory fetch fails", async () => {
		server.use(
			http.get("*/api/users", () =>
				errorResponse(500, "INTERNAL_ERROR", "Directory unavailable", "req-1"),
			),
		);
		renderUserPicker();

		expect(await screen.findByRole("alert")).toBeInTheDocument();
		expect(screen.getByText("INTERNAL_ERROR")).toBeInTheDocument();
	});
});
