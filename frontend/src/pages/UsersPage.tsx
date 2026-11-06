import { UserList } from "@features/users/components/UserList";

// The admin Users page: the account directory and the add-user flow. The
// route is gated by AdminRoute, so everything below assumes an admin
// caller — the list fields (role, createdAt) are the admin-only shape of
// GET /api/users. The header owns the page title (issue #459).
export function UsersPage() {
	return <UserList />;
}
