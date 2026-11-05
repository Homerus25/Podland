import type { Host } from "@shared/types/api";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { beforeEach, describe, expect, it } from "vitest";
import { renderApp } from "@/test/auth";
import { errorResponse } from "@/test/errors";
import { server } from "@/test/server";

const existingHost: Host = {
	id: "m-1",
	name: "prod-web-01",
	description: "Production web server",
	hostname: "web01.example.com",
	port: 22,
	sshUsername: "deploy",
	sshKeyFingerprint: "SHA256:aaa",
	podmanSocketPath: "/run/podman/podman.sock",
	ownerId: "u-owner",
	permission: "admin",
	isOwner: true,
	createdAt: "2026-08-01T10:00:00Z",
	updatedAt: "2026-08-01T10:00:00Z",
};

const createdHost: Host = {
	...existingHost,
	id: "m-2",
	name: "ci-runner",
};

function mockHosts(body: Host[]) {
	return http.get("*/api/hosts", () => HttpResponse.json(body));
}

let createRequests: Array<Record<string, unknown>> = [];

function mockCreate(
	status: number,
	body: Record<string, unknown>,
	delayMs = 0,
) {
	return http.post("*/api/hosts", async ({ request }) => {
		const payload = (await request.json()) as Record<string, unknown>;
		createRequests.push(payload);
		if (delayMs > 0) {
			await new Promise((resolve) => setTimeout(resolve, delayMs));
		}
		return HttpResponse.json(body, { status });
	});
}

function keyFile() {
	return new File(
		[
			"-----BEGIN OPENSSH PRIVATE KEY-----\nabc\n-----END OPENSSH PRIVATE KEY-----",
		],
		"id_ed25519",
		{
			type: "text/plain",
		},
	);
}

async function renderAndOpenModal(hosts: Host[] = [existingHost]) {
	server.use(mockHosts(hosts));
	renderApp("/hosts", {
		resolveInitialState: async () => ({ username: "alice", role: "user" }),
	});
	await screen.findByTestId("add-host-button");
	await userEvent.click(screen.getByTestId("add-host-button"));
	expect(await screen.findByTestId("modal")).toBeInTheDocument();
}

async function fillValidForm() {
	await userEvent.type(screen.getByLabelText(/^Name/), "ci-runner");
	await userEvent.type(screen.getByLabelText(/^Hostname or IP/), "ci.internal");
	await userEvent.clear(screen.getByLabelText(/^SSH port/));
	await userEvent.type(screen.getByLabelText(/^SSH port/), "2222");
	await userEvent.type(screen.getByLabelText(/^SSH username/), "ci");
	await userEvent.upload(screen.getByLabelText(/^SSH private key/), keyFile());
}

describe("Add Host modal", () => {
	beforeEach(() => {
		createRequests = [];
	});

	it("opens a modal with the form from the Add Host button", async () => {
		await renderAndOpenModal();
		expect(screen.getByLabelText(/^Name/)).toBeInTheDocument();
		expect(screen.getByLabelText(/^Hostname or IP/)).toBeInTheDocument();
		expect(screen.getByLabelText(/^SSH port/)).toBeInTheDocument();
		expect(screen.getByLabelText(/^SSH username/)).toBeInTheDocument();
		expect(screen.getByLabelText(/^SSH private key/)).toBeInTheDocument();
		expect(screen.getByLabelText(/^Description/)).toBeInTheDocument();
		expect(screen.getByLabelText(/^Podman socket path/)).toBeInTheDocument();
	});

	it("starts with the submit disabled on an empty form and sends no request", async () => {
		await renderAndOpenModal();

		const submit = screen.getByTestId("host-form-submit");
		expect(submit).toBeDisabled();
		await userEvent.click(submit);

		expect(createRequests).toHaveLength(0);
	});

	it("shows the port error while typing and keeps the submit disabled", async () => {
		await renderAndOpenModal();
		await userEvent.clear(screen.getByLabelText(/ssh port/i));

		expect(await screen.findByText("Port is required")).toBeInTheDocument();
		expect(screen.getByTestId("host-form-submit")).toBeDisabled();
		expect(createRequests).toHaveLength(0);
	});

	it("shows an inline error when the port is outside the numeric range", async () => {
		await renderAndOpenModal();
		await fillValidForm();
		await userEvent.clear(screen.getByLabelText(/^SSH port/));
		await userEvent.type(screen.getByLabelText(/^SSH port/), "99999");
		await userEvent.click(screen.getByTestId("host-form-submit"));

		expect(
			await screen.findByText("Port must be a number between 1 and 65535"),
		).toBeInTheDocument();
		expect(createRequests).toHaveLength(0);
	});

	it("creates a host, closes the modal, and refetches the list", async () => {
		let hosts: Host[] = [existingHost];
		await renderAndOpenModal();
		// Registered after the helper's static handler so msw prefers this one.
		server.use(
			http.get("*/api/hosts", () => HttpResponse.json(hosts)),
			http.post("*/api/hosts", async ({ request }) => {
				const body = (await request.json()) as Record<string, unknown>;
				createRequests.push(body);
				hosts = [existingHost, createdHost];
				return HttpResponse.json(createdHost, { status: 201 });
			}),
		);
		await fillValidForm();
		await userEvent.click(screen.getByTestId("host-form-submit"));

		await waitFor(() =>
			expect(screen.queryByTestId("modal")).not.toBeInTheDocument(),
		);
		expect(await screen.findByText("ci-runner")).toBeInTheDocument();
		expect(createRequests).toHaveLength(1);
		expect(createRequests[0]).toMatchObject({
			name: "ci-runner",
			hostname: "ci.internal",
			port: 2222,
			sshUsername: "ci",
			sshPrivateKey: expect.stringContaining("BEGIN OPENSSH PRIVATE KEY"),
		});
	});

	it("submits optional fields empty", async () => {
		server.use(
			mockHosts([]),
			mockCreate(201, createdHost as unknown as Record<string, unknown>),
		);
		await renderAndOpenModal([]);
		await fillValidForm();
		await userEvent.click(screen.getByTestId("host-form-submit"));

		await waitFor(() =>
			expect(screen.queryByTestId("modal")).not.toBeInTheDocument(),
		);
		expect(createRequests[0]).toMatchObject({
			description: "",
			podmanSocketPath: "",
		});
	});

	it("renders the error envelope in the modal and keeps it open when creation fails", async () => {
		server.use(
			mockHosts([existingHost]),
			http.post("*/api/hosts", () =>
				errorResponse(422, "HOST_UNREACHABLE", "SSH handshake failed"),
			),
		);
		await renderAndOpenModal();
		await fillValidForm();
		await userEvent.click(screen.getByTestId("host-form-submit"));

		expect(await screen.findByRole("alert")).toBeInTheDocument();
		expect(screen.getByText("HOST_UNREACHABLE")).toBeInTheDocument();
		expect(screen.getByText("SSH handshake failed")).toBeInTheDocument();
		expect(screen.getByTestId("modal")).toBeInTheDocument();
	});
});
