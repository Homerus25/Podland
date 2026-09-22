import { Heading } from "@chakra-ui/react";
import { UserList } from "@features/users/components/UserList";

// The admin Users page: the account directory and the add-user flow. The
// route is gated by AdminRoute, so everything below assumes an admin
// caller — the list fields (role, createdAt) are the admin-only shape of
// GET /api/users.
export function UsersPage() {
	return (
		<>
			<Heading mb={4}>Users</Heading>
			<UserList />
		</>
	);
}
