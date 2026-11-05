import {
	Button,
	FormControl,
	FormLabel,
	HStack,
	Select,
	Text,
	VStack,
} from "@chakra-ui/react";
import { PermissionBadge } from "@features/hosts/components/PermissionBadge";
import {
	useGrantHostAccess,
	useHostAccess,
	useRevokeHostAccess,
} from "@features/hosts/hooks/useHosts";
import { UserPicker } from "@features/users/components/UserPicker";
import { useUsers } from "@features/users/hooks/useUsers";
import type { UserSummary } from "@features/users/services/usersApi";
import { ErrorDisplay } from "@shared/components/ErrorDisplay";
import { ResourceStates } from "@shared/components/ResourceStates";
import type { HostPermission } from "@shared/types/api";
import { getApiError } from "@shared/utils/api";
import { useMemo, useState } from "react";

function formatGrantedAt(iso: string): string {
	return new Date(iso).toLocaleString();
}

// Sharing management for one host: the current grants plus grant/revoke
// actions. The backend identifies users by id, so rows resolve the username
// through the users list query and fall back to the raw id for users the
// directory does not know. The grant form picks its target from the same
// directory; `ownerId` keeps the owner out of the choices because ownership
// is not a grant (the owner row can never be revoked or re-granted).
export function ShareHostSection({
	hostId,
	ownerId,
}: {
	hostId: string;
	ownerId: string;
}) {
	const { data: grants, isPending, error } = useHostAccess(hostId);
	const grantAccess = useGrantHostAccess(hostId);
	const revokeAccess = useRevokeHostAccess(hostId);
	// One directory fetch serves both the row resolution and the picker
	// (the picker consumes the same shared users query).
	const { data: users } = useUsers();
	const usernameById = useMemo(
		() => new Map((users ?? []).map((user) => [user.id, user.username])),
		[users],
	);
	const [selectedUser, setSelectedUser] = useState<UserSummary | null>(null);
	const [permission, setPermission] = useState<HostPermission>("read");

	// Only people who can actually receive a grant: not the owner and not
	// anyone who already holds one (the backend rejects duplicate grants).
	const excludedUserIds = useMemo(
		() => [ownerId, ...(grants ?? []).map((grant) => grant.userId)],
		[ownerId, grants],
	);

	const submitGrant = () => {
		if (!selectedUser) {
			return;
		}
		grantAccess.reset();
		grantAccess.mutate(
			{ userId: selectedUser.id, permission },
			// A successful grant moves the user into the grants list, where
			// the exclusion above hides them anyway — clear the stale
			// selection at the call site.
			{ onSuccess: () => setSelectedUser(null) },
		);
	};

	return (
		<VStack spacing={4} align="stretch" data-testid="share-host-section">
			<ResourceStates
				resource="grant"
				isPending={isPending}
				error={error}
				items={grants}
				emptyMessage="No one else has access to this host yet. Use the form below to grant access."
			>
				<VStack spacing={2} align="stretch">
					{grants?.map((grant) => (
						<HStack
							key={grant.userId}
							justify="space-between"
							data-testid={`grant-row-${grant.userId}`}
						>
							{/* The grant facts stack at base (spec 393): username,
							    badge, and granted-at cannot sit side by side at 360px. */}
							<HStack
								spacing={4}
								flexDirection={{ base: "column", md: "row" }}
								align={{ base: "flex-start", md: "center" }}
							>
								<Text fontFamily="mono">
									{usernameById.get(grant.userId) ?? grant.userId}
								</Text>
								<PermissionBadge permission={grant.permission} />
								<Text color="gray.500" fontSize="sm">
									granted {formatGrantedAt(grant.grantedAt)}
								</Text>
							</HStack>
							<Button
								size="xs"
								colorScheme="red"
								variant="ghost"
								isLoading={
									revokeAccess.isPending &&
									revokeAccess.variables === grant.userId
								}
								onClick={() => {
									revokeAccess.reset();
									revokeAccess.mutate(grant.userId);
								}}
								data-testid={`revoke-access-${grant.userId}`}
							>
								Revoke
							</Button>
						</HStack>
					))}
				</VStack>
			</ResourceStates>
			{revokeAccess.error ? (
				<ErrorDisplay error={getApiError(revokeAccess.error)} />
			) : null}
			{/* The grant form stacks to one column at base (spec 393); the
			    fields stretch so the picker and select keep their width. */}
			<HStack
				spacing={4}
				as="form"
				flexDirection={{ base: "column", md: "row" }}
				align={{ base: "stretch", md: "center" }}
				onSubmit={(event) => {
					event.preventDefault();
					submitGrant();
				}}
			>
				<UserPicker
					value={selectedUser}
					onChange={setSelectedUser}
					excludeUserIds={excludedUserIds}
				/>
				<FormControl>
					<FormLabel htmlFor="grant-permission">Permission</FormLabel>
					<Select
						id="grant-permission"
						value={permission}
						onChange={(event) =>
							setPermission(event.target.value as HostPermission)
						}
					>
						<option value="read">read</option>
						<option value="write">write</option>
						<option value="admin">admin</option>
					</Select>
				</FormControl>
				<Button
					type="submit"
					colorScheme="blue"
					isLoading={grantAccess.isPending}
					isDisabled={!selectedUser}
					data-testid="grant-access-button"
				>
					Grant access
				</Button>
			</HStack>
			{grantAccess.error ? (
				<ErrorDisplay error={getApiError(grantAccess.error)} />
			) : null}
		</VStack>
	);
}
