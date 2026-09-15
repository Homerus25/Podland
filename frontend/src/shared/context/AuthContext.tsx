import { authApi } from "@features/auth/services/authApi";
import type { LoginCredentials, User } from "@shared/types/api";
import { setBearerToken, setSessionExpiredHandler } from "@shared/utils/api";
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
	login: (credentials: LoginCredentials) => Promise<void>;
	logout: () => Promise<void>;
}

export const AuthContext = createContext<AuthContextType>(null!);

// Auth state is memory-only, so a reload starts with nothing and must
// be rebuilt from the httpOnly refresh cookie: this silent-refresh
// resolver exchanges that cookie for a fresh access JWT and the user it
// belongs to. Any failure (no cookie, expired/revoked session, network)
// simply resolves to logged-out — an expired session behaves exactly
// like a logged-out one, no error screen. The seam exists so the
// routing guard can cover the resolving state (full-page spinner)
// instead of flashing protected content or the login form, and so tests
// can pin the initial state deterministically.
const resolveFromRefreshSession = async (): Promise<User | null> => {
	try {
		const { token, username } = await authApi.refresh();
		setBearerToken(token);
		return { username };
	} catch {
		return null;
	}
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
	const queryClient = useQueryClient();

	useEffect(() => {
		let cancelled = false;
		resolveInitialState().then((resolvedUser) => {
			if (cancelled) return;
			setUser(resolvedUser);
			setIsResolving(false);
		});
		return () => {
			cancelled = true;
		};
	}, [resolveInitialState]);

	const login = useCallback(async (credentials: LoginCredentials) => {
		const { token, username } = await authApi.login(credentials);
		setBearerToken(token);
		setUser({ username });
	}, []);

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
			login,
			logout,
		}),
		[user, isResolving, login, logout],
	);

	return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}
