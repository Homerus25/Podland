import { Box, Flex, useDisclosure, useMediaQuery } from "@chakra-ui/react";
import { useEffect } from "react";
import { Outlet } from "react-router-dom";
import { Header } from "./Header";
import { NavDrawer } from "./NavDrawer";
import { Sidebar } from "./Sidebar";

// The single source of truth for the navigation switch. Chakra's `lg`
// token is 992px; the spec pins the switch at 1024px, so the query is
// explicit and both surfaces render off this one value — no responsive
// CSS props that could drift from it. The test viewport mock imports
// it, so the mocked breakpoint cannot drift either.
export const desktopNavQuery = "(min-width: 1024px)";

export function Layout() {
	const [isDesktop] = useMediaQuery(desktopNavQuery, { ssr: false });
	const {
		isOpen: isDrawerOpen,
		onClose: closeDrawer,
		onToggle: toggleDrawer,
	} = useDisclosure();

	// Past the breakpoint the drawer stops rendering; resetting the
	// transient state here keeps a later shrink from resurrecting it.
	useEffect(() => {
		if (isDesktop) {
			closeDrawer();
		}
	}, [isDesktop, closeDrawer]);

	return (
		<Flex minH="100vh">
			{isDesktop && <Sidebar />}
			<Box flex="1" minW={0}>
				<Header
					mobileNav={
						isDesktop
							? undefined
							: { isOpen: isDrawerOpen, onToggle: toggleDrawer }
					}
				/>
				<Box as="main" p={6}>
					<Outlet />
				</Box>
			</Box>
			{!isDesktop && <NavDrawer isOpen={isDrawerOpen} onClose={closeDrawer} />}
		</Flex>
	);
}
