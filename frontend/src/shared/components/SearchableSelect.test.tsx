import {
	SearchableSelect,
	type SearchableSelectOption,
} from "@shared/components/SearchableSelect";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";
import { renderWithQuery } from "@/test/render";

// Pins the combobox ritual once, at the rendered DOM: filtering as the
// user types, keyboard navigation, single vs multi selection, the empty
// state, and the ARIA 1.2 combobox pattern. Every combobox in the app
// consumes the component as an adapter — the three #251 dialogs
// (string-valued multi-select) and the users feature's UserPicker
// (object-valued single-select) — and none of them re-pins any of this.

const options: SearchableSelectOption<string>[] = [
	{ value: "alpha", label: "Alpha" },
	{ value: "beta", label: "Beta" },
	{ value: "gamma", label: "Gamma" },
];

type HarnessProps = {
	options?: SearchableSelectOption<string>[];
	searchPlaceholder?: string;
	emptyMessage?: string;
	id?: string;
};

function SingleSelect({
	options: opts = options,
	searchPlaceholder,
	emptyMessage,
	id,
	onChange,
}: HarnessProps & { onChange?: (value: string | null) => void }) {
	const [value, setValue] = useState<string | null>(null);
	return (
		<SearchableSelect
			mode="single"
			options={opts}
			value={value}
			onChange={(next) => {
				setValue(next);
				onChange?.(next);
			}}
			searchPlaceholder={searchPlaceholder}
			emptyMessage={emptyMessage}
			id={id}
		/>
	);
}

function MultiSelect({
	options: opts = options,
	onChange,
}: HarnessProps & { onChange?: (value: string[]) => void }) {
	const [value, setValue] = useState<string[]>([]);
	return (
		<SearchableSelect
			mode="multi"
			options={opts}
			value={value}
			onChange={(next) => {
				setValue(next);
				onChange?.(next);
			}}
		/>
	);
}

function renderSingle(
	props: HarnessProps & { onChange?: (value: string | null) => void } = {},
) {
	return renderWithQuery(<SingleSelect {...props} />, { chakra: true });
}

function renderMulti(
	props: HarnessProps & { onChange?: (value: string[]) => void } = {},
) {
	return renderWithQuery(<MultiSelect {...props} />, { chakra: true });
}

