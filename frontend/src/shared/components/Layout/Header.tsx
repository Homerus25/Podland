import { HamburgerIcon } from "@chakra-ui/icons";
import { Flex, Heading, HStack, IconButton, Spacer } from "@chakra-ui/react";
import { AccountMenu } from "@features/auth/components/AccountMenu";
import { useAuth } from "@features/auth/hooks/useAuth";
import { HostHeaderIdentity } from "@features/hosts/components/HostHeaderIdentity";
import { useHostNavScope } from "@features/hosts/hostNav";
import { ColorModeToggle } from "@shared/components/ColorModeToggle";
import { useLocation } from "react-router-dom";

// The header's left slot (issue #459): the page title on the app-level
// pages, the current host's identity on every host-scoped route. The
// app name never renders here — it lives at the top of the sidebar.
const pageTitles: Record<string, string> = {
	"/": "Dashboard",
	"/hosts": "Hosts",
	"/users": "Users",
};

// The Layout hands this down only below the lg breakpoint, where the
// sidebar is hidden and the drawer replaces it; on desktop the prop is
// absent and no hamburger renders.
interface MobileNavToggle {
	isOpen: boolean;
	onToggle: () => void;
}

export function Header({ mobileNav }: { mobileNav?: MobileNavToggle }) {
	const { user } = useAuth();
	const { pathname } = useLocation();
	const scope = useHostNavScope();
	const pageTitle = pageTitles[pathname];

	return (
		// Above the nav drawer's overlay (Layout), so the hamburger
		// keeps toggling the drawer closed while it is open; below
		// Chakra's popover layer, so menus and dialogs still cover it.
		<Flex
			as="header"
			pos="relative"
			zIndex="modal"
			p={4}
			pl={6}
			borderBottomWidth="1px"
			align="center"
		>
			{mobileNav && (
				<IconButton
					aria-label="Toggle navigation"
					aria-expanded={mobileNav.isOpen}
					icon={<HamburgerIcon />}
					onClick={mobileNav.onToggle}
					variant="ghost"
					mr={2}
				/>
			)}
			{scope ? (
				<HostHeaderIdentity hostId={scope.hostId} />
			) : (
				pageTitle && <Heading size="md">{pageTitle}</Heading>
			)}
			<Spacer />
			<HStack spacing={4}>
				<ColorModeToggle />
				{user && <AccountMenu username={user.username} />}
			</HStack>
		</Flex>
	);
}
