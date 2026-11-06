import { screen, waitFor, within } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import { errorJson } from "@/test/errors";
import { server } from "@/test/server";
import {
	abortableNdjsonResponse,
	ndjsonResponse,
	streamFixtures,
} from "@/test/stream";
import {
	createHandler,
	createRequests,
	openCreatePage,
	pullRequests,
	pullStreamHandler,
	setupCreatePage,
	submitForm,
	typeImage,
} from "./createContainerTestHarness";

// The image-not-found pull flow on the create page (spec 341, ticket
// #349, reworked): a create that fails with IMAGE_NOT_FOUND turns the
// submit button into "Pull image & create container" — there is no separate
// pull button. Clicking it pulls the missing image through the images
// pull stream, and a successful pull re-submits the create with the
// preserved values. The seams are the rendered page and the MSW-backed
// endpoints (create + images pull stream), per the harness-and-builders
// convention.

const { progressEvent, completeEvent, errorEvent } = streamFixtures;

function imageNotFoundCreate() {
	return createHandler(
		404,
		errorJson("IMAGE_NOT_FOUND", "image not found", "req-1"),
	);
}

// A create handler that refuses the first request with IMAGE_NOT_FOUND
// (the image is missing) and accepts the second (after the pull).
function createFailsThenSucceedsHandler() {
	return http.post("*/api/hosts/m-1/containers", async ({ request }) => {
		createRequests.count++;
		createRequests.body = await request.json();
		if (createRequests.count === 1) {
			return HttpResponse.json(
				errorJson("IMAGE_NOT_FOUND", "image not found", "req-1"),
				{ status: 404 },
			);
		}
		return HttpResponse.json(
			{
				id: "c-new",
				name: "fresh",
				state: "created",
				image: "docker.io/library/nope:latest",
				createdAt: "2026-08-26T00:00:00Z",
			},
			{ status: 201 },
		);
	});
}

// The tests boot the full create-container page and drive userEvent
// chains through it; under full-suite parallel load the heaviest outgrows
// vitest's default 5s test timeout, so the file sets its own.
const TEST_TIMEOUT = 20_000;

