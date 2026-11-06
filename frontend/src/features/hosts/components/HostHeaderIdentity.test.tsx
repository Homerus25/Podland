import { screen, waitFor } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import { errorResponse } from "@/test/errors";
import { hostFixture, hostVersionFixture } from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";
import { HostHeaderIdentity } from "./HostHeaderIdentity";

const host = hostFixture();

function renderIdentity(hostId = host.id) {
	return renderWithQuery(<HostHeaderIdentity hostId={hostId} />, {
		chakra: true,
	});
}

function hostHandlers() {
	server.use(
		http.get(`*/api/hosts/${host.id}`, () => HttpResponse.json(host)),
		http.get(`*/api/hosts/${host.id}/version`, () =>
			HttpResponse.json(hostVersionFixture()),
		),
	);
}

describe("HostHeaderIdentity", () => {
	it("renders the host name with badge, endpoint, and ssh user on one row", async () => {
		hostHandlers();
		renderIdentity();

		expect(await screen.findByText(host.name)).toBeInTheDocument();
		expect(screen.getByTestId("host-endpoint")).toHaveTextContent(
			"web01.example.com:22",
		);
		expect(screen.getByTestId("host-ssh-user")).toHaveTextContent("deploy");
		expect(screen.getByTestId("host-permission")).toHaveTextContent("admin");
	});

	it("renders nothing while the host query is pending", async () => {
		let release: () => void = () => {};
		const gate = new Promise<void>((resolve) => {
			release = resolve;
		});
		server.use(
			http.get(`*/api/hosts/${host.id}`, async () => {
				await gate;
				return HttpResponse.json(host);
			}),
			http.get(`*/api/hosts/${host.id}/version`, () =>
				HttpResponse.json(hostVersionFixture()),
			),
		);
		renderIdentity();

		// The slot stays empty — no identity, no fallback title.
		expect(screen.queryByText(host.name)).not.toBeInTheDocument();
		expect(screen.queryByTestId("host-endpoint")).not.toBeInTheDocument();

		release();
		await screen.findByText(host.name);
	});

	it("falls back to the Hosts title when the host fetch fails", async () => {
		server.use(
			http.get(`*/api/hosts/${host.id}`, () =>
				errorResponse(404, "HOST_NOT_FOUND", "Host not found", "req-9"),
			),
			http.get(`*/api/hosts/${host.id}/version`, () =>
				HttpResponse.json(hostVersionFixture()),
			),
		);
		renderIdentity();

		expect(
			await screen.findByRole("heading", { name: "Hosts" }),
		).toBeInTheDocument();
		await waitFor(() =>
			expect(screen.queryByTestId("host-endpoint")).not.toBeInTheDocument(),
		);
	});
});
