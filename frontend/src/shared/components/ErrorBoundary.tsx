import { Button, Center, Heading, Text, VStack } from "@chakra-ui/react";
import { Component, type ErrorInfo, type ReactNode } from "react";

// The app's one render-crash net. React error boundaries must be class
// components; without one, a throw anywhere unmounts the whole tree and
// the page goes black. The fallback offers a reload — the only recovery,
// since the crashed tree's state is gone. Wired once in main.tsx, inside
// ChakraProvider so the fallback renders with the theme.
export class ErrorBoundary extends Component<
	{ children: ReactNode },
	{ error: Error | null }
> {
	state: { error: Error | null } = { error: null };

	static getDerivedStateFromError(error: Error) {
		return { error };
	}

	componentDidCatch(error: Error, info: ErrorInfo) {
		console.error("Uncaught render error:", error, info.componentStack);
	}

	render() {
		const { error } = this.state;
		if (error) {
			return (
				<Center minH="100vh" data-testid="error-boundary-fallback">
					<VStack spacing={4} maxW="lg" p={6} textAlign="center">
						<Heading size="lg">Something went wrong</Heading>
						<Text color="gray.500">
							An unexpected error crashed this page. Reloading usually fixes it.
						</Text>
						<Text
							fontFamily="mono"
							fontSize="sm"
							wordBreak="break-all"
							color="red.500"
							data-testid="error-boundary-message"
						>
							{error.message}
						</Text>
						<Button colorScheme="blue" onClick={() => window.location.reload()}>
							Reload page
						</Button>
					</VStack>
				</Center>
			);
		}
		return this.props.children;
	}
}
