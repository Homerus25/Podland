import { Box, Text, VStack } from "@chakra-ui/react";
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

export function Sidebar() {
	const isAdmin = useIsAdmin();
	const items = isAdmin ? [...navItems, ...adminNavItems] : navItems;

	return (
		<Box as="nav" w="240px" flexShrink={0} p={4} borderRightWidth="1px">
			<VStack spacing={1} align="stretch">
				{items.map((item) => (
					<NavLink
						key={item.to}
						to={item.to}
						end={item.end}
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
		</Box>
	);
}
