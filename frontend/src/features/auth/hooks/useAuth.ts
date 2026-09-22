import type { ChangePasswordInput } from "@features/auth/services/authApi";
import { authApi } from "@features/auth/services/authApi";
import { AuthContext } from "@shared/context/AuthContext";
import { useMutation } from "@tanstack/react-query";
import { useContext } from "react";

export function useAuth() {
	return useContext(AuthContext);
}

// The admin gate in one place: nav entries (Sidebar) and route guards
// (AdminRoute) ask the same question about the session user, so the role
// comparison has a single owner.
export function useIsAdmin() {
	const { user } = useAuth();
	return user?.role === "admin";
}

// The change rides the auth feature's own endpoint; there is no list to
// invalidate — the session adoption happens at the call site through the
// dialog's onSuccess seam.
export function useChangePassword() {
	return useMutation({
		mutationFn: (input: ChangePasswordInput) => authApi.changePassword(input),
	});
}
