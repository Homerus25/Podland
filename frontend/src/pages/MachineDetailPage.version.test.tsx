import type { Machine } from "@shared/types/api";
import { screen, waitFor } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import { renderApp } from "@/test/auth";
import { machineFixture, machineVersionFixture } from "@/test/factories";
import { server } from "@/test/server";

const machine: Machine = machineFixture();

// The page's default tab (Containers) mounts with the header, so its
// endpoint needs answering alongside the machine and version GETs.
function renderMachineDetailPage() {
	server.use(
		http.get(`*/api/machines/${machine.id}`, () => HttpResponse.json(machine)),
		http.get(`*/api/machines/${machine.id}/containers`, () =>
			HttpResponse.json({
				containers: [],
				total: 0,
				timestamp: "2026-08-26T00:00:00Z",
			}),
		),
	);
	return renderApp(`/machines/${machine.id}`, {
		resolveInitialState: async () => ({ username: "alice", role: "user" }),
	});
}

function versionHandler(respond: () => Promise<Response> | Response) {
	return http.get(`*/api/machines/${machine.id}/version`, async () =>
		respond(),
	);
}

describe("MachineDetailPage podman version", () => {
	it("renders the versions next to endpoint and ssh user", async () => {
		server.use(
			versionHandler(() => HttpResponse.json(machineVersionFixture())),
		);
		renderMachineDetailPage();

		const version = await screen.findByTestId("machine-podman-version");
		expect(version).toHaveTextContent("podman 5.4.1 · api 5.4.1");

		const header = screen.getByTestId("machine-endpoint").parentElement!;
		expect(header).toContainElement(screen.getByTestId("machine-endpoint"));
		expect(header).toContainElement(screen.getByTestId("machine-ssh-user"));
		expect(header).toContainElement(version);
	});

	it("renders podman version unavailable when the endpoint answers nulls", async () => {
		server.use(
			versionHandler(() =>
				HttpResponse.json(
					machineVersionFixture({
						podmanVersion: null,
						apiVersion: null,
					}),
				),
			),
		);
		renderMachineDetailPage();

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
				return HttpResponse.json(machineVersionFixture());
			}),
		);
		renderMachineDetailPage();

		await screen.findByText(machine.name);
		expect(screen.queryByTestId("machine-podman-version")).toBeNull();

		release();
		await waitFor(() =>
			expect(screen.getByTestId("machine-podman-version")).toHaveTextContent(
				"podman 5.4.1 · api 5.4.1",
			),
		);
	});
});
