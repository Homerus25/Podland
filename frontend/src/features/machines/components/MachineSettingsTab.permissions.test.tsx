import { MachineSettingsTab } from "@features/machines/components/MachineSettingsTab";
import type { MachinePermission } from "@shared/types/api";
import { screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { machineFixture } from "@/test/factories";
import { renderWithQuery } from "@/test/render";

function renderSettingsTab(permission: MachinePermission) {
	renderWithQuery(
		<MachineSettingsTab machine={machineFixture({ permission })} />,
		{ router: true },
	);
}

describe("permission gating on the machine settings tab", () => {
	it("shows all controls to admin users", () => {
		renderSettingsTab("admin");

		expect(screen.getByTestId("machine-form-submit")).toBeInTheDocument();
		expect(screen.getByTestId("delete-machine-button")).toBeInTheDocument();
		expect(screen.getByTestId("share-machine-section")).toBeInTheDocument();
		expect(screen.queryByTestId("permission-hint")).not.toBeInTheDocument();
	});

	it("lets write users edit metadata but hides sharing and delete with an explanation", () => {
		renderSettingsTab("write");

		expect(screen.getByTestId("machine-form-submit")).toBeInTheDocument();
		expect(
			screen.queryByTestId("delete-machine-button"),
		).not.toBeInTheDocument();
		expect(
			screen.queryByTestId("share-machine-section"),
		).not.toBeInTheDocument();
		expect(screen.getByTestId("permission-hint")).toHaveAttribute(
			"data-permission",
			"write",
		);
	});

	it("hides all write controls from read users with an explanation", () => {
		renderSettingsTab("read");

		expect(screen.queryByTestId("machine-form-submit")).not.toBeInTheDocument();
		expect(screen.queryByTestId("machine-saved")).not.toBeInTheDocument();
		expect(
			screen.queryByTestId("delete-machine-button"),
		).not.toBeInTheDocument();
		expect(
			screen.queryByTestId("share-machine-section"),
		).not.toBeInTheDocument();
		expect(screen.getByTestId("permission-hint")).toHaveAttribute(
			"data-permission",
			"read",
		);
	});
});
