import { Text, VStack } from "@chakra-ui/react";
import { useIsAdmin } from "@features/auth/hooks/useAuth";
import { NavLink } from "react-router-dom";

interface NavItem {
	label: string;
	to: string;
	end?: boolean;
}

const navItems: NavItem[] = [
	{ label: "Dashboard", to: "/", end: true },
	{ label: "Machines", to: "/machines" },
];

// The users admin surface exists for admins only, so its nav entry
// follows the same gate as the route behind it.
const adminNavItems: NavItem[] = [{ label: "Users", to: "/users" }];

// The nav list both navigation surfaces render — the desktop sidebar
// and the mobile drawer share items, admin gating, and active styling.
// onNavigate is the drawer's close-on-navigate seam; the sidebar passes
// nothing.
export function SidebarNav({ onNavigate }: { onNavigate?: () => void }) {
	const isAdmin = useIsAdmin();
	const items = isAdmin ? [...navItems, ...adminNavItems] : navItems;

	return (
		<VStack as="nav" spacing={1} align="stretch">
			{items.map((item) => (
				<NavLink
					key={item.to}
					to={item.to}
					end={item.end}
					onClick={onNavigate}
					style={{ textDecoration: "none" }}
				>
					{({ isActive }) => (
						<Text
							as="span"
							display="block"
							px={3}
							py={2}
							borderRadius="md"
							fontWeight={isActive ? "semibold" : "normal"}
							bg={isActive ? "brand.600" : "transparent"}
							color={isActive ? "white" : "inherit"}
							_hover={{
								bg: isActive ? "brand.700" : "gray.100",
								_dark: { bg: isActive ? "brand.700" : "gray.700" },
							}}
						>
							{item.label}
						</Text>
					)}
				</NavLink>
			))}
		</VStack>
	);
}
