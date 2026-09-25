import { MachineSettingsTab } from "@features/machines/components/MachineSettingsTab";
import type { Machine } from "@shared/types/api";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import { errorJson } from "@/test/errors";
import { machineFixture } from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

let updatePayload: Record<string, unknown> | undefined;

function grantsHandler() {
	// The admin fixture mounts the sharing section; pin its queries so its
	// error alerts (access list, and the picker's user directory) cannot
	// race the form's own error envelope.
	return [
		http.get("*/api/machines/m-1/access", () => HttpResponse.json([])),
		http.get("*/api/users", () => HttpResponse.json([])),
	];
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
	server.use(...grantsHandler());
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
				errorJson("MACHINE_NAME_TAKEN", "Machine name already exists", "req-4"),
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

	it("cancel reverts the fields to the stored values and clears the saved banner", async () => {
		const machine = machineFixture();
		server.use(updateHandler(machine));
		renderSettingsTab(machine);
		const user = userEvent.setup();
		const name = screen.getByLabelText(/^name/i);

		await editAndSave();
		await waitFor(() =>
			expect(screen.getByTestId("machine-saved")).toBeInTheDocument(),
		);

		// The stale-banner scenario: further edits sit under a banner from the
		// earlier save; cancel must clear both at once.
		await user.clear(name);
		await user.type(name, "prod-web-03");
		await user.click(screen.getByTestId("machine-form-cancel"));

		expect((screen.getByLabelText(/^name/i) as HTMLInputElement).value).toBe(
			"prod-web-01",
		);
		expect(screen.queryByTestId("machine-saved")).not.toBeInTheDocument();
	});

	it("cancel clears the error envelope and reverts the fields", async () => {
		const machine = machineFixture();
		server.use(
			updateHandler(
				machine,
				409,
				errorJson("MACHINE_NAME_TAKEN", "Machine name already exists", "req-5"),
				50,
			),
		);
		renderSettingsTab(machine);
		const user = userEvent.setup();

		await editAndSave();
		expect(await screen.findByRole("alert")).toBeInTheDocument();

		await user.click(screen.getByTestId("machine-form-cancel"));

		expect(screen.queryByRole("alert")).not.toBeInTheDocument();
		expect((screen.getByLabelText(/^name/i) as HTMLInputElement).value).toBe(
			"prod-web-01",
		);
	});

	it("cancel stays enabled mid-save and the late settle never restores the saved banner", async () => {
		const machine = machineFixture();
		server.use(updateHandler(machine, 200, undefined, 300));
		renderSettingsTab(machine);
		const user = userEvent.setup();
		updatePayload = undefined;

		await editAndSave();

		expect(screen.getByTestId("machine-form-submit")).toBeDisabled();
		expect(screen.getByTestId("machine-form-cancel")).toBeEnabled();
		await user.click(screen.getByTestId("machine-form-cancel"));
		expect((screen.getByLabelText(/^name/i) as HTMLInputElement).value).toBe(
			"prod-web-01",
		);

		// The PUT still lands server-side; its success must not resurrect
		// the banner over the reverted fields.
		await waitFor(() =>
			expect(updatePayload).toMatchObject({ name: "prod-web-02" }),
		);
		expect(screen.queryByTestId("machine-saved")).not.toBeInTheDocument();
	});

	it("cancel during a failing save keeps the error envelope from returning", async () => {
		const machine = machineFixture();
		server.use(
			updateHandler(
				machine,
				409,
				errorJson("MACHINE_NAME_TAKEN", "Machine name already exists", "req-6"),
				300,
			),
		);
		renderSettingsTab(machine);
		const user = userEvent.setup();
		updatePayload = undefined;

		await editAndSave();
		await user.click(screen.getByTestId("machine-form-cancel"));

		await waitFor(() =>
			expect(updatePayload).toMatchObject({ name: "prod-web-02" }),
		);
		expect(screen.queryByRole("alert")).not.toBeInTheDocument();
		expect((screen.getByLabelText(/^name/i) as HTMLInputElement).value).toBe(
			"prod-web-01",
		);
	});
});
