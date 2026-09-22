import { useIsAdmin } from "@features/auth/hooks/useAuth";
import type { ReactNode } from "react";
import { Navigate } from "react-router-dom";

// The admin counterpart of ProtectedRoute: nests below it, so the session
// is already resolved and authenticated by the time this renders — the
// only question left is the role. A signed-in non-admin is sent home
// instead of seeing the admin surface (and no trace of it renders).
export function AdminRoute({ children }: { children: ReactNode }) {
	const isAdmin = useIsAdmin();

	if (!isAdmin) {
		return <Navigate to="/" replace />;
	}
	return <>{children}</>;
}
