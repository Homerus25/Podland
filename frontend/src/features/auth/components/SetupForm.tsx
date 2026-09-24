import {
	Button,
	FormControl,
	FormErrorMessage,
	FormHelperText,
	FormLabel,
	Input,
	VStack,
} from "@chakra-ui/react";
import { useAuth } from "@features/auth/hooks/useAuth";
import type { SetupInput } from "@features/auth/services/authApi";
import { authApi } from "@features/auth/services/authApi";
import { ErrorDisplay } from "@shared/components/ErrorDisplay";
import { type ApiError, PASSWORD_MIN_LENGTH } from "@shared/types/api";
import { getApiError } from "@shared/utils/api";
import { useState } from "react";
import { useForm } from "react-hook-form";
import { useNavigate } from "react-router-dom";

const required = (label: string) => (value: string) =>
	value.trim().length > 0 || `${label} is required`;

interface SetupFormValues extends SetupInput {
	confirmPassword: string;
}

export function SetupForm() {
	const { adoptSession, markConfigured } = useAuth();
	const navigate = useNavigate();
	const [envelopeError, setEnvelopeError] = useState<ApiError | null>(null);
	const {
		register,
		handleSubmit,
		formState: { errors, isSubmitting },
	} = useForm<SetupFormValues>();

	const onSubmit = async ({ username, password }: SetupFormValues) => {
		setEnvelopeError(null);
		try {
			// Setup answers with the login payload, so the first admin is
			// signed in on the spot — adopted like any other session entry.
			adoptSession(await authApi.setup({ username, password }));
			navigate("/", { replace: true });
		} catch (error) {
			const apiError = getApiError(error);
			if (apiError.code === "SETUP_COMPLETED") {
				// Someone else completed setup while this form was open: the
				// one-time offer is gone, the parked visitor goes to login.
				markConfigured();
				navigate("/login", { replace: true });
				return;
			}
			setEnvelopeError(apiError);
		}
	};

	return (
		<VStack
			as="form"
			onSubmit={handleSubmit(onSubmit)}
			spacing={4}
			align="stretch"
		>
			{envelopeError && <ErrorDisplay error={envelopeError} />}
			<FormControl isInvalid={!!errors.username}>
				<FormLabel htmlFor="username">Username</FormLabel>
				<Input
					id="username"
					autoComplete="username"
					{...register("username", { validate: required("Username") })}
				/>
				{errors.username && (
					<FormErrorMessage>{errors.username.message}</FormErrorMessage>
				)}
			</FormControl>
			<FormControl isInvalid={!!errors.password}>
				<FormLabel htmlFor="password">Password</FormLabel>
				<Input
					id="password"
					type="password"
					autoComplete="new-password"
					{...register("password", {
						validate: {
							required: required("Password"),
							minLength: (value) =>
								value.length >= PASSWORD_MIN_LENGTH ||
								`Password must be at least ${PASSWORD_MIN_LENGTH} characters`,
						},
					})}
				/>
				{errors.password ? (
					<FormErrorMessage>{errors.password.message}</FormErrorMessage>
				) : (
					<FormHelperText>
						At least {PASSWORD_MIN_LENGTH} characters
					</FormHelperText>
				)}
			</FormControl>
			<FormControl isInvalid={!!errors.confirmPassword}>
				<FormLabel htmlFor="confirm-password">Confirm password</FormLabel>
				<Input
					id="confirm-password"
					type="password"
					autoComplete="new-password"
					{...register("confirmPassword", {
						validate: {
							required: required("Password confirmation"),
							matches: (value, { password }) =>
								value === password || "Passwords do not match",
						},
					})}
				/>
				{errors.confirmPassword && (
					<FormErrorMessage>{errors.confirmPassword.message}</FormErrorMessage>
				)}
			</FormControl>
			<Button type="submit" isLoading={isSubmitting}>
				Create admin account
			</Button>
		</VStack>
	);
}
