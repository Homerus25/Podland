import { Text, VStack } from "@chakra-ui/react";
import { useIsAdmin } from "@features/auth/hooks/useAuth";
import { HostNavSection } from "@features/hosts/components/HostNavSection";
import { AppBrand } from "@shared/components/AppBrand";
import { NavLink } from "react-router-dom";
import { navItemStyle } from "./navItemStyle";

interface NavItem {
	label: string;
	to: string;
	end?: boolean;
}

const navItems: NavItem[] = [
	{ label: "Dashboard", to: "/", end: true },
	// end: the Hosts link is active only on the list page — never on the
	// host-scoped routes, where the host tab section carries the active
	// state instead (issue #459).
	{ label: "Hosts", to: "/hosts", end: true },
];

// The users admin surface exists for admins only, so its nav entry
// follows the same gate as the route behind it.
const adminNavItems: NavItem[] = [{ label: "Users", to: "/users" }];

// The nav list both navigation surfaces render — the desktop sidebar
// and the mobile drawer share items, admin gating, and active styling.
// onNavigate is the drawer's close-on-navigate seam; the sidebar passes
// nothing. The app brand (logo + title) lives here above the links, and
// the current host's tabs render as a second section below the main
// links on host-scoped routes (issue #459).
export function SidebarNav({ onNavigate }: { onNavigate?: () => void }) {
	const isAdmin = useIsAdmin();
	const items = isAdmin ? [...navItems, ...adminNavItems] : navItems;

	return (
		<VStack as="nav" spacing={1} align="stretch">
			<AppBrand headingSize="md" mb={6} px={3} />
			{items.map((item) => (
				<NavLink
					key={item.to}
					to={item.to}
					end={item.end}
					onClick={onNavigate}
					style={{ textDecoration: "none" }}
				>
					{({ isActive }) => (
						<Text as="span" display="block" {...navItemStyle(isActive)}>
							{item.label}
						</Text>
					)}
				</NavLink>
			))}
			<HostNavSection onNavigate={onNavigate} />
		</VStack>
	);
}