describe("CreateContainerPage image-not-found pull flow", () => {
	it(
		"turns the submit into Pull image & create container on IMAGE_NOT_FOUND, values preserved",
		async () => {
			setupCreatePage(
				createHandler(
					404,
					errorJson("IMAGE_NOT_FOUND", "image not found", "req-1"),
				),
			);
			const user = await openCreatePage();

			typeImage("docker.io/library/nope:latest");
			await user.type(screen.getByLabelText(/name/i), "cache");
			await submitForm(user);

			// The create failure surfaces in the error region; the separate
			// pull button is gone and the submit button carries the pull.
			expect(await screen.findByRole("alert")).toBeInTheDocument();
			expect(screen.getByText("image not found")).toBeInTheDocument();
			expect(screen.queryByTestId("pull-image-button")).not.toBeInTheDocument();
			expect(screen.getByTestId("container-form-submit")).toHaveTextContent(
				"Pull image & create container",
			);

			// The form values are untouched.
			expect(
				screen.getByDisplayValue("docker.io/library/nope:latest"),
			).toBeInTheDocument();
			expect(screen.getByDisplayValue("cache")).toBeInTheDocument();
		},
		TEST_TIMEOUT,
	);

	it("streams pull progress inline when Pull image & create is clicked", async () => {
		setupCreatePage(imageNotFoundCreate());
		// A stream that emits progress and then stays open, so the
		// inline progress region is assertable while the pull runs.
		server.use(
			pullStreamHandler(
				abortableNdjsonResponse(
					[
						progressEvent({
							state: {
								current: 10,
								total: 100,
								status: "Downloading layers",
							},
						}),
					],
					new AbortController().signal,
				),
			),
		);
		const user = await openCreatePage();

		typeImage("docker.io/library/nope:latest");
		await submitForm(user);
		// The error region must be up (and the submit relabeled) before
		// the click targets the pull-mode button.
		await screen.findByRole("alert");
		await user.click(screen.getByTestId("container-form-submit"));

		// The progress streams inside the error region.
		const region = await screen.findByTestId("container-form-error");
		expect(
			await within(region).findByTestId("container-pull-progress"),
		).toBeInTheDocument();
		expect(within(region).getByText("Downloading layers")).toBeInTheDocument();

		// The pull request carried the missing image reference.
		await waitFor(() => expect(pullRequests.count).toBe(1));
		expect(pullRequests.body).toEqual({
			image: "docker.io/library/nope:latest",
		});
	});

	it("re-submits the create automatically after the pull", async () => {
		setupCreatePage(createFailsThenSucceedsHandler());
		server.use(
			pullStreamHandler(
				ndjsonResponse([
					progressEvent({
						state: {
							current: 100,
							total: 100,
							status: "Downloading layers",
						},
					}),
					completeEvent(),
				]),
			),
		);
		const user = await openCreatePage();

		typeImage("docker.io/library/nope:latest");
		await user.type(screen.getByLabelText(/name/i), "cache");
		await submitForm(user);
		// The error region must be up (and the submit relabeled) before
		// the click targets the pull-mode button.
		await screen.findByRole("alert");
		await user.click(screen.getByTestId("container-form-submit"));

		// The create re-submitted itself with the preserved values.
		await waitFor(() => expect(createRequests.count).toBe(2));
		expect(createRequests.body).toMatchObject({
			image: "docker.io/library/nope:latest",
			name: "cache",
		});

		// The successful retry navigated back to the containers tab.
		expect(await screen.findByTestId("containers-tab")).toHaveTextContent(
			"containers",
		);
		await waitFor(() =>
			expect(screen.queryByLabelText(/image/i)).not.toBeInTheDocument(),
		);
	});

	it("shows the pull error inline and preserves the values on pull failure", async () => {
		setupCreatePage(imageNotFoundCreate());
		server.use(
			pullStreamHandler(
				ndjsonResponse([
					errorEvent("REGISTRY_UNREACHABLE", "Registry is down"),
				]),
			),
		);
		const user = await openCreatePage();

		typeImage("docker.io/library/nope:latest");
		await user.type(screen.getByLabelText(/name/i), "cache");
		await submitForm(user);
		// The error region must be up (and the submit relabeled) before
		// the click targets the pull-mode button.
		await screen.findByRole("alert");
		await user.click(screen.getByTestId("container-form-submit"));

		// The pull failure surfaces next to the create error, inside
		// the form's error region.
		const region = await screen.findByTestId("container-form-error");
		expect(
			await within(region).findByText("REGISTRY_UNREACHABLE"),
		).toBeInTheDocument();
		expect(within(region).getByText("Registry is down")).toBeInTheDocument();

		// The create was not re-submitted, the values are preserved,
		// and the same button can retry the pull.
		expect(createRequests.count).toBe(1);
		expect(
			screen.getByDisplayValue("docker.io/library/nope:latest"),
		).toBeInTheDocument();
		expect(screen.getByDisplayValue("cache")).toBeInTheDocument();
		expect(screen.getByTestId("container-form-submit")).toHaveTextContent(
			"Pull image & create container",
		);
	});

	it("reverts to a plain create when the image reference is edited after IMAGE_NOT_FOUND", async () => {
		setupCreatePage(imageNotFoundCreate());
		const user = await openCreatePage();

		typeImage("docker.io/library/nope:latest");
		await submitForm(user);
		await screen.findByRole("alert");
		expect(screen.getByTestId("container-form-submit")).toHaveTextContent(
			"Pull image & create container",
		);

		// Editing the image invalidates the not-found verdict: the stale
		// error clears and the submit reverts to a plain create.
		await user.type(screen.getByLabelText(/image/i), "-fixed");
		expect(screen.queryByRole("alert")).not.toBeInTheDocument();
		expect(screen.getByTestId("container-form-submit")).toHaveTextContent(
			"Create container",
		);

		// The next submit goes straight to the create, without a pull.
		await user.click(screen.getByTestId("container-form-submit"));
		await waitFor(() => expect(createRequests.count).toBe(2));
		expect(pullRequests.count).toBe(0);
	});
});
