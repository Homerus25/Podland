import {
	HostStatusBadge,
	hostStatusColorScheme,
} from "@shared/components/HostStatusBadge";
import { fireEvent, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { renderWithQuery } from "@/test/render";

function renderBadge(props: Parameters<typeof HostStatusBadge>[0]) {
	return renderWithQuery(<HostStatusBadge {...props} />, {
		chakra: true,
	});
}

describe("HostStatusBadge", () => {
	it.each([
		["online", "green"],
		["offline", "red"],
		["checking", "gray"],
		["unknown", "gray"],
	])("maps %s to %s", (status, scheme) => {
		expect(hostStatusColorScheme(status)).toBe(scheme);
	});

	it("renders online with its label", () => {
		renderBadge({ status: "online" });

		const badge = screen.getByTestId("host-status-badge");
		expect(badge).toHaveTextContent("online");
		expect(badge.className).toContain("chakra-badge");
	});

	it("renders offline with its label", () => {
		renderBadge({ status: "offline" });

		expect(screen.getByTestId("host-status-badge")).toHaveTextContent(
			"offline",
		);
	});

	it("renders checking with its label", () => {
		renderBadge({ status: "checking" });

		expect(screen.getByTestId("host-status-badge")).toHaveTextContent(
			"checking",
		);
	});

	it("renders unknown with its label", () => {
		renderBadge({ status: "unknown" });

		expect(screen.getByTestId("host-status-badge")).toHaveTextContent(
			"unknown",
		);
	});

	it("always shows the text label next to the colored dot", () => {
		renderBadge({ status: "online" });

		expect(screen.getByTestId("host-status-badge-dot")).toBeInTheDocument();
		expect(screen.getByTestId("host-status-badge-label")).toHaveTextContent(
			"online",
		);
	});

	it("exposes checkedAt as a tooltip", async () => {
		renderBadge({ status: "online", checkedAt: "2026-08-29T10:00:00Z" });

		fireEvent.focus(screen.getByTestId("host-status-badge"));
		expect(await screen.findByRole("tooltip")).toHaveTextContent(
			"Checked at 2026-08-29T10:00:00Z",
		);
		expect(screen.getByTestId("host-status-badge")).toHaveAttribute(
			"aria-label",
			"Checked at 2026-08-29T10:00:00Z",
		);
	});

	it("has no tooltip without checkedAt", () => {
		renderBadge({ status: "checking" });

		expect(screen.getByTestId("host-status-badge")).not.toHaveAttribute(
			"aria-label",
		);
	});
});
