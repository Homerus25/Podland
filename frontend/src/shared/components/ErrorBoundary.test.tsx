import { ErrorBoundary } from "@shared/components/ErrorBoundary";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { renderWithQuery } from "@/test/render";

function Thrower(): never {
	throw new Error("boom");
}

// React logs caught errors through console.error; the spy keeps the test
// output clean and doubles as the assertion that componentDidCatch fired.
function silenceConsoleError() {
	return vi.spyOn(console, "error").mockImplementation(() => {});
}

afterEach(() => {
	vi.restoreAllMocks();
});

describe("ErrorBoundary", () => {
	it("renders its children when nothing throws", () => {
		renderWithQuery(
			<ErrorBoundary>
				<div data-testid="child">all good</div>
			</ErrorBoundary>,
			{ chakra: true },
		);

		expect(screen.getByTestId("child")).toHaveTextContent("all good");
		expect(
			screen.queryByTestId("error-boundary-fallback"),
		).not.toBeInTheDocument();
	});

	it("renders the fallback with the error message when a child throws", () => {
		const consoleError = silenceConsoleError();
		renderWithQuery(
			<ErrorBoundary>
				<Thrower />
			</ErrorBoundary>,
			{ chakra: true },
		);

		const fallback = screen.getByTestId("error-boundary-fallback");
		expect(fallback).toHaveTextContent("Something went wrong");
		expect(screen.getByTestId("error-boundary-message")).toHaveTextContent(
			"boom",
		);
		expect(consoleError).toHaveBeenCalledWith(
			"Uncaught render error:",
			expect.any(Error),
			expect.anything(),
		);
	});

	it("reloads the page when the fallback's button is clicked", async () => {
		silenceConsoleError();
		// jsdom's Location.reload is unforgeable, so the whole location
		// object is replaced for the duration of the click.
		const reload = vi.fn();
		const locationDescriptor = Object.getOwnPropertyDescriptor(
			window,
			"location",
		);
		Object.defineProperty(window, "location", {
			configurable: true,
			writable: true,
			value: { ...window.location, reload },
		});
		try {
			renderWithQuery(
				<ErrorBoundary>
					<Thrower />
				</ErrorBoundary>,
				{ chakra: true },
			);

			await userEvent.click(
				screen.getByRole("button", { name: /reload page/i }),
			);
			expect(reload).toHaveBeenCalledOnce();
		} finally {
			if (locationDescriptor) {
				Object.defineProperty(window, "location", locationDescriptor);
			}
		}
	});
});
