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
// user types, keyboard navigation, single vs multi selection, and the
// empty state. The three #251 dialogs consume the component as adapters
// and do not re-pin any of this.

const options: SearchableSelectOption[] = [
	{ value: "alpha", label: "Alpha" },
	{ value: "beta", label: "Beta" },
	{ value: "gamma", label: "Gamma" },
];

type HarnessProps = {
	options?: SearchableSelectOption[];
	searchPlaceholder?: string;
	emptyMessage?: string;
};

function SingleSelect({
	options: opts = options,
	searchPlaceholder,
	emptyMessage,
	onChange,
}: HarnessProps & { onChange?: (value: string) => void }) {
	const [value, setValue] = useState("");
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
	props: HarnessProps & { onChange?: (value: string) => void } = {},
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

		// removing the chip clears the value and brings the option back
		await user.click(screen.getByTestId("searchable-select-remove-alpha"));
		expect(onChange).toHaveBeenLastCalledWith("");
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
});
