import {
	changeUserRole,
	createUser,
	listUsers,
	resetUserPassword,
} from "@features/users/services/usersApi";
import { usersQueryKey } from "@shared/queries/queryKeys";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

// The user directory is app-wide (not machine-scoped), so one shared key
// serves every consumer — this page, the share form's picker.
export function useUsers() {
	return useQuery({
		queryKey: usersQueryKey,
		queryFn: listUsers,
	});
}

// Every users mutation invalidates the one shared key on success, so all
// consumers refetch the post-mutation directory. App-wide, so the
// machine-scoped list-invalidating factory does not apply.
function useUsersInvalidatingMutation<TInput, TData>(
	mutationFn: (input: TInput) => Promise<TData>,
) {
	const queryClient = useQueryClient();
	return useMutation({
		mutationFn,
		onSuccess: () => queryClient.invalidateQueries({ queryKey: usersQueryKey }),
	});
}

// Admin-only creation: the page refetches and shows the created user, the
// picker sees the new directory.
export function useCreateUser() {
	return useUsersInvalidatingMutation(createUser);
}

// Admin-only role change. A self-targeted change is rejected server-side
// (SELF_ROLE_CHANGE) and surfaces through the mutation's error inside the
// dialog.
export function useChangeUserRole() {
	return useUsersInvalidatingMutation(changeUserRole);
}

// Admin-only password reset. The reset itself revokes every session of the
// target server-side; the caller communicates the forced re-login (a toast,
// via the dialog's onSuccess seam).
export function useResetUserPassword() {
	return useUsersInvalidatingMutation(resetUserPassword);
}
