import { ChakraProvider } from "@chakra-ui/react";
import { useAuth } from "@features/auth/hooks/useAuth";
import { AuthProvider } from "@shared/context/AuthContext";
import type { User } from "@shared/types/api";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, useLocation } from "react-router-dom";
import { App } from "@/App";
import { theme } from "@/theme";

export function UsernameProbe() {
	const { user } = useAuth();
	return <div data-testid="auth-user">{user?.username ?? "anonymous"}</div>;
}

// Makes the current URL observable to assertions without reaching into
// the router internals.
export function LocationProbe() {
	const location = useLocation();
	return (
		<div data-testid="location">
			{location.pathname}
			{location.search}
		</div>
	);
}

export async function loginViaForm(
	username = "alice",
	password = "wonderland",
) {
	await userEvent.type(screen.getByLabelText(/username/i), username);
	await userEvent.type(screen.getByLabelText(/password/i), password);
	await userEvent.click(screen.getByRole("button", { name: /log in/i }));
}

export function renderApp(
	at = "/login",
	options: {
		resolveInitialState?: () => Promise<User | null>;
		usernameProbe?: boolean;
	} = {},
) {
	const { resolveInitialState, usernameProbe = true } = options;
	const queryClient = new QueryClient({
		defaultOptions: { queries: { retry: false } },
	});
	return render(
		<QueryClientProvider client={queryClient}>
			<ChakraProvider theme={theme}>
				<MemoryRouter initialEntries={[at]}>
					<AuthProvider resolveInitialState={resolveInitialState}>
						<LocationProbe />
						{usernameProbe && <UsernameProbe />}
						<App />
					</AuthProvider>
				</MemoryRouter>
			</ChakraProvider>
		</QueryClientProvider>,
	);
}
