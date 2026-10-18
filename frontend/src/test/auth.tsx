import { useAuth } from "@features/auth/hooks/useAuth";
import { ErrorBoundary } from "@shared/components/ErrorBoundary";
import { AuthProvider } from "@shared/context/AuthContext";
import type { User } from "@shared/types/api";
import type { QueryClient } from "@tanstack/react-query";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useLocation } from "react-router-dom";
import { App } from "@/App";
import { renderWithQuery } from "@/test/render";

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
	// The form renders only after the boot resolution (restore + probe)
	// completes, so waiting for it is part of driving it.
	await userEvent.type(await screen.findByLabelText(/username/i), username);
	await userEvent.type(screen.getByLabelText(/password/i), password);
	await userEvent.click(screen.getByRole("button", { name: /log in/i }));
}

export function renderApp(
	at = "/login",
	options: {
		resolveInitialState?: () => Promise<User | null>;
		usernameProbe?: boolean;
		// Optional client override for suites that must pin cache-window
		// behavior, e.g. the app-wide five-minute stale time from main.tsx.
		queryClient?: QueryClient;
	} = {},
) {
	const { resolveInitialState, usernameProbe = true, queryClient } = options;
	// AuthProvider stays inside the router (auth-inside-router order), with
	// the probes above the App so suites can read them before routes render.
	// The ErrorBoundary mirrors main.tsx, so app-level suites run under the
	// same render-crash net as production.
	return renderWithQuery(
		<ErrorBoundary>
			<AuthProvider resolveInitialState={resolveInitialState}>
				<LocationProbe />
				{usernameProbe && <UsernameProbe />}
				<App />
			</AuthProvider>
		</ErrorBoundary>,
		{ router: { initialEntries: [at] }, chakra: true, queryClient },
	);
}
