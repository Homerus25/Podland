import { screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { renderWithQuery } from "@/test/render";
import { HostNavSection } from "./HostNavSection";

// The section reads the route through useHostNavScope, so each case is
// one router entry; the scope matrix itself is pinned in hostNav.test.ts.
function renderSection(at: string) {
	return renderWithQuery(<HostNavSection />, {
		router: { initialEntries: [at] },
		chakra: true,
	});
}

const TAB_LABELS = [
	"Containers",
	"Volumes",
	"Images",
	"Networks",
	"Quadlets",
	"Pods",
	"Secrets",
	"Settings",
] as const;

describe("HostNavSection", () => {
	it("renders nothing off the host-scoped routes", () => {
		renderSection("/hosts");
		for (const label of TAB_LABELS) {
			expect(
				screen.queryByRole("link", { name: label }),
			).not.toBeInTheDocument();
		}
	});

	it("lists the eight host tabs in deep-link order below the divider", () => {
		renderSection("/hosts/m-1");

		expect(screen.getAllByRole("link").map((link) => link.textContent)).toEqual(
			[...TAB_LABELS],
		);
		expect(screen.getByRole("link", { name: "Volumes" })).toHaveAttribute(
			"href",
			"/hosts/m-1?tab=volumes",
		);
	});

	it("marks the ?tab= param's tab active on the host detail page", () => {
		renderSection("/hosts/m-1?tab=volumes");

		expect(screen.getByRole("link", { name: "Volumes" })).toHaveAttribute(
			"aria-current",
			"page",
		);
		expect(
			screen.getByRole("link", { name: "Containers" }),
		).not.toHaveAttribute("aria-current");
	});

	it("marks the sub-page's resource tab active", () => {
		renderSection("/hosts/m-1/containers/c-1");

		expect(screen.getByRole("link", { name: "Containers" })).toHaveAttribute(
			"aria-current",
			"page",
		);
	});

	it("marks the target kind's tab active on the logs page", () => {
		renderSection("/hosts/m-1/logs/quadlet/q-1");

		expect(screen.getByRole("link", { name: "Quadlets" })).toHaveAttribute(
			"aria-current",
			"page",
		);
	});
});
