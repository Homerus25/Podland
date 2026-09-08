import {
	canDeleteMachine,
	canEditMachine,
	canManageSecrets,
	canRevealSecrets,
	canShareMachine,
	effectivePermission,
} from "@features/machines/permissions";
import type { Machine, MachinePermission } from "@shared/types/api";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import { renderApp } from "@/test/auth";
import { machineFixture } from "@/test/factories";
import { server } from "@/test/server";

function mockMachineHandlers(machine: Machine) {
	server.use(
		http.get(`*/api/machines/${machine.id}`, () => HttpResponse.json(machine)),
	);
	server.use(
		http.get(`*/api/machines/${machine.id}/access`, () =>
			HttpResponse.json([]),
		),
	);
}

async function renderSettings(permission: MachinePermission) {
	const machine = machineFixture({ permission });
	mockMachineHandlers(machine);
	renderApp(`/machines/${machine.id}`, {
		resolveInitialState: async () => ({ username: "alice" }),
	});
	await screen.findByText(machine.name);
	await userEvent.click(screen.getByRole("tab", { name: /settings/i }));
}

describe("permission gating on the machine detail page", () => {
	it("shows all controls to admin users", async () => {
		await renderSettings("admin");

		expect(screen.getByTestId("machine-form-submit")).toBeInTheDocument();
		expect(screen.getByTestId("delete-machine-button")).toBeInTheDocument();
		expect(screen.getByTestId("share-machine-section")).toBeInTheDocument();
		expect(screen.queryByTestId("permission-hint")).not.toBeInTheDocument();
	});

	it("lets write users edit metadata but hides sharing and delete with an explanation", async () => {
		await renderSettings("write");

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

	it("hides all write controls from read users with an explanation", async () => {
		await renderSettings("read");

		expect(screen.queryByTestId("machine-form-submit")).not.toBeInTheDocument();
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

	it("derives write access from the machines list when the detail response omits permission", async () => {
		// The backend only populates `permission` on GET /api/machines; the
		// detail response omits it, so the page must merge the list's level in.
		server.use(
			http.get("*/api/machines", () => HttpResponse.json([machineFixture()])),
			http.get("*/api/machines/m-1", () => {
				const { permission: _permission, ...detail } = machineFixture();
				return HttpResponse.json(detail);
			}),
			http.get("*/api/machines/m-1/secrets", () => HttpResponse.json([])),
		);
		renderApp("/machines/m-1", {
			resolveInitialState: async () => ({ username: "alice" }),
		});
		await screen.findByText(machineFixture().name);
		await userEvent.click(screen.getByRole("tab", { name: /secrets/i }));

		await screen.findByTestId("create-secret-button");
		expect(
			screen.queryByTestId("secrets-permission-hint"),
		).not.toBeInTheDocument();
		expect(screen.getByTestId("machine-permission")).toHaveTextContent("admin");
	});

	it.each(["read", "write", "admin"] as const)(
		"offers the same View navigation on the machines list for %s users",
		async (permission) => {
			server.use(
				http.get("*/api/machines", () =>
					HttpResponse.json([machineFixture({ permission })]),
				),
			);
			renderApp("/machines", {
				resolveInitialState: async () => ({ username: "alice" }),
			});

			const card = await screen.findByTestId("machine-card");
			expect(card).toBeInTheDocument();

			// The list has no machine-scoped write actions, so there is nothing
			// for a read user to be baited with — assert no stray actions appear.
			const cardButtons = card.querySelectorAll("button");
			expect(cardButtons).toHaveLength(0);
		},
	);
});

describe("permissions module", () => {
	it("treats owner as admin via the backend-seeded permission and derives gating consistently", () => {
		const ownerMachine = machineFixture({ permission: "admin" });
		const permission = effectivePermission(ownerMachine);
		expect(permission).toBe("admin");
		expect(canEditMachine(permission)).toBe(true);
		expect(canShareMachine(permission)).toBe(true);
		expect(canDeleteMachine(permission)).toBe(true);

		expect(canEditMachine("read")).toBe(false);
		expect(canEditMachine("write")).toBe(true);
		expect(canShareMachine("write")).toBe(false);
		expect(canDeleteMachine("write")).toBe(false);
	});

	it("gates secret management at write and secret reveal at admin", () => {
		expect(canManageSecrets("read")).toBe(false);
		expect(canManageSecrets("write")).toBe(true);
		expect(canManageSecrets("admin")).toBe(true);

		expect(canRevealSecrets("read")).toBe(false);
		expect(canRevealSecrets("write")).toBe(false);
		expect(canRevealSecrets("admin")).toBe(true);
	});
});
