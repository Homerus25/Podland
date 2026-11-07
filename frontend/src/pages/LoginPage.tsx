import { Box, VStack } from "@chakra-ui/react";
import { LoginForm } from "@features/auth/components/LoginForm";
import { useAuth } from "@features/auth/hooks/useAuth";
import { AppBrand } from "@shared/components/AppBrand";
import { FullPageSpinner } from "@shared/components/FullPageSpinner";
import { Navigate } from "react-router-dom";

export function LoginPage() {
	const { isResolving, needsSetup } = useAuth();

	// The form waits for the boot probe: on a fresh instance the login
	// form is a dead end — every route lands on setup — so a resolved
	// needs-setup flag steers there instead of rendering it.
	if (isResolving) {
		return <FullPageSpinner />;
	}
	if (needsSetup) {
		return <Navigate to="/setup" replace />;
	}

	return (
		<VStack spacing={8} justify="center" minH="100vh" px={4}>
			<AppBrand />
			<Box w="full" maxW="sm">
				<LoginForm />
			</Box>
		</VStack>
	);
}
