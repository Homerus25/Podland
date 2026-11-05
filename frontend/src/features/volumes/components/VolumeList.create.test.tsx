import { VolumeList } from "@features/volumes/components/VolumeList";
import type { VolumeSummary } from "@features/volumes/services/volumesApi";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http, type JsonBodyType } from "msw";
import { beforeEach, describe, expect, it } from "vitest";
import { errorJson } from "@/test/errors";
import { hostFixture, volumeFixture } from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

const existing = volumeFixture({ name: "existing" });

let listRequests: { count: number; volumes: () => VolumeSummary[] } = {
	count: 0,
	volumes: () => [existing],
};
let createRequests: { count: number; body: JsonBodyType } = {
	count: 0,
	body: null,
};

function resetCounters() {
	listRequests = { count: 0, volumes: () => [existing] };
	createRequests = { count: 0, body: null };
}

function installHandlers(
	createResponse: () => { status: number; body: JsonBodyType } = () => ({
		status: 201,
		body: volumeFixture({ name: "fresh" }),
	}),
) {
	server.use(
		http.get("*/api/hosts/m-1/volumes", () => {
			listRequests.count += 1;
			return HttpResponse.json(listRequests.volumes());
		}),
		http.post("*/api/hosts/m-1/volumes", async ({ request }) => {
			createRequests.count += 1;
			createRequests.body = await request.json();
			const response = createResponse();
			if (response.status !== 201) {
				return HttpResponse.json(response.body, { status: response.status });
			}
			listRequests.volumes = () => [existing, volumeFixture({ name: "fresh" })];
			return HttpResponse.json(response.body, { status: 201 });
		}),
	);
}

function renderVolumeList(permission: "read" | "write" | "admin" = "admin") {
	renderWithQuery(<VolumeList host={hostFixture({ permission })} />, {
		chakra: true,
		router: true,
	});
}

async function openCreateModal() {
	renderVolumeList();
	await screen.findByText("existing");
	const user = userEvent.setup();
	await user.click(screen.getByTestId("create-volume-button"));
	expect(await screen.findByTestId("modal")).toBeInTheDocument();
	return user;
}

// The full-form flow drives a long userEvent chain; under full-suite
// parallel load it outgrows vitest's default 5s test timeout, so the
// file sets its own.
const TEST_TIMEOUT = 20_000;

describe("VolumeList create", () => {
	beforeEach(resetCounters);

	it("hides the create button behind a hint for read-level users", async () => {
		installHandlers();
		renderVolumeList("read");
		await screen.findByText("existing");

		expect(
			screen.queryByTestId("create-volume-button"),
		).not.toBeInTheDocument();
		expect(screen.getByTestId("volumes-permission-hint")).toHaveTextContent(
			/read-only access/i,
		);
	});

	it("shows the create button for write-level users", async () => {
		installHandlers();
		renderVolumeList("write");
		expect(
			await screen.findByTestId("create-volume-button"),
		).toBeInTheDocument();
	});

	it(
		"submits name, labels, driver and options",
		async () => {
			installHandlers();
			const user = await openCreateModal();

			await user.type(screen.getByLabelText(/^name/i), "  app-data  ");
			await user.click(screen.getByTestId("add-label"));
			await user.type(screen.getByLabelText("label key 1"), "team");
			await user.type(screen.getByLabelText("label value 1"), "platform");

			await user.click(screen.getByTestId("volume-advanced-toggle"));
			await user.type(screen.getByLabelText("Driver"), "local");
			await user.click(screen.getByTestId("add-option"));
			await user.type(screen.getByLabelText("option key 1"), "type");
			await user.type(screen.getByLabelText("option value 1"), "tmpfs");

			await user.click(screen.getByTestId("volume-form-submit"));

			await waitFor(() => expect(createRequests.count).toBe(1));
			expect(createRequests.body).toEqual({
				name: "app-data",
				labels: { team: "platform" },
				driver: "local",
				options: { type: "tmpfs" },
			});
		},
		TEST_TIMEOUT,
	);

	it("renders the labels and options rows through the shared key/value editor", async () => {
		installHandlers();
		const user = await openCreateModal();

		await user.click(screen.getByTestId("add-label"));
		expect(screen.getByLabelText("label key 1")).toBeInTheDocument();
		expect(screen.getByLabelText("label value 1")).toBeInTheDocument();
		expect(screen.getByTestId("remove-label-0")).toHaveAttribute(
			"aria-label",
			"Remove label entry 1",
		);

		await user.click(screen.getByTestId("volume-advanced-toggle"));
		await user.click(screen.getByTestId("add-option"));
		expect(screen.getByLabelText("option key 1")).toBeInTheDocument();
		expect(screen.getByTestId("remove-option-0")).toHaveAttribute(
			"aria-label",
			"Remove option entry 1",
		);

		// The adapter turns the per-row key validation on: a value without
		// a key gates submit.
		await user.type(screen.getByLabelText("label value 1"), "platform");
		await user.click(screen.getByTestId("volume-form-submit"));

		expect(
			await screen.findByText("Key is required when a value is set"),
		).toBeInTheDocument();
		expect(createRequests.count).toBe(0);
	});

	it("creates an anonymous volume with an empty name and toasts the generated name", async () => {
		installHandlers(() => ({
			status: 201,
			body: volumeFixture({ name: "a1b2c3d4e5f6", anonymous: true }),
		}));
		const user = await openCreateModal();

		await user.click(screen.getByTestId("volume-form-submit"));

		await waitFor(() => expect(createRequests.count).toBe(1));
		expect(createRequests.body).toEqual({ name: "" });
		expect(
			await screen.findByText(/anonymous volume created: a1b2c3d4e5f6/i),
		).toBeInTheDocument();
		expect(screen.queryByTestId("modal")).not.toBeInTheDocument();
	});

	it("closes the modal and toasts the name for a named volume", async () => {
		installHandlers();
		const user = await openCreateModal();

		await user.type(screen.getByLabelText(/^name/i), "app-data");
		await user.click(screen.getByTestId("volume-form-submit"));

		await waitFor(() => expect(createRequests.count).toBe(1));
		expect(
			await screen.findByText(/volume created: fresh/i),
		).toBeInTheDocument();
		expect(screen.queryByTestId("modal")).not.toBeInTheDocument();
	});

	it("invalidates the volumes list so the new volume appears", async () => {
		installHandlers();
		const user = await openCreateModal();

		expect(listRequests.count).toBe(1);
		await user.type(screen.getByLabelText(/^name/i), "app-data");
		await user.click(screen.getByTestId("volume-form-submit"));

		await screen.findByText("fresh");
		expect(listRequests.count).toBeGreaterThan(1);
	});

	it("surfaces backend validation errors inline in the modal", async () => {
		installHandlers(() => ({
			status: 400,
			body: errorJson(
				"INVALID_REQUEST",
				"invalid volume name: bad name",
				"r-1",
			),
		}));
		const user = await openCreateModal();

		await user.type(screen.getByLabelText(/^name/i), "bad name");
		await user.click(screen.getByTestId("volume-form-submit"));

		await waitFor(() => expect(createRequests.count).toBe(1));
		expect(await screen.findByText(/invalid volume name/i)).toBeInTheDocument();
		expect(screen.getByTestId("modal")).toBeInTheDocument();
	});
});
