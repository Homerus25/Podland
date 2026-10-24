import { CheckboxList } from "@shared/components/CheckboxList";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

// The checkbox list's behavior, pinned once here (spec #355): the option's
// key is the selection identity, the checked state follows the selected
// array, and toggling reports (key, checked) — the containers create page
// and the pod create dialog are the two consumers.

const OPTIONS = [
	{ key: "app-net", label: "app-net" },
	{ key: "db-net", label: "db-net" },
];

describe("CheckboxList", () => {
	it("renders one checkbox per option with its label", () => {
		render(<CheckboxList options={OPTIONS} selected={[]} onToggle={vi.fn()} />);

		expect(
			screen.getByRole("checkbox", { name: "app-net" }),
		).toBeInTheDocument();
		expect(
			screen.getByRole("checkbox", { name: "db-net" }),
		).toBeInTheDocument();
	});

	it("checks exactly the selected keys", () => {
		render(
			<CheckboxList
				options={OPTIONS}
				selected={["db-net"]}
				onToggle={vi.fn()}
			/>,
		);

		expect(screen.getByRole("checkbox", { name: "app-net" })).not.toBeChecked();
		expect(screen.getByRole("checkbox", { name: "db-net" })).toBeChecked();
	});

	it("reports (key, checked) on toggle", async () => {
		const user = userEvent.setup();
		const onToggle = vi.fn();
		render(
			<CheckboxList options={OPTIONS} selected={[]} onToggle={onToggle} />,
		);

		await user.click(screen.getByRole("checkbox", { name: "app-net" }));

		expect(onToggle).toHaveBeenCalledWith("app-net", true);
	});
});
