import { PasswordField, passwordRules } from "@shared/components/PasswordField";
import { PASSWORD_MIN_LENGTH } from "@shared/types/api";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useForm } from "react-hook-form";
import { describe, expect, it } from "vitest";
import { renderWithQuery } from "@/test/render";

function Harness({ error }: { error?: string }) {
	const { register } = useForm<{ secret: string }>({
		defaultValues: { secret: "" },
	});
	return (
		<PasswordField
			id="test-password"
			label="Test password"
			registration={register("secret")}
			error={error}
		/>
	);
}

describe("PasswordField", () => {
	it("renders the label wired to a password input", () => {
		renderWithQuery(<Harness />);
		const input = screen.getByLabelText(/^Test password/);
		expect(input).toBeInTheDocument();
		expect(input).toHaveAttribute("type", "password");
	});

	it("registers the field, so typed values reach the form", async () => {
		renderWithQuery(<Harness />);
		await userEvent.type(screen.getByLabelText(/^Test password/), "typed-value");
		expect(screen.getByLabelText(/^Test password/)).toHaveValue("typed-value");
	});

	it("surfaces the field error through the error region", () => {
		renderWithQuery(<Harness error="Password is required" />);
		expect(screen.getByText("Password is required")).toBeInTheDocument();
	});
});

describe("passwordRules", () => {
	it("carries the required message and the shared minimum-length policy", () => {
		const rules = passwordRules("Password is required");
		expect(rules.required).toBe("Password is required");
		expect(rules.minLength).toEqual({
			value: PASSWORD_MIN_LENGTH,
			message: `Password must be at least ${PASSWORD_MIN_LENGTH} characters`,
		});
	});
});
