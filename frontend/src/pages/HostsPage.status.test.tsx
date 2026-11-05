import type { Host } from "@shared/types/api";
import { fireEvent, screen, waitFor } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import { renderApp } from "@/test/auth";
import { hostFixture } from "@/test/factories";
import { server } from "@/test/server";

const hostsFixture: Host[] = [
	hostFixture({ description: "" }),
	hostFixture({
		id: "m-2",
		name: "ci-runner",
		description: "",
		hostname: "ci.internal",
		port: 2222,
		sshUsername: "ci",
		sshKeyFingerprint: "SHA256:bbb",
		permission: "read",
		createdAt: "2026-08-02T10:00:00Z",
		updatedAt: "2026-08-02T10:00:00Z",
	}),
];

function hostsHandler(body: Host[]) {
	return http.get("*/api/hosts", () => HttpResponse.json(body));
}

function statusHandler(
	respond: (hostId: string) => Promise<Response> | Response,
	seen: string[] = [],
) {
	return http.get("*/api/hosts/:id/status", async ({ params }) => {
		seen.push(String(params.id));
		return respond(String(params.id));
	});
}

function renderHostsPage() {
	return renderApp("/hosts", {
		resolveInitialState: async () => ({ username: "alice", role: "user" }),
	});
}

describe("HostsPage status badges", () => {
	it("renders online and offline badges from the status endpoint", async () => {
		server.use(
			hostsHandler(hostsFixture),
			statusHandler((id) =>
				HttpResponse.json({
					status: id === "m-1" ? "online" : "offline",
					checkedAt: "2026-08-29T10:00:00Z",
				}),
			),
		);
		renderHostsPage();

		expect(await screen.findByText("prod-web-01")).toBeInTheDocument();
		expect(await screen.findByText("online")).toBeInTheDocument();
		expect(await screen.findByText("offline")).toBeInTheDocument();
	});

	it("shows checking while the request is in flight, then the result", async () => {
		let release: () => void = () => {};
		const gate = new Promise<void>((resolve) => {
			release = resolve;
		});
		server.use(
			hostsHandler(hostsFixture),
			statusHandler(async () => {
				await gate;
				return HttpResponse.json({
					status: "online",
					checkedAt: "2026-08-29T10:00:00Z",
				});
			}),
		);
		renderHostsPage();

		expect(await screen.findByText("prod-web-01")).toBeInTheDocument();
		expect((await screen.findAllByText("checking")).length).toBe(2);

		release();
		await waitFor(() => expect(screen.getAllByText("online")).toHaveLength(2));
	});

	it("shows unknown when the status request fails", async () => {
		server.use(
			hostsHandler(hostsFixture),
			statusHandler(() => new HttpResponse(null, { status: 500 })),
		);
		renderHostsPage();

		expect(await screen.findByText("prod-web-01")).toBeInTheDocument();
		expect(await screen.findByText("unknown")).toBeInTheDocument();
	});

	it("fires exactly one status request per host id", async () => {
		const seen: string[] = [];
		server.use(
			hostsHandler(hostsFixture),
			statusHandler(
				() =>
					HttpResponse.json({
						status: "online",
						checkedAt: "2026-08-29T10:00:00Z",
					}),
				seen,
			),
		);
		renderHostsPage();

		await waitFor(() => {
			expect(screen.getAllByTestId("host-status-badge")).toHaveLength(2);
			expect(screen.getAllByText("online")).toHaveLength(2);
		});
		expect(seen.sort()).toEqual(["m-1", "m-2"]);
	});

	it("exposes the checkedAt timestamp as a tooltip on the badge", async () => {
		server.use(
			hostsHandler(hostsFixture),
			statusHandler(() =>
				HttpResponse.json({
					status: "online",
					checkedAt: "2026-08-29T10:00:00Z",
				}),
			),
		);
		renderHostsPage();

		await screen.findAllByText("online");
		const badge = screen.getAllByTestId("host-status-badge")[0];
		fireEvent.focus(badge);
		const tooltip = await screen.findByRole("tooltip");
		expect(tooltip).toHaveTextContent("Checked at 2026-08-29T10:00:00Z");
		expect(badge).toHaveAttribute(
			"aria-label",
			"Checked at 2026-08-29T10:00:00Z",
		);
	});

	it("re-probes every host when the overview is re-opened", async () => {
		const seen: string[] = [];
		server.use(
			hostsHandler(hostsFixture),
			statusHandler(
				() =>
					HttpResponse.json({
						status: "online",
						checkedAt: "2026-08-29T10:00:00Z",
					}),
				seen,
			),
		);
		const { unmount } = renderHostsPage();
		await screen.findAllByText("online");
		unmount();

		renderHostsPage();
		await screen.findAllByText("online");
		expect(seen.sort()).toEqual(["m-1", "m-1", "m-2", "m-2"]);
	});
});
