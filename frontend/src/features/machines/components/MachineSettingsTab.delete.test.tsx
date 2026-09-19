import { MachineSettingsTab } from "@features/machines/components/MachineSettingsTab";
import { useMachines } from "@features/machines/hooks/useMachines";
import type { Machine } from "@shared/types/api";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { useLocation } from "react-router-dom";
import { beforeEach, describe, expect, it } from "vitest";
import { machineFixture } from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

let deleteRequested = false;
let machinesListRequests = 0;

function deleteHandler(status = 204, delayMs = 0) {
	return http.delete("*/api/machines/m-1", async () => {
		if (delayMs) await new Promise((resolve) => setTimeout(resolve, delayMs));
		if (status !== 204) {
			return HttpResponse.json(
				{
					requestId: "req-5",
					error: { message: "Failed to delete machine", code: "DELETE_FAILED" },
				},
				{ status },
			);
		}
		deleteRequested = true;
		return new HttpResponse(null, { status: 204 });
	});
}

function machinesListHandler() {
	return http.get("*/api/machines", () => {
		machinesListRequests++;
		return HttpResponse.json([machineFixture()]);
	});
}

// Delete success invalidates the machines list; an active observer is needed
// so the invalidation actually refetches through MSW.
function MachinesListProbe() {
	useMachines();
	return null;
}

function LocationProbe() {
	const location = useLocation();
	return <div data-testid="location">{location.pathname}</div>;
}

function renderSettingsTab(machine: Machine = machineFixture()) {
	renderWithQuery(
		<>
			<LocationProbe />
			<MachinesListProbe />
			<MachineSettingsTab machine={machine} />
		</>,
		{ router: { initialEntries: ["/machines/m-1"] } },
	);
}

async function openDeleteModal() {
	const user = userEvent.setup();
	await user.click(screen.getByTestId("delete-machine-button"));
	expect(await screen.findByTestId("modal")).toBeInTheDocument();
	return user;
}

describe("MachineSettingsTab (delete machine)", () => {
	beforeEach(() => {
		deleteRequested = false;
		machinesListRequests = 0;
	});

	it("fires no delete request on cancel", async () => {
		server.use(deleteHandler(), machinesListHandler());
		renderSettingsTab();
		const user = await openDeleteModal();

		await user.click(screen.getByRole("button", { name: /cancel/i }));

		await waitFor(() =>
			expect(screen.queryByTestId("modal")).not.toBeInTheDocument(),
		);
		expect(deleteRequested).toBe(false);
	});

	it("deletes, refetches the machines list, and navigates to /machines on success", async () => {
		server.use(deleteHandler(), machinesListHandler());
		renderSettingsTab();
		const user = await openDeleteModal();
		const requestsBefore = machinesListRequests;

		await user.click(screen.getByTestId("confirm-delete-button"));

		await waitFor(() => expect(deleteRequested).toBe(true));
		expect(await screen.findByTestId("location")).toHaveTextContent(
			"/machines",
		);
		// The delete mutation invalidated the machines list query, whose
		// observer refetched through MSW.
		await waitFor(() =>
			expect(machinesListRequests).toBeGreaterThan(requestsBefore),
		);
	});

	it("renders the error envelope inside the modal, keeps it open, and navigates nowhere on failure", async () => {
		server.use(deleteHandler(409, 50), machinesListHandler());
		renderSettingsTab();
		const user = await openDeleteModal();

		await user.click(screen.getByTestId("confirm-delete-button"));

		expect(screen.getByTestId("confirm-delete-button")).toBeDisabled();
		const modal = screen.getByTestId("modal");
		expect(await within(modal).findByRole("alert")).toBeInTheDocument();
		expect(within(modal).getByText("DELETE_FAILED")).toBeInTheDocument();
		expect(
			within(modal).getByText("Failed to delete machine"),
		).toBeInTheDocument();
		// Normalized failure: the modal stays open for another attempt (the
		// confirm button re-enables) and the tab behind is untouched.
		await waitFor(() =>
			expect(screen.getByTestId("confirm-delete-button")).toBeEnabled(),
		);
		expect(screen.getByTestId("modal")).toBeInTheDocument();
		expect(screen.getByTestId("location")).toHaveTextContent("/machines/m-1");
	});
});
