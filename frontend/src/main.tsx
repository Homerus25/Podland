import { ChakraProvider, ColorModeScript } from "@chakra-ui/react";
import { AuthProvider } from "@shared/context/AuthContext";
import { shouldRetryQuery } from "@shared/queries/shouldRetryQuery";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ReactQueryDevtools } from "@tanstack/react-query-devtools";
import { createRoot } from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import { App } from "./App";
import { theme } from "./theme";

const queryClient = new QueryClient({
	defaultOptions: {
		queries: {
			refetchOnWindowFocus: false,
			retry: shouldRetryQuery,
			staleTime: 5 * 60 * 1000, // 5 minutes
		},
		mutations: {
			retry: 0, // Fail fast
		},
	},
});

createRoot(document.getElementById("root")!).render(
	<QueryClientProvider client={queryClient}>
		<ColorModeScript initialColorMode={theme.config.initialColorMode} />
		<ChakraProvider theme={theme}>
			<BrowserRouter>
				<AuthProvider>
					<App />
				</AuthProvider>
			</BrowserRouter>
		</ChakraProvider>
		{import.meta.env.DEV && <ReactQueryDevtools initialIsOpen={false} />}
	</QueryClientProvider>,
);
