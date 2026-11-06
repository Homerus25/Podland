import type { UserRole } from "@shared/types/api";
import { setBearerToken } from "@shared/utils/api";
import { act, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { afterEach, describe, expect, it } from "vitest";
import { renderApp } from "@/test/auth";
import { hostFixture, hostVersionFixture } from "@/test/factories";
import { server } from "@/test/server";
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
		expect(screen.getByRole("link", { name: "Hosts" })).toBeInTheDocument();
		expect(
			screen.queryByRole("button", { name: "Toggle navigation" }),
		).not.toBeInTheDocument();

		await userEvent.click(screen.getByRole("link", { name: "Hosts" }));

		expect(
			await screen.findByRole("heading", { name: "Hosts" }),
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
		expect(screen.getByRole("link", { name: "Hosts" })).toBeInTheDocument();

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

		await userEvent.click(screen.getByRole("link", { name: "Hosts" }));

		expect(toggle).toHaveAttribute("aria-expanded", "false");
		expect(
			await screen.findByRole("heading", { name: "Hosts" }),
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

describe("shell titles (issue 459)", () => {
	const originalMatchMedia = window.matchMedia;

	afterEach(() => {
		window.matchMedia = originalMatchMedia;
	});

	it("shows the app title in the sidebar, never in the header", async () => {
		mockViewport(true);
		renderShell("/");

		expect(
			await screen.findByRole("heading", { name: "Dashboard" }),
		).toBeInTheDocument();
		// The sidebar owns the app title; the header's left slot is the
		// page title.
		expect(
			screen.getByRole("heading", { name: "Podland" }),
		).toBeInTheDocument();
		expect(screen.getByRole("banner")).not.toHaveTextContent("Podland");
		expect(screen.getByRole("banner")).toHaveTextContent("Dashboard");
	});

	it("shows the app title in the mobile drawer above the links", async () => {
		mockViewport(false);
		renderShell("/");

		expect(
			await screen.findByRole("heading", { name: "Dashboard" }),
		).toBeInTheDocument();
		expect(
			screen.queryByRole("heading", { name: "Podland" }),
		).not.toBeInTheDocument();

		await userEvent.click(
			screen.getByRole("button", { name: "Toggle navigation" }),
		);

		expect(
			screen.getByRole("heading", { name: "Podland" }),
		).toBeInTheDocument();
		expect(screen.getByRole("banner")).not.toHaveTextContent("Podland");
	});

	it("shows the page title in the header on the app-level pages", async () => {
		mockViewport(true);
		renderShell("/users", "alice", "admin");

		expect(
			await screen.findByRole("heading", { name: "Users" }),
		).toBeInTheDocument();
		expect(screen.getByRole("banner")).toHaveTextContent("Users");
	});

	it("shows the host identity in the header and the host tabs in the sidebar on a host-scoped route", async () => {
		mockViewport(true);
		const host = hostFixture();
		server.use(
			http.get(`*/api/hosts/${host.id}`, () => HttpResponse.json(host)),
			http.get(`*/api/hosts/${host.id}/containers`, () =>
				HttpResponse.json({
					containers: [],
					total: 0,
					timestamp: "2026-08-26T00:00:00Z",
				}),
			),
			http.get(`*/api/hosts/${host.id}/version`, () =>
				HttpResponse.json(
					hostVersionFixture({
						podmanVersion: null,
						apiVersion: null,
					}),
				),
			),
		);
		renderShell(`/hosts/${host.id}`);

		const banner = await screen.findByRole("banner");
		expect(await within(banner).findByText(host.name)).toBeInTheDocument();
		expect(within(banner).getByTestId("host-endpoint")).toBeInTheDocument();
		expect(within(banner).getByTestId("host-ssh-user")).toBeInTheDocument();
		expect(within(banner).getByTestId("host-permission")).toHaveTextContent(
			"admin",
		);

		// The Hosts main link is active only on the list page; on the
		// host-scoped route the matching host tab carries the active
		// state instead.
		expect(screen.getByRole("link", { name: "Hosts" })).not.toHaveAttribute(
			"aria-current",
		);
		expect(screen.getByRole("link", { name: "Containers" })).toHaveAttribute(
			"aria-current",
			"page",
		);
		expect(banner).not.toHaveTextContent("Podland");
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
