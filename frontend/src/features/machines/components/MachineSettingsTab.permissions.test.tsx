import { MachineSettingsTab } from "@features/machines/components/MachineSettingsTab";
import type { MachinePermission } from "@shared/types/api";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it } from "vitest";
import { machineFixture } from "@/test/factories";

function renderSettingsTab(permission: MachinePermission) {
	const queryClient = new QueryClient({
		defaultOptions: { queries: { retry: false } },
	});
	render(
		<QueryClientProvider client={queryClient}>
			<MemoryRouter>
				<MachineSettingsTab machine={machineFixture({ permission })} />
			</MemoryRouter>
		</QueryClientProvider>,
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
