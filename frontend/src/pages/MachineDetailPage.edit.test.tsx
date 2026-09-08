import type { Machine } from "@shared/types/api";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import { renderApp } from "@/test/auth";
import { machineFixture } from "@/test/factories";
import { server } from "@/test/server";

let updatePayload: Record<string, unknown> | undefined;

function machineHandler(machine: Machine) {
	return http.get(`*/api/machines/${machine.id}`, () =>
		HttpResponse.json(machine),
	);
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

async function renderSettingsTab(
	machine: Machine = machineFixture({ permission: "write" }),
) {
	server.use(machineHandler(machine));
	renderApp(`/machines/${machine.id}`, {
		resolveInitialState: async () => ({ username: "alice" }),
	});
	await screen.findByText(machine.name);
	await userEvent.click(screen.getByRole("tab", { name: /settings/i }));
}

async function editAndSave() {
	const name = screen.getByLabelText(/^name/i);
	await userEvent.clear(name);
	await userEvent.type(name, "prod-web-02");
	await userEvent.click(screen.getByTestId("machine-form-submit"));
}

describe("MachineDetailPage Settings tab (edit machine)", () => {
	it("shows the edit form prefilled with the machine metadata", async () => {
		await renderSettingsTab();

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

	it("persists edits without requiring the key file", async () => {
		server.use(updateHandler(machineFixture({ permission: "write" })));
		await renderSettingsTab();

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
		server.use(updateHandler(machineFixture({ permission: "write" })));
		await renderSettingsTab();

		const keyFile = new File(
			["-----BEGIN OPENSSH PRIVATE KEY-----"],
			"id_ed25519",
		);
		await userEvent.upload(screen.getByLabelText(/ssh private key/i), keyFile);
		await editAndSave();

		await waitFor(() =>
			expect(screen.getByTestId("machine-saved")).toBeInTheDocument(),
		);
		expect(updatePayload).toMatchObject({
			sshPrivateKey: "-----BEGIN OPENSSH PRIVATE KEY-----",
		});
	});

	it("renders the error envelope when the save fails", async () => {
		server.use(
			updateHandler(
				machineFixture({ permission: "write" }),
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
		await renderSettingsTab();

		await editAndSave();

		expect(screen.getByTestId("machine-form-submit")).toBeDisabled();
		expect(await screen.findByRole("alert")).toBeInTheDocument();
		expect(screen.getByText("MACHINE_NAME_TAKEN")).toBeInTheDocument();
		expect(screen.getByText("Machine name already exists")).toBeInTheDocument();
	});

	it("hides the edit form for read-permission users", async () => {
		await renderSettingsTab(machineFixture({ permission: "read" }));

		expect(screen.queryByLabelText(/^name/i)).not.toBeInTheDocument();
		expect(screen.queryByTestId("machine-form-submit")).not.toBeInTheDocument();
	});
});
