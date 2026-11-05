import type { Host } from "@shared/types/api";
import { screen, waitFor } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import { renderApp } from "@/test/auth";
import { hostFixture, hostVersionFixture } from "@/test/factories";
import { server } from "@/test/server";

const host: Host = hostFixture();

// The page's default tab (Containers) mounts with the header, so its
// endpoint needs answering alongside the host and version GETs.
function renderHostDetailPage() {
	server.use(
		http.get(`*/api/hosts/${host.id}`, () => HttpResponse.json(host)),
		http.get(`*/api/hosts/${host.id}/containers`, () =>
			HttpResponse.json({
				containers: [],
				total: 0,
				timestamp: "2026-08-26T00:00:00Z",
			}),
		),
	);
	return renderApp(`/hosts/${host.id}`, {
		resolveInitialState: async () => ({ username: "alice", role: "user" }),
	});
}

function versionHandler(respond: () => Promise<Response> | Response) {
	return http.get(`*/api/hosts/${host.id}/version`, async () => respond());
}

describe("HostDetailPage podman version", () => {
	it("renders the versions next to endpoint and ssh user", async () => {
		server.use(versionHandler(() => HttpResponse.json(hostVersionFixture())));
		renderHostDetailPage();

		const version = await screen.findByTestId("host-podman-version");
		expect(version).toHaveTextContent("podman 5.4.1 · api 5.4.1");

		const header = screen.getByTestId("host-endpoint").parentElement!;
		expect(header).toContainElement(screen.getByTestId("host-endpoint"));
		expect(header).toContainElement(screen.getByTestId("host-ssh-user"));
		expect(header).toContainElement(version);
	});

	it("renders podman version unavailable when the endpoint answers nulls", async () => {
		server.use(
			versionHandler(() =>
				HttpResponse.json(
					hostVersionFixture({
						podmanVersion: null,
						apiVersion: null,
					}),
				),
			),
		);
		renderHostDetailPage();

		expect(
			await screen.findByText("podman version unavailable"),
		).toBeInTheDocument();
	});

	it("shows no version text while the fetch is pending", async () => {
		let release: () => void = () => {};
		const gate = new Promise<void>((resolve) => {
			release = resolve;
		});
		server.use(
			versionHandler(async () => {
				await gate;
				return HttpResponse.json(hostVersionFixture());
			}),
		);
		renderHostDetailPage();

		await screen.findByText(host.name);
		expect(screen.queryByTestId("host-podman-version")).toBeNull();

		release();
		await waitFor(() =>
			expect(screen.getByTestId("host-podman-version")).toHaveTextContent(
				"podman 5.4.1 · api 5.4.1",
			),
		);
	});
});
