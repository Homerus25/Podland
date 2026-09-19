import { MachineSettingsTab } from "@features/machines/components/MachineSettingsTab";
import type { Machine } from "@shared/types/api";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import { machineFixture } from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

let updatePayload: Record<string, unknown> | undefined;

function grantsHandler() {
	// The admin fixture mounts the sharing section; pin its query so its
	// error alert cannot race the form's own error envelope.
	return http.get("*/api/machines/m-1/access", () => HttpResponse.json([]));
}

function updateHandler(
	machine: Machine,
	status = 200,
	body?: unknown,
	delayMs = 0,
) {
	return http.put(`*/api/machines/${machine.id}`, async ({ request }) => {
		if (delayMs) await new Promise((resolve) => setTimeout(resolve, delayMs));
		updatePayload = (await request.json()) as Record<string, unknown>;
		return HttpResponse.json(body ?? machine, { status });
	});
}

function renderSettingsTab(machine: Machine = machineFixture()) {
	server.use(grantsHandler());
	renderWithQuery(<MachineSettingsTab machine={machine} />, {
		router: true,
	});
}

async function editAndSave() {
	const user = userEvent.setup();
	const name = screen.getByLabelText(/^name/i);
	await user.clear(name);
	await user.type(name, "prod-web-02");
	await user.click(screen.getByTestId("machine-form-submit"));
}

describe("MachineSettingsTab (edit machine)", () => {
	it("shows the edit form prefilled with the machine metadata", () => {
		renderSettingsTab();

		expect((screen.getByLabelText(/^name/i) as HTMLInputElement).value).toBe(
			"prod-web-01",
		);
		expect(
			(screen.getByLabelText(/^description$/i) as HTMLInputElement).value,
		).toBe("Production web server");
		expect(
			(screen.getByLabelText(/hostname or ip/i) as HTMLInputElement).value,
		).toBe("web01.example.com");
		expect((screen.getByLabelText(/ssh port/i) as HTMLInputElement).value).toBe(
			"22",
		);
		expect(
			(screen.getByLabelText(/ssh username/i) as HTMLInputElement).value,
		).toBe("deploy");
	});

	it("persists edits and shows the saved banner without requiring the key file", async () => {
		const machine = machineFixture();
		server.use(updateHandler(machine));
		renderSettingsTab(machine);

		await editAndSave();

		await waitFor(() =>
			expect(screen.getByTestId("machine-saved")).toBeInTheDocument(),
		);
		expect(updatePayload).toMatchObject({
			name: "prod-web-02",
			description: "Production web server",
			hostname: "web01.example.com",
			port: 22,
			sshUsername: "deploy",
		});
		expect(updatePayload).not.toHaveProperty("sshPrivateKey");
	});

	it("sends an uploaded key file in the update request body", async () => {
		const machine = machineFixture();
		server.use(updateHandler(machine));
		renderSettingsTab(machine);
		const user = userEvent.setup();

		const keyFile = new File(
			["-----BEGIN OPENSSH PRIVATE KEY-----"],
			"id_ed25519",
		);
		await user.upload(screen.getByLabelText(/ssh private key/i), keyFile);
		await editAndSave();

		await waitFor(() =>
			expect(screen.getByTestId("machine-saved")).toBeInTheDocument(),
		);
		expect(updatePayload).toMatchObject({
			sshPrivateKey: "-----BEGIN OPENSSH PRIVATE KEY-----",
		});
	});

	it("renders the error envelope on the form when the save fails", async () => {
		const machine = machineFixture();
		server.use(
			updateHandler(
				machine,
				409,
				{
					requestId: "req-4",
					error: {
						message: "Machine name already exists",
						code: "MACHINE_NAME_TAKEN",
					},
				},
				50,
			),
		);
		renderSettingsTab(machine);

		await editAndSave();

		expect(screen.getByTestId("machine-form-submit")).toBeDisabled();
		expect(await screen.findByRole("alert")).toBeInTheDocument();
		expect(screen.getByText("MACHINE_NAME_TAKEN")).toBeInTheDocument();
		expect(screen.getByText("Machine name already exists")).toBeInTheDocument();
	});
});
