import type { Machine } from "@shared/types/api";
import { screen } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import { renderApp } from "@/test/auth";
import { machineFixture } from "@/test/factories";
import { server } from "@/test/server";

const machinesFixture: Machine[] = [
	machineFixture(),
	machineFixture({
		id: "m-2",
		name: "ci-runner",
		description: "Shared CI runner",
		hostname: "ci.internal",
		port: 2222,
		sshUsername: "ci",
		sshKeyFingerprint: "SHA256:bbb",
		permission: "read",
		createdAt: "2026-08-02T10:00:00Z",
		updatedAt: "2026-08-02T10:00:00Z",
	}),
];

function machinesHandler(body: Machine[], status = 200) {
	return http.get("*/api/machines", () => HttpResponse.json(body, { status }));
}

function renderMachinesPage() {
	return renderApp("/machines", {
		resolveInitialState: async () => ({ username: "alice" }),
	});
}

describe("MachinesPage", () => {
	it("renders one card per machine with name, description, hostname:port, and permission badge", async () => {
		server.use(machinesHandler(machinesFixture));
		renderMachinesPage();

		expect(await screen.findByText("prod-web-01")).toBeInTheDocument();
		expect(screen.getByText("Production web server")).toBeInTheDocument();
		expect(screen.getByText("web01.example.com:22")).toBeInTheDocument();
		expect(screen.getByText("admin")).toBeInTheDocument();

		expect(screen.getByText("ci-runner")).toBeInTheDocument();
		expect(screen.getByText("Shared CI runner")).toBeInTheDocument();
		expect(screen.getByText("ci.internal:2222")).toBeInTheDocument();
		expect(screen.getByText("read")).toBeInTheDocument();
	});

	it("shows a loading state before machines arrive", async () => {
		server.use(
			http.get("*/api/machines", async () => {
				await new Promise((resolve) => setTimeout(resolve, 50));
				return HttpResponse.json(machinesFixture);
			}),
		);
		renderMachinesPage();

		expect(await screen.findByTestId("machines-loading")).toBeInTheDocument();
	});

	it("shows an explanatory empty state when the user has no machines", async () => {
		server.use(machinesHandler([]));
		renderMachinesPage();

		const empty = await screen.findByTestId("machines-empty");
		expect(empty).toBeInTheDocument();
		expect(empty.textContent).toMatch(/no machines/i);
		expect(screen.queryByTestId("machine-card")).not.toBeInTheDocument();
	});

	it("shows generic text for internal error codes, hiding leaked details", async () => {
		server.use(
			http.get("*/api/machines", () =>
				HttpResponse.json(
					{
						requestId: "req-3",
						error: { message: "database is on fire", code: "INTERNAL_ERROR" },
					},
					{ status: 500 },
				),
			),
		);
		renderMachinesPage();

		expect(
			await screen.findByText("An unexpected error occurred"),
		).toBeInTheDocument();
		expect(screen.getByText("INTERNAL_ERROR")).toBeInTheDocument();
		expect(screen.queryByText("database is on fire")).not.toBeInTheDocument();
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
