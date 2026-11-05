import type { Host } from "@shared/types/api";
import { screen } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import { renderApp } from "@/test/auth";
import { errorResponse } from "@/test/errors";
import { hostFixture } from "@/test/factories";
import { server } from "@/test/server";

const hostsFixture: Host[] = [
	hostFixture(),
	hostFixture({
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

function hostsHandler(body: Host[], status = 200) {
	return http.get("*/api/hosts", () => HttpResponse.json(body, { status }));
}

function renderHostsPage() {
	return renderApp("/hosts", {
		resolveInitialState: async () => ({ username: "alice", role: "user" }),
	});
}

describe("HostsPage", () => {
	it("renders one card per host with name, description, hostname:port, and permission badge", async () => {
		server.use(hostsHandler(hostsFixture));
		renderHostsPage();

		expect(await screen.findByText("prod-web-01")).toBeInTheDocument();
		expect(screen.getByText("Production web server")).toBeInTheDocument();
		expect(screen.getByText("web01.example.com:22")).toBeInTheDocument();
		expect(screen.getByText("admin")).toBeInTheDocument();

		expect(screen.getByText("ci-runner")).toBeInTheDocument();
		expect(screen.getByText("Shared CI runner")).toBeInTheDocument();
		expect(screen.getByText("ci.internal:2222")).toBeInTheDocument();
		expect(screen.getByText("read")).toBeInTheDocument();
	});

	it("shows a loading state before hosts arrive", async () => {
		server.use(
			http.get("*/api/hosts", async () => {
				await new Promise((resolve) => setTimeout(resolve, 50));
				return HttpResponse.json(hostsFixture);
			}),
		);
		renderHostsPage();

		expect(await screen.findByTestId("hosts-loading")).toBeInTheDocument();
	});

	it("shows an explanatory empty state when the user has no hosts", async () => {
		server.use(hostsHandler([]));
		renderHostsPage();

		const empty = await screen.findByTestId("hosts-empty");
		expect(empty).toBeInTheDocument();
		expect(empty.textContent).toMatch(/no hosts/i);
		expect(screen.queryByTestId("host-card")).not.toBeInTheDocument();
	});

	it("shows generic text for internal error codes, hiding leaked details", async () => {
		server.use(
			http.get("*/api/hosts", () =>
				errorResponse(500, "INTERNAL_ERROR", "database is on fire", "req-3"),
			),
		);
		renderHostsPage();

		expect(
			await screen.findByText("An unexpected error occurred"),
		).toBeInTheDocument();
		expect(screen.getByText("INTERNAL_ERROR")).toBeInTheDocument();
		expect(screen.queryByText("database is on fire")).not.toBeInTheDocument();
	});

	it.each(["read", "write", "admin"] as const)(
		"offers the same View navigation on the hosts list for %s users",
		async (permission) => {
			server.use(
				http.get("*/api/hosts", () =>
					HttpResponse.json([hostFixture({ permission })]),
				),
			);
			renderApp("/hosts", {
				resolveInitialState: async () => ({ username: "alice", role: "user" }),
			});

			const card = await screen.findByTestId("host-card");
			expect(card).toBeInTheDocument();

			// The list has no host-scoped write actions, so there is nothing
			// for a read user to be baited with — assert no stray actions appear.
			const cardButtons = card.querySelectorAll("button");
			expect(cardButtons).toHaveLength(0);
		},
	);
});
