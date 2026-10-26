import {
	Button,
	Checkbox,
	FormControl,
	FormErrorMessage,
	FormLabel,
	Input,
	VStack,
} from "@chakra-ui/react";
import { useAuth } from "@features/auth/hooks/useAuth";
import { ErrorDisplay } from "@shared/components/ErrorDisplay";
import type { ApiError, LoginCredentials } from "@shared/types/api";
import { getApiError } from "@shared/utils/api";
import { useState } from "react";
import { useForm } from "react-hook-form";
import type { Location } from "react-router-dom";
import { useLocation, useNavigate } from "react-router-dom";

const required = (label: string) => (value: string) =>
	value.trim().length > 0 || `${label} is required`;

// The remember-me preference is the one app-owned localStorage key: a
// per-browser boolean, pre-filled on every visit to the login form and
// persisted only by a successful login (spec #378, ticket #380). It
// carries no auth material — the token stays memory-only, the refresh
// token httpOnly in the cookie.
export const REMEMBER_DEVICE_STORAGE_KEY = "podland-remember-device";

const readRememberDevice = (): boolean =>
	localStorage.getItem(REMEMBER_DEVICE_STORAGE_KEY) === "true";

export function LoginForm() {
	const { login } = useAuth();
	const navigate = useNavigate();
	const location = useLocation();
	const from = (location.state as { from?: Location } | null)?.from;
	const [envelopeError, setEnvelopeError] = useState<ApiError | null>(null);
	const {
		register,
		handleSubmit,
		formState: { errors, isSubmitting },
	} = useForm<LoginCredentials>({
		defaultValues: { rememberDevice: readRememberDevice() },
	});

	const onSubmit = async (credentials: LoginCredentials) => {
		setEnvelopeError(null);
		try {
			await login(credentials);
			// Persisted only by a successful login: a failed attempt must
			// not overwrite the stored preference (ticket #380).
			localStorage.setItem(
				REMEMBER_DEVICE_STORAGE_KEY,
				String(credentials.rememberDevice === true),
			);
			navigate(from ?? "/", { replace: true });
		} catch (error) {
			setEnvelopeError(getApiError(error));
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
					autoComplete="current-password"
					{...register("password", { validate: required("Password") })}
				/>
				{errors.password && (
					<FormErrorMessage>{errors.password.message}</FormErrorMessage>
				)}
			</FormControl>
			<Checkbox {...register("rememberDevice")}>Remember this device</Checkbox>
			<Button type="submit" isLoading={isSubmitting}>
				Log in
			</Button>
		</VStack>
	);
}
