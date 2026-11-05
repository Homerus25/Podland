import { HostSettingsTab } from "@features/hosts/components/HostSettingsTab";
import type { Host, HostPermission } from "@shared/types/api";
import { screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { hostFixture } from "@/test/factories";
import { renderWithQuery } from "@/test/render";

function renderSettingsTab(host: Host) {
	renderWithQuery(<HostSettingsTab host={host} />, { router: true });
}

function granteeWith(permission: HostPermission): Host {
	return hostFixture({ permission, isOwner: false });
}

describe("permission gating on the host settings tab", () => {
	it("shows the edit form, sharing, and deletion to the host's owner", () => {
		renderSettingsTab(hostFixture());

		expect(screen.getByTestId("host-form-submit")).toBeInTheDocument();
		expect(screen.getByTestId("delete-host-button")).toBeInTheDocument();
		expect(screen.getByTestId("share-host-section")).toBeInTheDocument();
		expect(
			screen.queryByTestId("settings-permission-hint"),
		).not.toBeInTheDocument();
	});

	it.each(["read", "write", "admin"] as const)(
		"shows %s grantees exactly one truthful hint instead of the owner-only controls",
		(permission) => {
			renderSettingsTab(granteeWith(permission));

			expect(screen.queryByTestId("host-form-submit")).not.toBeInTheDocument();
			expect(
				screen.queryByTestId("delete-host-button"),
			).not.toBeInTheDocument();
			expect(
				screen.queryByTestId("share-host-section"),
			).not.toBeInTheDocument();

			const hints = screen.getAllByTestId("settings-permission-hint");
			expect(hints).toHaveLength(1);
			expect(hints[0]).toHaveAttribute("data-requirement", "owner");
			expect(hints[0]).toHaveTextContent("Only the owner can edit this host.");
		},
	);
});
