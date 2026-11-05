import { PermissionHint } from "@shared/components/PermissionHint";
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

// Pins the module's three templates verbatim, the derived
// `{scope}-permission-hint` testid, and the data-requirement attribute
// values. Consumers select the hint through its derived testid.
describe("PermissionHint", () => {
	it("renders the write template with the guarded resource family", () => {
		render(
			<PermissionHint
				scope="containers"
				requirement="write"
				resource="container"
			/>,
		);

		const hint = screen.getByTestId("containers-permission-hint");
		expect(hint).toHaveTextContent(
			"You have read-only access to this host. Ask an owner or admin for write access to manage its containers.",
		);
		expect(hint).toHaveAttribute("data-requirement", "write");
	});

	it("renders the admin template as the fixed secret-reveal sentence", () => {
		render(<PermissionHint scope="reveal" requirement="admin" />);

		const hint = screen.getByTestId("reveal-permission-hint");
		expect(hint).toHaveTextContent(
			"Revealing the secret's value requires admin access.",
		);
		expect(hint).toHaveAttribute("data-requirement", "admin");
	});

	it("renders the owner template with the verb phrase", () => {
		render(
			<PermissionHint scope="settings" requirement="owner" phrase="edit" />,
		);

		const hint = screen.getByTestId("settings-permission-hint");
		expect(hint).toHaveTextContent("Only the owner can edit this host.");
		expect(hint).toHaveAttribute("data-requirement", "owner");
	});
});
