import { CheckboxList } from "@shared/components/CheckboxList";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

// The checkbox list's behavior, pinned once here (spec #371, ticket #366):
// the option's key is the selection identity, the checked state follows the
// selected array, and the list owns the toggle — the callback receives the
// next selection (checked appends the key, unchecked removes it), so no
// consumer re-writes the append/remove dance. The containers create page
// and the pod create dialog are the two consumers.

const OPTIONS = [
	{ key: "app-net", label: "app-net" },
	{ key: "db-net", label: "db-net" },
];

describe("CheckboxList", () => {
	it("renders one checkbox per option with its label", () => {
		render(<CheckboxList options={OPTIONS} selected={[]} onSelect={vi.fn()} />);

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
				onSelect={vi.fn()}
			/>,
		);

		expect(screen.getByRole("checkbox", { name: "app-net" })).not.toBeChecked();
		expect(screen.getByRole("checkbox", { name: "db-net" })).toBeChecked();
	});

	it("reports the next selection with the key appended on check", async () => {
		const user = userEvent.setup();
		const onSelect = vi.fn();
		render(
			<CheckboxList
				options={OPTIONS}
				selected={["db-net"]}
				onSelect={onSelect}
			/>,
		);

		await user.click(screen.getByRole("checkbox", { name: "app-net" }));

		expect(onSelect).toHaveBeenCalledWith(["db-net", "app-net"]);
	});

	it("reports the next selection with the key removed on uncheck", async () => {
		const user = userEvent.setup();
		const onSelect = vi.fn();
		render(
			<CheckboxList
				options={OPTIONS}
				selected={["app-net", "db-net"]}
				onSelect={onSelect}
			/>,
		);

		await user.click(screen.getByRole("checkbox", { name: "app-net" }));

		expect(onSelect).toHaveBeenCalledWith(["db-net"]);
	});

	it("toggles by key, not label: same-named options stay distinct", async () => {
		const user = userEvent.setup();
		const onSelect = vi.fn();
		render(
			<CheckboxList
				options={[
					{ key: "net-1", label: "shared" },
					{ key: "net-2", label: "shared" },
				]}
				selected={["net-2"]}
				onSelect={onSelect}
			/>,
		);

		const [first, second] = screen.getAllByRole("checkbox", { name: "shared" });
		expect(first).not.toBeChecked();
		expect(second).toBeChecked();

		await user.click(first);

		expect(onSelect).toHaveBeenCalledWith(["net-2", "net-1"]);
	});
});
