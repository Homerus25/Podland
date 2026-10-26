import type { UserRole } from "@shared/types/api";
import { setBearerToken } from "@shared/utils/api";
import { act, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import { renderApp } from "@/test/auth";
import { mockViewport } from "@/test/viewport";

function renderShell(at = "/", username = "alice", role: UserRole = "user") {
	return renderApp(at, {
		resolveInitialState: async () => ({ username, role }),
		usernameProbe: false,
	});
}

afterEach(() => {
	setBearerToken(null);
});

const prefersDark = (query: string) => query.includes("dark");

function mockSystemPreference(dark: boolean) {
	window.matchMedia = (query: string) => ({
		matches: dark && prefersDark(query),
		media: query,
		onchange: null,
		addListener: () => {},
		removeListener: () => {},
		addEventListener: () => {},
		removeEventListener: () => {},
		dispatchEvent: () => false,
	});
}

describe("app shell", () => {
	const originalMatchMedia = window.matchMedia;

	afterEach(() => {
		window.matchMedia = originalMatchMedia;
	});

	it("renders the header with username and sidebar navigation around page content", async () => {
		mockViewport(true);
		renderShell("/");

		expect(
			await screen.findByRole("heading", { name: "Dashboard" }),
		).toBeInTheDocument();
		expect(screen.getByText("alice")).toBeInTheDocument();
		expect(screen.getByRole("link", { name: "Dashboard" })).toBeInTheDocument();
		expect(screen.getByRole("link", { name: "Machines" })).toBeInTheDocument();
		expect(
			screen.queryByRole("button", { name: "Toggle navigation" }),
		).not.toBeInTheDocument();

		await userEvent.click(screen.getByRole("link", { name: "Machines" }));

		expect(
			await screen.findByRole("heading", { name: "Machines" }),
		).toBeInTheDocument();
	});
});

describe("mobile navigation drawer", () => {
	const originalMatchMedia = window.matchMedia;

	afterEach(() => {
		window.matchMedia = originalMatchMedia;
	});

	async function openDrawer() {
		const toggle = screen.getByRole("button", { name: "Toggle navigation" });
		await userEvent.click(toggle);
		return toggle;
	}

	it("hides the sidebar below the breakpoint and toggles the drawer through the hamburger", async () => {
		mockViewport(false);
		renderShell("/");

		expect(
			await screen.findByRole("heading", { name: "Dashboard" }),
		).toBeInTheDocument();
		const toggle = screen.getByRole("button", { name: "Toggle navigation" });
		expect(toggle).toHaveAttribute("aria-expanded", "false");
		expect(
			screen.queryByRole("link", { name: "Dashboard" }),
		).not.toBeInTheDocument();

		await userEvent.click(toggle);

		expect(toggle).toHaveAttribute("aria-expanded", "true");
		expect(screen.getByRole("link", { name: "Dashboard" })).toBeInTheDocument();
		expect(screen.getByRole("link", { name: "Machines" })).toBeInTheDocument();

		await userEvent.click(toggle);

		expect(toggle).toHaveAttribute("aria-expanded", "false");
		expect(
			screen.queryByRole("link", { name: "Dashboard" }),
		).not.toBeInTheDocument();
	});

	it("closes on a tapped nav item and navigates", async () => {
		mockViewport(false);
		renderShell("/");

		expect(
			await screen.findByRole("heading", { name: "Dashboard" }),
		).toBeInTheDocument();
		const toggle = await openDrawer();

		await userEvent.click(screen.getByRole("link", { name: "Machines" }));

		expect(toggle).toHaveAttribute("aria-expanded", "false");
		expect(
			await screen.findByRole("heading", { name: "Machines" }),
		).toBeInTheDocument();
	});

	it("closes on a tapped overlay", async () => {
		mockViewport(false);
		renderShell("/");

		expect(
			await screen.findByRole("heading", { name: "Dashboard" }),
		).toBeInTheDocument();
		const toggle = await openDrawer();

		await userEvent.click(screen.getByTestId("nav-drawer-overlay"));

		expect(toggle).toHaveAttribute("aria-expanded", "false");
		expect(
			screen.queryByRole("link", { name: "Dashboard" }),
		).not.toBeInTheDocument();
	});

	it("closes on Escape", async () => {
		mockViewport(false);
		renderShell("/");

		expect(
			await screen.findByRole("heading", { name: "Dashboard" }),
		).toBeInTheDocument();
		const toggle = await openDrawer();

		await userEvent.keyboard("{Escape}");

		expect(toggle).toHaveAttribute("aria-expanded", "false");
		expect(
			screen.queryByRole("link", { name: "Dashboard" }),
		).not.toBeInTheDocument();
	});

	it("shows the Users entry in the drawer only for admins", async () => {
		mockViewport(false);
		const adminShell = renderShell("/", "alice", "admin");

		expect(
			await screen.findByRole("heading", { name: "Dashboard" }),
		).toBeInTheDocument();
		await openDrawer();
		expect(screen.getByRole("link", { name: "Users" })).toBeInTheDocument();
		adminShell.unmount();

		renderShell("/", "alice", "user");

		expect(
			await screen.findByRole("heading", { name: "Dashboard" }),
		).toBeInTheDocument();
		await openDrawer();
		expect(
			screen.queryByRole("link", { name: "Users" }),
		).not.toBeInTheDocument();
	});

	it("stops rendering the drawer past the breakpoint and does not resurrect it", async () => {
		const viewport = mockViewport(false);
		renderShell("/");

		expect(
			await screen.findByRole("heading", { name: "Dashboard" }),
		).toBeInTheDocument();
		await openDrawer();
		expect(screen.getByTestId("nav-drawer-overlay")).toBeInTheDocument();

		// The listener fires outside user event wrappers, so the state
		// update it triggers needs act to flush before asserting.
		act(() => {
			viewport.resize(true);
		});

		expect(
			screen.queryByRole("button", { name: "Toggle navigation" }),
		).not.toBeInTheDocument();
		expect(screen.queryByTestId("nav-drawer-overlay")).not.toBeInTheDocument();
		expect(screen.getByRole("link", { name: "Dashboard" })).toBeInTheDocument();

		act(() => {
			viewport.resize(false);
		});

		const toggleAfterResize = screen.getByRole("button", {
			name: "Toggle navigation",
		});
		expect(toggleAfterResize).toHaveAttribute("aria-expanded", "false");
		expect(
			screen.queryByRole("link", { name: "Dashboard" }),
		).not.toBeInTheDocument();
	});

	it("keeps the drawer's open state transient", async () => {
		mockViewport(false);
		renderShell("/");

		expect(
			await screen.findByRole("heading", { name: "Dashboard" }),
		).toBeInTheDocument();
		const keysBefore = [
			...Object.keys(window.localStorage),
			...Object.keys(window.sessionStorage),
		]
			.sort()
			.join(",");

		await openDrawer();

		expect(screen.getByRole("link", { name: "Dashboard" })).toBeInTheDocument();
		expect(
			[
				...Object.keys(window.localStorage),
				...Object.keys(window.sessionStorage),
			]
				.sort()
				.join(","),
		).toBe(keysBefore);
	});
});

describe("color mode", () => {
	const originalMatchMedia = window.matchMedia;

	afterEach(() => {
		window.matchMedia = originalMatchMedia;
		localStorage.clear();
		document.body.classList.remove("chakra-ui-light", "chakra-ui-dark");
	});

	it("flips light/dark on toggle and persists the choice in localStorage", async () => {
		localStorage.clear();
		mockSystemPreference(false);
		renderShell("/");

		expect(
			await screen.findByRole("heading", { name: "Dashboard" }),
		).toBeInTheDocument();
		expect(document.body.classList.contains("chakra-ui-light")).toBe(true);
		expect(localStorage.getItem("chakra-ui-color-mode")).toBe("light");

		await userEvent.click(
			screen.getByRole("button", { name: "Switch to dark mode" }),
		);

		expect(document.body.classList.contains("chakra-ui-dark")).toBe(true);
		expect(document.documentElement.dataset.theme).toBe("dark");
		expect(localStorage.getItem("chakra-ui-color-mode")).toBe("dark");
		expect(
			screen.getByRole("button", { name: "Switch to light mode" }),
		).toBeInTheDocument();
	});

	it("defaults to the system color preference when no choice is stored", async () => {
		localStorage.clear();
		mockSystemPreference(true);
		renderShell("/");

		expect(
			await screen.findByRole("heading", { name: "Dashboard" }),
		).toBeInTheDocument();
		expect(document.body.classList.contains("chakra-ui-dark")).toBe(true);
		expect(document.documentElement.dataset.theme).toBe("dark");
	});
});
