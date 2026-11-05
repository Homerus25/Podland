import { useTheme } from "@chakra-ui/react";
import { QueryClient, useQueryClient } from "@tanstack/react-query";
import { screen } from "@testing-library/react";
import { useLocation } from "react-router-dom";
import { describe, expect, it } from "vitest";
import { renderHookWithQuery, renderWithQuery } from "@/test/render";

function LocationProbe() {
	const location = useLocation();
	return <div data-testid="location">{location.pathname}</div>;
}

describe("renderWithQuery", () => {
	it("wraps each call in a fresh retry:false client and returns it", () => {
		const a = renderWithQuery(<div />);
		const b = renderWithQuery(<div />);

		expect(a.queryClient).not.toBe(b.queryClient);
		expect(a.queryClient.getDefaultOptions().queries?.retry).toBe(false);
		expect(b.queryClient.getDefaultOptions().queries?.retry).toBe(false);
	});

	it("serves an injected client in context and returns that same instance", () => {
		const injected = new QueryClient({
			defaultOptions: { queries: { retry: false } },
		});
		let served: QueryClient | undefined;
		function ClientProbe() {
			served = useQueryClient();
			return null;
		}

		const { queryClient } = renderWithQuery(<ClientProbe />, {
			queryClient: injected,
		});

		expect(served).toBe(injected);
		expect(queryClient).toBe(injected);
	});

	it("honors the router boolean by wrapping a MemoryRouter", () => {
		renderWithQuery(<LocationProbe />, { router: true });

		expect(screen.getByTestId("location")).toHaveTextContent("/");
	});

	it("honors initialEntries in the router object form", () => {
		renderWithQuery(<LocationProbe />, {
			router: { initialEntries: ["/hosts/m-1"] },
		});

		expect(screen.getByTestId("location")).toHaveTextContent("/hosts/m-1");
	});

	it("wraps the app theme under the chakra flag", () => {
		let brand500: string | undefined;
		function ThemeProbe() {
			brand500 = useTheme().colors.brand?.[500];
			return null;
		}

		renderWithQuery(<ThemeProbe />, { chakra: true });

		// The app brand token (frontend/src/theme.ts). Chakra's default theme
		// ships no brand scale at all, so this token distinguishes app theme
		// from the Chakra default the inline wrappers used to absorb.
		expect(brand500).toBe("#0ea5e9");
	});
});

describe("renderHookWithQuery", () => {
	it("serves the returned client to the hook, fresh or injected", () => {
		const { result, queryClient } = renderHookWithQuery(() => useQueryClient());

		expect(result.current).toBe(queryClient);

		const injected = new QueryClient();
		const hooked = renderHookWithQuery(() => useQueryClient(), {
			queryClient: injected,
		});

		expect(hooked.result.current).toBe(injected);
		expect(hooked.queryClient).toBe(injected);
	});
});
