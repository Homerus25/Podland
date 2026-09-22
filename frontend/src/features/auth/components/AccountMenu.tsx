import { Button, Menu, MenuButton, MenuItem, MenuList } from "@chakra-ui/react";
import { ChangePasswordDialog } from "@features/auth/components/ChangePasswordDialog";
import { useAuth, useChangePassword } from "@features/auth/hooks/useAuth";
import type { LoginResponse } from "@shared/types/api";
import { useState } from "react";
import { useNavigate } from "react-router-dom";

interface AccountMenuProps {
	username: string;
}

// The one identity corner: the signed-in username opens the account menu
// with the password change and the logout, replacing the old bare username
// text plus standalone log-out button.
export function AccountMenu({ username }: AccountMenuProps) {
	const { adoptSession, logout } = useAuth();
	const changePassword = useChangePassword();
	const navigate = useNavigate();
	const [isChangeOpen, setIsChangeOpen] = useState(false);

	const handleLogout = async () => {
		await logout();
		navigate("/login", { replace: true });
	};

	// The re-issued login arrives through the live mutation result by the
	// time the module's onSuccess seam fires; adopting it keeps this device
	// signed in while every other session dies with the old password.
	const adoptReissuedSession = (payload: LoginResponse | undefined) => {
		if (payload) {
			adoptSession(payload);
		}
	};

	return (
		<>
			<Menu>
				<MenuButton as={Button} data-testid="account-menu-button">
					{username}
				</MenuButton>
				<MenuList>
					<MenuItem onClick={() => setIsChangeOpen(true)}>
						Change password
					</MenuItem>
					<MenuItem onClick={handleLogout}>Log out</MenuItem>
				</MenuList>
			</Menu>
			{isChangeOpen ? (
				<ChangePasswordDialog
					mutation={changePassword}
					onClose={() => setIsChangeOpen(false)}
					onSuccess={() => adoptReissuedSession(changePassword.data)}
				/>
			) : null}
		</>
	);
}
