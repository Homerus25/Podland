import { Box, Heading, Text, VStack } from "@chakra-ui/react";
import { SetupForm } from "@features/auth/components/SetupForm";
import { useAuth } from "@features/auth/hooks/useAuth";
import { FullPageSpinner } from "@shared/components/FullPageSpinner";
import { Navigate } from "react-router-dom";

const appName = import.meta.env.VITE_APP_NAME || "Podland";

export function SetupPage() {
	const { isResolving, isAuthenticated, needsSetup } = useAuth();

	if (isResolving) {
		return <FullPageSpinner />;
	}
	// The one-time page never returns: a signed-in visitor, or any visitor
	// of a configured instance, is sent into the app. Logged-out visitors
	// among them bounce off ProtectedRoute to the login form.
	if (isAuthenticated || !needsSetup) {
		return <Navigate to="/" replace />;
	}

	return (
		<VStack spacing={4} justify="center" minH="100vh" px={4}>
			<Heading>{appName}</Heading>
			<Text color="gray.500">
				Create the first admin account for this instance.
			</Text>
			<Box w="full" maxW="sm">
				<SetupForm />
			</Box>
		</VStack>
	);
}
