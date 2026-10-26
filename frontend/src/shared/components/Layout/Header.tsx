import { HamburgerIcon } from "@chakra-ui/icons";
import { Flex, Heading, HStack, IconButton, Spacer } from "@chakra-ui/react";
import { AccountMenu } from "@features/auth/components/AccountMenu";
import { useAuth } from "@features/auth/hooks/useAuth";
import { ColorModeToggle } from "@shared/components/ColorModeToggle";

const appName = import.meta.env.VITE_APP_NAME || "Podland";

// The Layout hands this down only below the lg breakpoint, where the
// sidebar is hidden and the drawer replaces it; on desktop the prop is
// absent and no hamburger renders.
interface MobileNavToggle {
	isOpen: boolean;
	onToggle: () => void;
}

export function Header({ mobileNav }: { mobileNav?: MobileNavToggle }) {
	const { user } = useAuth();

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
			<Heading size="md">{appName}</Heading>
			<Spacer />
			<HStack spacing={4}>
				<ColorModeToggle />
				{user && <AccountMenu username={user.username} />}
			</HStack>
		</Flex>
	);
}
