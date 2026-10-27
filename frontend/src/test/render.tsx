import { ChakraProvider } from "@chakra-ui/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { RenderHookResult, RenderResult } from "@testing-library/react";
import { render, renderHook } from "@testing-library/react";
import type { ReactElement, ReactNode } from "react";
import { MemoryRouter } from "react-router-dom";
import { theme } from "@/theme";

// A MemoryRouter initial entry: a path string, or the object form carrying
// navigation state (react-router's InitialEntry, which the package does not
// re-export).
type TestRouteEntry = string | { pathname: string; state?: unknown };

// The one owner of the test provider stack: every suite renders through
// these helpers instead of hand-rolling QueryClient/Chakra/Router wrappers.
// A fresh retry:false client per call keeps failed queries from spinning,
// and returning the client lets suites observe cache-level behavior on the
// instance the tree actually uses.
export type RenderWithQueryOptions = {
	// true wraps a MemoryRouter; the object form additionally seeds
	// initialEntries (e.g. deep links like /machines/m-1, or an entry object
	// carrying navigation state).
	router?: boolean | { initialEntries?: TestRouteEntry[] };
	// The app theme is installed here because useToast renders its toast
	// through a portal, which dies without a ChakraProvider above the tree.
	chakra?: boolean;
	queryClient?: QueryClient;
};

function freshQueryClient() {
	return new QueryClient({
		defaultOptions: { queries: { retry: false } },
	});
}

export function renderWithQuery(
	ui: ReactElement,
	options: RenderWithQueryOptions = {},
): RenderResult & { queryClient: QueryClient } {
	const { router, chakra, queryClient = freshQueryClient() } = options;
	let tree: ReactNode = ui;
	if (router) {
		tree = (
			<MemoryRouter
				initialEntries={
					typeof router === "object" ? router.initialEntries : undefined
				}
			>
				{tree}
			</MemoryRouter>
		);
	}
	if (chakra) {
		tree = <ChakraProvider theme={theme}>{tree}</ChakraProvider>;
	}
	// Stack order matches renderApp: QueryClient > Chakra > Router. A
	// component needing only Chakra still rides the harness; an unused
	// QueryClientProvider is inert.
	const result = render(
		<QueryClientProvider client={queryClient}>{tree}</QueryClientProvider>,
	);
	return { ...result, queryClient };
}

export function renderHookWithQuery<TResult, TProps>(
	callback: (initialProps: TProps) => TResult,
	options: { queryClient?: QueryClient } = {},
): RenderHookResult<TResult, TProps> & { queryClient: QueryClient } {
	const { queryClient = freshQueryClient() } = options;
	const result = renderHook(callback, {
		wrapper: ({ children }: { children?: ReactNode }) => (
			<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
		),
	});
	return { ...result, queryClient };
}
