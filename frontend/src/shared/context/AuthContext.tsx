import { authApi } from "@features/auth/services/authApi";
import type { LoginCredentials, LoginResponse, User } from "@shared/types/api";
import {
	refreshSession,
	setBearerToken,
	setSessionExpiredHandler,
} from "@shared/utils/api";
import { useQueryClient } from "@tanstack/react-query";
import type { ReactNode } from "react";
import {
	createContext,
	useCallback,
	useEffect,
	useMemo,
	useState,
} from "react";

interface AuthContextType {
	user: User | null;
	isAuthenticated: boolean;
	isResolving: boolean;
	// First-run steering: true only when the boot landed logged-out and
	// the anonymous status probe reported a user-less instance. Routing
	// reads it — login redirects to setup while it stands, setup
	// redirects into the app when it does not.
	needsSetup: boolean;
	login: (credentials: LoginCredentials) => Promise<void>;
	// Installs an already-issued session payload (the response of an
	// endpoint that re-issues the login, e.g. self-service password
	// change): fresh access token in, user replaced. No request, no cache
	// clear — the session continues, only its credentials rotate.
	adoptSession: (payload: LoginResponse) => void;
	// Records the server's word that the instance is configured — the
	// setup endpoint's SETUP_COMPLETED answer to a parked form — so the
	// boot probe's stale flag cannot bounce the redirected visitor back
	// to the dead setup page.
	markConfigured: () => void;
	logout: () => Promise<void>;
}

export const AuthContext = createContext<AuthContextType>(null!);

// Auth state is memory-only, so a reload starts with nothing and must
// be rebuilt from the httpOnly refresh cookie: this silent-refresh
// resolver asks the one refresh owner (refreshSession) for a fresh
// access JWT and the user it belongs to. Any failure simply resolves to
// logged-out — an expired session behaves exactly
// like a logged-out one, no error screen. The seam exists so the
// routing guard can cover the resolving state (full-page spinner)
// instead of flashing protected content or the login form, and so tests
// can pin the initial state deterministically.
const resolveFromRefreshSession = async (): Promise<User | null> => {
	const payload = await refreshSession();
	return payload === null
		? null
		: { username: payload.username, role: payload.role };
};

export function AuthProvider({
	children,
	resolveInitialState = resolveFromRefreshSession,
}: {
	children: ReactNode;
	resolveInitialState?: () => Promise<User | null>;
}) {
	const [user, setUser] = useState<User | null>(null);
	const [isResolving, setIsResolving] = useState(true);
	const [needsSetup, setNeedsSetup] = useState(false);
	const queryClient = useQueryClient();

	useEffect(() => {
		let cancelled = false;
		resolveInitialState().then(async (resolvedUser) => {
			if (cancelled) return;
			setUser(resolvedUser);
			// The boot probe is the logged-out branch only: a restored
			// session proves a configured instance, so exactly one
			// anonymous status call resolves the flag for everyone else.
			// Its failure is an unreachable instance — normal login — not
			// an error state. isResolving covers the probe too, so routing
			// reads it only after it has an answer.
			if (resolvedUser === null) {
				try {
					const status = await authApi.status();
					if (cancelled) return;
					setNeedsSetup(status.needsSetup);
				} catch {
					if (cancelled) return;
				}
			}
			setIsResolving(false);
		});
		return () => {
			cancelled = true;
		};
	}, [resolveInitialState]);

	// The one owner of "install a fresh session": every entry into the
	// signed-in state (login, adoptSession) rides it, so the token and user
	// state move together in exactly one vocabulary. A session also settles
	// the setup question — an instance someone is signed into is configured.
	const adoptSession = useCallback((payload: LoginResponse) => {
		setBearerToken(payload.token);
		setUser({ username: payload.username, role: payload.role });
		setNeedsSetup(false);
	}, []);

	// The 409 a parked setup form receives is authoritative: the flag the
	// boot probe resolved no longer holds, and without this correction the
	// login page the form redirects to would bounce the visitor straight
	// back to the dead setup page.
	const markConfigured = useCallback(() => {
		setNeedsSetup(false);
	}, []);

	const login = useCallback(
		async (credentials: LoginCredentials) => {
			adoptSession(await authApi.login(credentials));
		},
		[adoptSession],
	);

	// Logout revokes the server-side refresh session, then drops the
	// in-memory token even if the call fails — a failed exit must not
	// strand the user in the app. (The backend clears the cookie before
	// revoking, so a failed call usually means the session was already
	// unreachable, and logout is idempotent on retry.) The query cache is
	// cleared with the same state drop, in the same finally: logout is the
	// one in-session identity exit, so the next identity never starts from
	// the predecessor's cached lists.
	const logout = useCallback(async () => {
		try {
			await authApi.logout();
		} catch {
			// Revocation was best-effort; local state must still be dropped.
		} finally {
			setBearerToken(null);
			setUser(null);
			queryClient.clear();
		}
	}, [queryClient]);

	// The 401 policy owns session death; while this provider lives, its
	// one registered handler is the existing exit path. Unmount
	// unregisters, so a seam firing after teardown reaches nobody.
	useEffect(() => {
		setSessionExpiredHandler(logout);
		return () => {
			setSessionExpiredHandler(null);
		};
	}, [logout]);

	const value = useMemo(
		() => ({
			user,
			isAuthenticated: user !== null,
			isResolving,
			needsSetup,
			login,
			adoptSession,
			markConfigured,
			logout,
		}),
		[
			user,
			isResolving,
			needsSetup,
			login,
			adoptSession,
			markConfigured,
			logout,
		],
	);

	return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}
