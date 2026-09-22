import { Flex, Heading, HStack, Spacer } from "@chakra-ui/react";
import { AccountMenu } from "@features/auth/components/AccountMenu";
import { useAuth } from "@features/auth/hooks/useAuth";
import { ColorModeToggle } from "@shared/components/ColorModeToggle";

const appName = import.meta.env.VITE_APP_NAME || "Podland";

export function Header() {
	const { user } = useAuth();

	return (
		<Flex as="header" p={4} pl={6} borderBottomWidth="1px" align="center">
			<Heading size="md">{appName}</Heading>
			<Spacer />
			<HStack spacing={4}>
				<ColorModeToggle />
				{user && <AccountMenu username={user.username} />}
			</HStack>
		</Flex>
	);
}