describe("SearchableSelect", () => {
	it("renders the search input and opens the option list on focus", async () => {
		const user = userEvent.setup();
		renderSingle();

		const input = screen.getByTestId("searchable-select-input");
		expect(input).toHaveAttribute("placeholder", "Search…");
		expect(
			screen.queryByTestId("searchable-select-list"),
		).not.toBeInTheDocument();

		await user.click(input);

		expect(screen.getByTestId("searchable-select-list")).toBeInTheDocument();
		expect(
			screen.getByTestId("searchable-select-option-alpha"),
		).toHaveTextContent("Alpha");
		expect(
			screen.getByTestId("searchable-select-option-beta"),
		).toHaveTextContent("Beta");
		expect(
			screen.getByTestId("searchable-select-option-gamma"),
		).toHaveTextContent("Gamma");
	});

	it("filters the options as the user types, case-insensitive substring", async () => {
		const user = userEvent.setup();
		renderSingle();

		const input = screen.getByTestId("searchable-select-input");
		await user.click(input);
		await user.type(input, "ALP");

		expect(
			screen.getByTestId("searchable-select-option-alpha"),
		).toBeInTheDocument();
		expect(
			screen.queryByTestId("searchable-select-option-beta"),
		).not.toBeInTheDocument();
		expect(
			screen.queryByTestId("searchable-select-option-gamma"),
		).not.toBeInTheDocument();
	});

	it("renders the empty state when no option matches the search", async () => {
		const user = userEvent.setup();
		renderSingle();

		const input = screen.getByTestId("searchable-select-input");
		await user.click(input);
		await user.type(input, "zzz");

		expect(screen.getByTestId("searchable-select-empty")).toHaveTextContent(
			"No matches found.",
		);
		expect(
			screen.queryByTestId("searchable-select-option-alpha"),
		).not.toBeInTheDocument();
	});

	it("renders the empty state when the option list is empty", async () => {
		const user = userEvent.setup();
		renderSingle({ options: [] });

		await user.click(screen.getByTestId("searchable-select-input"));

		expect(screen.getByTestId("searchable-select-empty")).toHaveTextContent(
			"No matches found.",
		);
	});

	it("renders the emptyMessage override in place of the default copy", async () => {
		const user = userEvent.setup();
		renderSingle({ options: [], emptyMessage: "No running containers." });

		await user.click(screen.getByTestId("searchable-select-input"));

		expect(screen.getByTestId("searchable-select-empty")).toHaveTextContent(
			"No running containers.",
		);
	});

	it("uses the searchPlaceholder prop for the input", () => {
		renderSingle({ searchPlaceholder: "Find a container…" });

		expect(screen.getByTestId("searchable-select-input")).toHaveAttribute(
			"placeholder",
			"Find a container…",
		);
	});

	it("moves the active option with the arrow keys and selects it with Enter", async () => {
		const onChange = vi.fn();
		const user = userEvent.setup();
		renderSingle({ onChange });

		const input = screen.getByTestId("searchable-select-input");
		await user.click(input);

		const alpha = screen.getByTestId("searchable-select-option-alpha");
		expect(alpha).toHaveAttribute("data-active", "true");
		expect(input).toHaveAttribute("aria-activedescendant", alpha.id);

		await user.keyboard("{ArrowDown}");
		const beta = screen.getByTestId("searchable-select-option-beta");
		expect(beta).toHaveAttribute("data-active", "true");
		expect(input).toHaveAttribute("aria-activedescendant", beta.id);

		await user.keyboard("{ArrowDown}");
		expect(
			screen.getByTestId("searchable-select-option-gamma"),
		).toHaveAttribute("data-active", "true");

		// clamped at the last option
		await user.keyboard("{ArrowDown}");
		expect(
			screen.getByTestId("searchable-select-option-gamma"),
		).toHaveAttribute("data-active", "true");

		await user.keyboard("{ArrowUp}");
		expect(screen.getByTestId("searchable-select-option-beta")).toHaveAttribute(
			"data-active",
			"true",
		);

		await user.keyboard("{Enter}");
		expect(onChange).toHaveBeenCalledWith("beta");
	});

	it("closes the list on Escape and reopens it on focus", async () => {
		const user = userEvent.setup();
		renderSingle();

		const input = screen.getByTestId("searchable-select-input");
		await user.click(input);
		expect(screen.getByTestId("searchable-select-list")).toBeInTheDocument();

		await user.keyboard("{Escape}");
		expect(
			screen.queryByTestId("searchable-select-list"),
		).not.toBeInTheDocument();

		await user.click(input);
		expect(screen.getByTestId("searchable-select-list")).toBeInTheDocument();
	});

	it("single mode: picking closes the list and shows a removable chip", async () => {
		const onChange = vi.fn();
		const user = userEvent.setup();
		renderSingle({ onChange });

		const input = screen.getByTestId("searchable-select-input");
		await user.click(input);
		await user.click(screen.getByTestId("searchable-select-option-alpha"));

		expect(onChange).toHaveBeenCalledWith("alpha");
		expect(
			screen.queryByTestId("searchable-select-list"),
		).not.toBeInTheDocument();
		expect(
			screen.getByTestId("searchable-select-chip-alpha"),
		).toHaveTextContent("Alpha");

		// the chosen option leaves the list while it is chosen
		await user.click(input);
		expect(
			screen.queryByTestId("searchable-select-option-alpha"),
		).not.toBeInTheDocument();
		expect(
			screen.getByTestId("searchable-select-option-beta"),
		).toBeInTheDocument();

		// removing the chip clears the value (null in single mode) and
		// brings the option back
		await user.click(screen.getByTestId("searchable-select-remove-alpha"));
		expect(onChange).toHaveBeenLastCalledWith(null);
		expect(
			screen.queryByTestId("searchable-select-chip-alpha"),
		).not.toBeInTheDocument();
		expect(
			screen.getByTestId("searchable-select-option-alpha"),
		).toBeInTheDocument();
	});

	it("multi mode: picks accumulate, the list stays open, and chips remove", async () => {
		const onChange = vi.fn();
		const user = userEvent.setup();
		renderMulti({ onChange });

		const input = screen.getByTestId("searchable-select-input");
		await user.click(input);
		await user.click(screen.getByTestId("searchable-select-option-alpha"));

		expect(onChange).toHaveBeenLastCalledWith(["alpha"]);
		expect(screen.getByTestId("searchable-select-list")).toBeInTheDocument();
		expect(
			screen.getByTestId("searchable-select-chip-alpha"),
		).toHaveTextContent("Alpha");
		expect(
			screen.queryByTestId("searchable-select-option-alpha"),
		).not.toBeInTheDocument();

		await user.click(screen.getByTestId("searchable-select-option-beta"));
		expect(onChange).toHaveBeenLastCalledWith(["alpha", "beta"]);
		expect(screen.getByTestId("searchable-select-chip-beta")).toHaveTextContent(
			"Beta",
		);

		await user.click(screen.getByTestId("searchable-select-remove-alpha"));
		expect(onChange).toHaveBeenLastCalledWith(["beta"]);
		expect(
			screen.queryByTestId("searchable-select-chip-alpha"),
		).not.toBeInTheDocument();
		expect(
			screen.getByTestId("searchable-select-option-alpha"),
		).toBeInTheDocument();
	});

	it("does not select anything on Enter when no option matches", async () => {
		const onChange = vi.fn();
		const user = userEvent.setup();
		renderSingle({ onChange });

		const input = screen.getByTestId("searchable-select-input");
		await user.click(input);
		await user.type(input, "zzz");
		await user.keyboard("{Enter}");

		expect(onChange).not.toHaveBeenCalled();
		expect(screen.getByTestId("searchable-select-empty")).toBeInTheDocument();
	});

	it("gives each instance a unique id family so two selects never collide", async () => {
		const user = userEvent.setup();
		renderWithQuery(
			<>
				<SingleSelect />
				<SingleSelect />
			</>,
			{ chakra: true },
		);

		const inputs = screen.getAllByTestId("searchable-select-input");
		expect(inputs).toHaveLength(2);

		await user.click(inputs[0]);
		await user.click(inputs[1]);

		const lists = screen.getAllByTestId("searchable-select-list");
		expect(lists).toHaveLength(2);
		expect(lists[0].id).not.toBe(lists[1].id);
		expect(inputs[0]).toHaveAttribute("aria-controls", lists[0].id);
		expect(inputs[1]).toHaveAttribute("aria-controls", lists[1].id);

		const optionsA = lists[0].querySelectorAll('[role="option"]');
		const optionsB = lists[1].querySelectorAll('[role="option"]');
		expect(optionsA[0].id).not.toBe(optionsB[0].id);
	});

	it("marks the selected option with aria-selected", async () => {
		const user = userEvent.setup();
		renderSingle();

		const input = screen.getByTestId("searchable-select-input");
		await user.click(input);

		// Nothing selected yet: every visible option is aria-selected=false.
		for (const option of screen.getAllByRole("option")) {
			expect(option).toHaveAttribute("aria-selected", "false");
		}

		// Pick alpha; it leaves the list (single mode), so re-open and
		// check the remaining options are still unselected.
		await user.click(screen.getByTestId("searchable-select-option-alpha"));
		await user.click(input);
		for (const option of screen.getAllByRole("option")) {
			expect(option).toHaveAttribute("aria-selected", "false");
		}
	});

	it("follows the ARIA 1.2 combobox pattern on the input", () => {
		renderSingle();

		const input = screen.getByTestId("searchable-select-input");
		expect(input).toHaveAttribute("role", "combobox");
		expect(input).toHaveAttribute("aria-autocomplete", "list");
		expect(input).toHaveAttribute("autocomplete", "off");
	});

	it("passes the id prop through to the input for label association", () => {
		renderSingle({ id: "grant-user-search" });

		expect(screen.getByTestId("searchable-select-input")).toHaveAttribute(
			"id",
			"grant-user-search",
		);
	});

	it("single mode starts from a null selection and renders no chip", () => {
		renderSingle();

		expect(
			screen.queryByTestId("searchable-select-chips"),
		).not.toBeInTheDocument();
	});

	it("supports object-valued options: the full object reaches onChange and the chip keys off the label", async () => {
		const onChange = vi.fn();
		const user = userEvent.setup();
		const alice = { id: "u-1", username: "alice" };
		const bob = { id: "u-2", username: "bob" };
		function ObjectSelect() {
			const [value, setValue] = useState<{
				id: string;
				username: string;
			} | null>(null);
			return (
				<SearchableSelect
					mode="single"
					options={[
						{ value: alice, label: "alice" },
						{ value: bob, label: "bob" },
					]}
					value={value}
					onChange={(next) => {
						setValue(next);
						onChange(next);
					}}
				/>
			);
		}
		renderWithQuery(<ObjectSelect />, { chakra: true });

		const input = screen.getByTestId("searchable-select-input");
		await user.click(input);
		await user.click(screen.getByTestId("searchable-select-option-alice"));

		expect(onChange).toHaveBeenCalledWith(alice);
		expect(
			screen.getByTestId("searchable-select-chip-alice"),
		).toHaveTextContent("alice");
		expect(
			screen.queryByTestId("searchable-select-option-alice"),
		).not.toBeInTheDocument();

		await user.click(screen.getByTestId("searchable-select-remove-alice"));
		expect(onChange).toHaveBeenLastCalledWith(null);
		expect(
			screen.queryByTestId("searchable-select-chip-alice"),
		).not.toBeInTheDocument();

		// the option returns to the list once the selection is cleared
		await user.click(input);
		expect(
			screen.getByTestId("searchable-select-option-alice"),
		).toBeInTheDocument();
	});

	it("keyOf: selection compares by key, so rebuilt option objects cannot strand a held value", async () => {
		const onChange = vi.fn();
		const user = userEvent.setup();
		// The "refetched" directory: same users, fresh objects.
		const directory = [
			{ id: "u-1", username: "alice" },
			{ id: "u-2", username: "bob" },
		];
		// The caller holds a summary from the previous fetch — a distinct
		// object from anything in the current options array.
		const heldBob = { id: "u-2", username: "bob" };
		function KeyedSelect() {
			const [value, setValue] = useState<{
				id: string;
				username: string;
			} | null>(heldBob);
			return (
				<SearchableSelect
					mode="single"
					options={directory.map((u) => ({ value: u, label: u.username }))}
					value={value}
					keyOf={(u) => u.id}
					onChange={(next) => {
						setValue(next);
						onChange(next);
					}}
				/>
			);
		}
		renderWithQuery(<KeyedSelect />, { chakra: true });

		// The held value renders as the chip (keyed off the id) and bob
		// leaves the offered list — no re-pick, no onChange.
		expect(screen.getByTestId("searchable-select-chip-u-2")).toHaveTextContent(
			"bob",
		);
		const input = screen.getByTestId("searchable-select-input");
		await user.click(input);
		expect(
			screen.getByTestId("searchable-select-option-u-1"),
		).toHaveTextContent("alice");
		expect(
			screen.queryByTestId("searchable-select-option-u-2"),
		).not.toBeInTheDocument();
		expect(onChange).not.toHaveBeenCalled();

		// Removing the chip clears the selection by key.
		await user.click(screen.getByTestId("searchable-select-remove-u-2"));
		expect(onChange).toHaveBeenLastCalledWith(null);
		expect(
			screen.getByTestId("searchable-select-option-u-2"),
		).toBeInTheDocument();
	});

	it("keyOf: per-option testids key off the key, so duplicate labels cannot collide", async () => {
		const user = userEvent.setup();
		function KeyedSelect() {
			const [value, setValue] = useState<{ id: string; name: string } | null>(
				null,
			);
			return (
				<SearchableSelect
					mode="single"
					options={[
						{ value: { id: "n-1", name: "nfs" }, label: "nfs" },
						{ value: { id: "n-2", name: "nfs" }, label: "nfs" },
					]}
					value={value}
					keyOf={(v) => v.id}
					onChange={setValue}
				/>
			);
		}
		renderWithQuery(<KeyedSelect />, { chakra: true });

		await user.click(screen.getByTestId("searchable-select-input"));
		expect(
			screen.getByTestId("searchable-select-option-n-1"),
		).toHaveTextContent("nfs");
		expect(
			screen.getByTestId("searchable-select-option-n-2"),
		).toHaveTextContent("nfs");
	});
});
