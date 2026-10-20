import type { ContainerSummary } from "@features/containers/services/containersApi";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import { errorJson } from "@/test/errors";
import { containerFixture } from "@/test/factories";
import { server } from "@/test/server";
import {
	containersHandler,
	createHandler,
	createRequests,
	existing,
	openCreateModal,
	optionListHandlers,
	renderContainerList,
	resetCounters,
	setupCreateDialog,
} from "./createContainerTestHarness";

// Dialog-behavior pins for the create-container dialog: form shape and
// validation, submit gating, list wiring, success close, and the error
// envelope. The form-to-payload mapping contract is pinned without a DOM
// in services/containersApi.create.test.ts (issue 328, candidate C3);
// the full-payload smoke lives in
// ContainerList.create.integration.test.tsx.

describe("ContainerList create", () => {
	it("opens a modal form with required image and optional name and command", async () => {
		setupCreateDialog();
		await openCreateModal();

		expect(screen.getByLabelText(/image/i)).toBeRequired();
		expect(screen.getByLabelText(/name/i)).not.toBeRequired();
		expect(screen.getByLabelText(/command/i)).not.toBeRequired();
		// The empty form is invalid, so the module footer's submit starts
		// disabled and enables only once the image is filled.
		expect(screen.getByTestId("container-form-submit")).toBeDisabled();
	});

	it("starts with the submit disabled on an empty form and sends no request", async () => {
		setupCreateDialog();
		const user = await openCreateModal();

		await user.click(screen.getByTestId("container-form-submit"));

		expect(createRequests.count).toBe(0);
		expect(screen.getByTestId("modal")).toBeInTheDocument();
	});

	it("shows the image error while typing a whitespace-only value and sends no request", async () => {
		setupCreateDialog();
		const user = await openCreateModal();

		await user.type(screen.getByLabelText(/image/i), "   ");
		expect(await screen.findByText("Image is required")).toBeInTheDocument();
		expect(screen.getByTestId("container-form-submit")).toBeDisabled();
		await user.click(screen.getByTestId("container-form-submit"));

		expect(createRequests.count).toBe(0);
	});

	it("offers the create action on an empty machine and creates the first container", async () => {
		resetCounters();
		let containers: ContainerSummary[] = [];
		server.use(
			containersHandler(() => containers),
			...optionListHandlers(),
			http.post("*/api/machines/m-1/containers", async ({ request }) => {
				createRequests.count++;
				await request.json();
				containers = [
					containerFixture({
						id: "c-new",
						names: ["/fresh"],
						image: "docker.io/library/redis:7",
						state: "created",
					}),
				];
				return HttpResponse.json(
					{
						id: "c-new",
						name: "fresh",
						state: "created",
						image: "docker.io/library/redis:7",
						createdAt: "2026-08-26T00:00:00Z",
					},
					{ status: 201 },
				);
			}),
		);
		renderContainerList();

		expect(await screen.findByTestId("containers-empty")).toBeInTheDocument();
		const user = userEvent.setup();
		await user.click(screen.getByTestId("create-container-button"));
		await user.type(
			await screen.findByLabelText(/image/i),
			"docker.io/library/redis:7",
		);
		await user.click(screen.getByTestId("container-form-submit"));

		await waitFor(() =>
			expect(screen.queryByTestId("modal")).not.toBeInTheDocument(),
		);
		expect(await screen.findByText("fresh")).toBeInTheDocument();
		expect(createRequests.count).toBe(1);
	});

	it("closes the modal and shows the new container after success", async () => {
		resetCounters();
		let containers = [existing];
		server.use(
			containersHandler(() => containers),
			...optionListHandlers(),
			http.post("*/api/machines/m-1/containers", async ({ request }) => {
				createRequests.count++;
				await request.json();
				containers = [
					existing,
					containerFixture({
						id: "c-new",
						names: ["/fresh"],
						image: "docker.io/library/redis:7",
						state: "created",
					}),
				];
				return HttpResponse.json(
					{
						id: "c-new",
						name: "fresh",
						state: "created",
						image: "docker.io/library/redis:7",
						createdAt: "2026-08-26T00:00:00Z",
					},
					{ status: 201 },
				);
			}),
		);
		const user = await openCreateModal();

		await user.type(
			screen.getByLabelText(/image/i),
			"docker.io/library/redis:7",
		);
		await user.click(screen.getByTestId("container-form-submit"));

		await waitFor(() =>
			expect(screen.queryByTestId("modal")).not.toBeInTheDocument(),
		);
		expect(
			await screen.findByText("docker.io/library/redis:7"),
		).toBeInTheDocument();
	});

	it("keeps the modal open and shows the error envelope on failure", async () => {
		setupCreateDialog(
			createHandler(
				404,
				errorJson("IMAGE_NOT_FOUND", "image not found", "req-14"),
			),
		);
		const user = await openCreateModal();

		await user.type(
			screen.getByLabelText(/image/i),
			"docker.io/library/nope:latest",
		);
		await user.click(screen.getByTestId("container-form-submit"));

		expect(await screen.findByRole("alert")).toBeInTheDocument();
		expect(screen.getByText("IMAGE_NOT_FOUND")).toBeInTheDocument();
		expect(screen.getByText("image not found")).toBeInTheDocument();
		expect(screen.getByTestId("modal")).toBeInTheDocument();
		expect(
			screen.getByDisplayValue("docker.io/library/nope:latest"),
		).toBeInTheDocument();
	});

	it("renders the environment rows through the shared key/value editor", async () => {
		setupCreateDialog();
		const user = await openCreateModal();

		const addButton = screen.getByTestId("add-env-variable");
		expect(addButton).toHaveTextContent("Add variable");
		await user.click(addButton);

		expect(screen.getByLabelText("Environment variable 1 key")).toHaveAttribute(
			"placeholder",
			"KEY",
		);
		expect(
			screen.getByLabelText("Environment variable 1 value"),
		).toBeInTheDocument();
		expect(screen.getByTestId("remove-env-variable-0")).toHaveAttribute(
			"aria-label",
			"Remove environment variable 1",
		);
	});
});
