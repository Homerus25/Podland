import { MachineSettingsTab } from "@features/machines/components/MachineSettingsTab";
import type { Machine, MachinePermission } from "@shared/types/api";
import { screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { machineFixture } from "@/test/factories";
import { renderWithQuery } from "@/test/render";

function renderSettingsTab(machine: Machine) {
	renderWithQuery(<MachineSettingsTab machine={machine} />, { router: true });
}

function granteeWith(permission: MachinePermission): Machine {
	return machineFixture({ permission, isOwner: false });
}

describe("permission gating on the machine settings tab", () => {
	it("shows the edit form, sharing, and deletion to the machine's owner", () => {
		renderSettingsTab(machineFixture());

		expect(screen.getByTestId("machine-form-submit")).toBeInTheDocument();
		expect(screen.getByTestId("delete-machine-button")).toBeInTheDocument();
		expect(screen.getByTestId("share-machine-section")).toBeInTheDocument();
		expect(
			screen.queryByTestId("settings-permission-hint"),
		).not.toBeInTheDocument();
	});

	it.each(["read", "write", "admin"] as const)(
		"shows %s grantees exactly one truthful hint instead of the owner-only controls",
		(permission) => {
			renderSettingsTab(granteeWith(permission));

			expect(
				screen.queryByTestId("machine-form-submit"),
			).not.toBeInTheDocument();
			expect(
				screen.queryByTestId("delete-machine-button"),
			).not.toBeInTheDocument();
			expect(
				screen.queryByTestId("share-machine-section"),
			).not.toBeInTheDocument();

			const hints = screen.getAllByTestId("settings-permission-hint");
			expect(hints).toHaveLength(1);
			expect(hints[0]).toHaveAttribute("data-requirement", "owner");
			expect(hints[0]).toHaveTextContent(
				"Only the owner can edit this machine.",
			);
		},
	);
});
