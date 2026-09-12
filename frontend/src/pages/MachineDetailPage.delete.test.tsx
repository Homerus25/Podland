import type { Machine } from "@shared/types/api";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import { renderApp } from "@/test/auth";
import { machineFixture } from "@/test/factories";
import { server } from "@/test/server";

let deleteRequested = false;
let machinesListRequests = 0;

function machineHandler(machine: Machine) {
	return http.get(`*/api/machines/${machine.id}`, () =>
		HttpResponse.json(machine),
	);
}

function deleteHandler(machine: Machine, status = 204, delayMs = 0) {
	return http.delete(`*/api/machines/${machine.id}`, async () => {
		if (delayMs) await new Promise((resolve) => setTimeout(resolve, delayMs));
		deleteRequested = true;
		if (status === 200) return new HttpResponse(null, { status: 204 });
		return HttpResponse.json(
			{
				requestId: "req-5",
				error: { message: "Failed to delete machine", code: "DELETE_FAILED" },
			},
			{ status },
		);
	});
}

async function renderSettingsTab(machine: Machine = machineFixture()) {
	deleteRequested = false;
	server.use(machineHandler(machine));
	server.use(
		http.get("*/api/machines/m-1/access", () => HttpResponse.json([])),
	);
	server.use(
		http.get("*/api/machines", () => {
			machinesListRequests++;
			return HttpResponse.json([machine]);
		}),
	);
	renderApp(`/machines/${machine.id}`, {
		resolveInitialState: async () => ({ username: "alice" }),
	});
	await screen.findByText(machine.name);
	await userEvent.click(screen.getByRole("tab", { name: /settings/i }));
}

describe("MachineDetailPage Settings tab (delete machine)", () => {
	it("fires no delete request on cancel", async () => {
		server.use(deleteHandler(machineFixture()));
		await renderSettingsTab();

		await userEvent.click(screen.getByTestId("delete-machine-button"));
		expect(screen.getByTestId("modal")).toBeInTheDocument();
		await userEvent.click(screen.getByRole("button", { name: /cancel/i }));

		await waitFor(() =>
			expect(screen.queryByTestId("modal")).not.toBeInTheDocument(),
		);
		expect(deleteRequested).toBe(false);
	});

	it("deletes, refetches the machines list, and navigates back on success", async () => {
		server.use(deleteHandler(machineFixture()));
		await renderSettingsTab();

		await userEvent.click(screen.getByTestId("delete-machine-button"));
		await userEvent.click(
			screen.getByRole("button", { name: /delete machine/i }),
		);

		expect(
			await screen.findByText("Machines", { selector: "h1, h2" }),
		).toBeInTheDocument();
		await waitFor(() => expect(deleteRequested).toBe(true));
		expect(screen.getByTestId("location")).toHaveTextContent("/machines");
		expect(screen.getByTestId("location")).not.toHaveTextContent(
			"/machines/m-1",
		);
		// Navigating back mounts the machines list, whose query the delete
		// mutation invalidated — the list data is fetched fresh, not served stale.
		expect(machinesListRequests).toBeGreaterThanOrEqual(1);
	});

	it("renders the error envelope inside the modal, keeps it open, and navigates nowhere on failure", async () => {
		server.use(deleteHandler(machineFixture(), 409, 50));
		await renderSettingsTab();

		await userEvent.click(screen.getByTestId("delete-machine-button"));
		await userEvent.click(
			screen.getByRole("button", { name: /delete machine/i }),
		);

		expect(screen.getByTestId("confirm-delete-button")).toBeDisabled();
		const modal = screen.getByTestId("modal");
		expect(await within(modal).findByRole("alert")).toBeInTheDocument();
		expect(within(modal).getByText("DELETE_FAILED")).toBeInTheDocument();
		expect(
			within(modal).getByText("Failed to delete machine"),
		).toBeInTheDocument();
		// Normalized failure: the modal stays open for another attempt (the
		// confirm button re-enables) and the page behind is untouched.
		await waitFor(() =>
			expect(screen.getByTestId("confirm-delete-button")).toBeEnabled(),
		);
		expect(screen.getByTestId("modal")).toBeInTheDocument();
		expect(screen.getByTestId("location")).toHaveTextContent("/machines/m-1");
	});

	it.each(["write", "read"] as const)(
		"hides the delete action for %s-permission users",
		async (permission) => {
			await renderSettingsTab(machineFixture({ permission }));

			expect(
				screen.queryByTestId("delete-machine-button"),
			).not.toBeInTheDocument();
		},
	);
});
