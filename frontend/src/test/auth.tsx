import { useAuth } from "@features/auth/hooks/useAuth";
import { AuthProvider } from "@shared/context/AuthContext";
import type { User } from "@shared/types/api";
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
	// AuthProvider stays inside the router (auth-inside-router order), with
	// the probes above the App so suites can read them before routes render.
	return renderWithQuery(
		<AuthProvider resolveInitialState={resolveInitialState}>
			<LocationProbe />
			{usernameProbe && <UsernameProbe />}
			<App />
		</AuthProvider>,
		{ router: { initialEntries: [at] }, chakra: true },
	);
}
