import {
	FormControl,
	FormErrorMessage,
	FormLabel,
	Input,
} from "@chakra-ui/react";
import { PASSWORD_MIN_LENGTH } from "@shared/types/api";
import type {
	FieldPath,
	FieldValues,
	RegisterOptions,
	UseFormRegisterReturn,
} from "react-hook-form";

// The register rules for a policy-bearing password field: required plus
// the backend's minimum length (400 PASSWORD_TOO_SHORT), so the obvious
// mistake never needs a round trip. Generic over the form's field path so
// the result stays assignable to register's narrowed options type.
export function passwordRules<
	TFieldValues extends FieldValues = FieldValues,
	TName extends FieldPath<TFieldValues> = FieldPath<TFieldValues>,
>(requiredMessage: string): RegisterOptions<TFieldValues, TName> {
	return {
		required: requiredMessage,
		minLength: {
			value: PASSWORD_MIN_LENGTH,
			message: `Password must be at least ${PASSWORD_MIN_LENGTH} characters`,
		},
	};
}

interface PasswordFieldProps {
	// Doubles as the label's htmlFor target, like every form field here.
	id: string;
	label: string;
	registration: UseFormRegisterReturn;
	error?: string;
	// Defaults to "new-password"; the current-password field passes
	// "current-password".
	autoComplete?: string;
	// Monospace input for generated-looking secrets (add-user,
	// reset-password).
	mono?: boolean;
}

// One password field: the label + input + error scaffold every password
// form shares, with the required marker and the type/autoComplete
// plumbing in one place. The length policy rides through `registration`
// — build it with passwordRules.
export function PasswordField({
	id,
	label,
	registration,
	error,
	autoComplete = "new-password",
	mono = false,
}: PasswordFieldProps) {
	return (
		<FormControl isInvalid={!!error} isRequired>
			<FormLabel htmlFor={id}>{label}</FormLabel>
			<Input
				id={id}
				fontFamily={mono ? "mono" : undefined}
				type="password"
				autoComplete={autoComplete}
				{...registration}
			/>
			<FormErrorMessage>{error}</FormErrorMessage>
		</FormControl>
	);
}
