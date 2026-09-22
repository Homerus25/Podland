import { UserPicker } from "@features/users/components/UserPicker";
import type { UserSummary } from "@features/users/services/usersApi";
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { useState } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
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

async function openListbox() {
	const user = userEvent.setup();
	await user.click(screen.getByTestId("user-picker-input"));
	const listbox = await screen.findByTestId("user-picker-listbox");
	return { user, options: within(listbox).getAllByRole("option") };
}

describe("UserPicker", () => {
	beforeEach(() => {
		users = defaultUsers;
		usersRequests = 0;
		server.use(...usersHandlers());
	});

	it("lists the directory when opened", async () => {
		renderUserPicker();

		const { options } = await openListbox();

		expect(options.map((option) => option.textContent)).toEqual([
			"alice",
			"bob",
			"carol",
		]);
	});

	it("filters client-side while typing: the options narrow without refetching the directory", async () => {
		renderUserPicker();
		const user = userEvent.setup();

		await user.click(screen.getByTestId("user-picker-input"));
		await screen.findByTestId("user-picker-listbox");
		await user.type(screen.getByTestId("user-picker-input"), "ca");

		const listbox = screen.getByTestId("user-picker-listbox");
		expect(
			within(listbox)
				.getAllByRole("option")
				.map((option) => option.textContent),
		).toEqual(["carol"]);
		// One directory request for the whole interaction: narrowing happens
		// in memory, not on the network.
		expect(usersRequests).toBe(1);
	});

	it("selects with the mouse: onChange receives the user and the input shows the username", async () => {
		const onChange = renderUserPicker();

		const { user } = await openListbox();
		await user.click(screen.getByTestId("user-option-u-2"));

		expect(onChange).toHaveBeenCalledWith(
			expect.objectContaining({ id: "u-2", username: "bob" }),
		);
		expect(screen.getByTestId("user-picker-input")).toHaveValue("bob");
		expect(screen.queryByTestId("user-picker-listbox")).not.toBeInTheDocument();
	});

	it("selects with the keyboard: ArrowDown/ArrowUp move the highlight, Enter picks", async () => {
		const onChange = renderUserPicker();
		const user = userEvent.setup();

		const input = screen.getByTestId("user-picker-input");
		await user.click(input);
		await screen.findByTestId("user-picker-listbox");
		// The highlight starts on the first choice; one ArrowDown reaches bob.
		await user.keyboard("{ArrowDown}");

		const options = within(
			screen.getByTestId("user-picker-listbox"),
		).getAllByRole("option");
		expect(options[1]).toHaveAttribute("aria-selected", "true");
		expect(input).toHaveAttribute("aria-activedescendant", options[1].id);

		// ArrowUp walks back to the first choice.
		await user.keyboard("{ArrowUp}");
		expect(options[0]).toHaveAttribute("aria-selected", "true");

		await user.keyboard("{ArrowDown}{Enter}");

		expect(onChange).toHaveBeenCalledWith(
			expect.objectContaining({ id: "u-2", username: "bob" }),
		);
		expect(input).toHaveValue("bob");
	});

	it("reopens on a second click after a selection, focus having never left the input", async () => {
		renderUserPicker();

		const { user } = await openListbox();
		await user.click(screen.getByTestId("user-option-u-2"));
		expect(screen.queryByTestId("user-picker-listbox")).not.toBeInTheDocument();

		await user.click(screen.getByTestId("user-picker-input"));

		expect(
			await screen.findByTestId("user-picker-listbox"),
		).toBeInTheDocument();
	});

	it("never offers excluded users", async () => {
		renderUserPicker({ excludeUserIds: ["u-1", "u-3"] });

		const { options } = await openListbox();

		expect(options.map((option) => option.textContent)).toEqual(["bob"]);
	});

	it("reports no match when the query filters everything out", async () => {
		renderUserPicker();
		const user = userEvent.setup();

		await user.type(screen.getByTestId("user-picker-input"), "zzz");

		expect(screen.getByTestId("user-picker-empty")).toHaveTextContent(
			"No matching users",
		);
	});

	it("closes the listbox on Escape without changing the selection", async () => {
		const onChange = renderUserPicker();
		const user = userEvent.setup();

		await user.click(screen.getByTestId("user-picker-input"));
		await screen.findByTestId("user-picker-listbox");
		await user.keyboard("{Escape}");

		expect(screen.queryByTestId("user-picker-listbox")).not.toBeInTheDocument();
		expect(onChange).not.toHaveBeenCalled();
	});

	it("discards the selection when the user edits the text", async () => {
		const onChange = renderUserPicker({
			value: userFixture({ id: "u-2", username: "bob" }),
		});
		const user = userEvent.setup();

		const input = screen.getByTestId("user-picker-input");
		expect(input).toHaveValue("bob");
		await user.type(input, "x");

		expect(onChange).toHaveBeenCalledWith(null);
	});
});
