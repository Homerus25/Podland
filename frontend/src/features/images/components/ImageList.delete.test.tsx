import { ImageList } from "@features/images/components/ImageList";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import { errorJson, errorResponse } from "@/test/errors";
import { imageFixture, machineFixture } from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

const imagesFixture = [
	imageFixture({
		id: "sha256:abc123def4567890abc123def4567890abc123def4567890abc123def4567890",
	}),
];

let deleteRequests: { url: URL; count: number } = {
	url: new URL("http://localhost"),
	count: 0,
};

function resetCounters() {
	deleteRequests = { url: new URL("http://localhost"), count: 0 };
}

function imagesHandler(images = imagesFixture) {
	return http.get("*/api/machines/m-1/images", () => HttpResponse.json(images));
}

function deleteHandler(status = 204, body?: Record<string, unknown>) {
	return http.delete("*/api/machines/m-1/images/*", ({ request }) => {
		deleteRequests.count++;
		deleteRequests.url = new URL(request.url);
		if (status !== 204) return HttpResponse.json(body, { status });
		return new HttpResponse(null, { status: 204 });
	});
}

async function renderImageList() {
	renderWithQuery(<ImageList machine={machineFixture()} />, {
		router: { initialEntries: ["/machines/m-1"] },
	});
	await screen.findByText("docker.io/library/nginx:1.27");
}

describe("ImageList delete", () => {
	it("opens a confirmation modal and sends no delete request before confirming", async () => {
		resetCounters();
		server.use(imagesHandler(), deleteHandler());
		await renderImageList();

		await userEvent.click(
			screen.getAllByRole("button", { name: /delete docker\.io/i })[0],
		);
		expect(screen.getByTestId("modal")).toBeInTheDocument();
		expect(deleteRequests.count).toBe(0);
	});

	it("deletes without force on confirm and the row disappears after the list refetches", async () => {
		resetCounters();
		let remaining = [...imagesFixture];
		server.use(
			http.get("*/api/machines/m-1/images", () => HttpResponse.json(remaining)),
			deleteHandler(204, undefined),
		);
		server.use(
			http.delete("*/api/machines/m-1/images/*", ({ request }) => {
				deleteRequests.count++;
				deleteRequests.url = new URL(request.url);
				remaining = [];
				return new HttpResponse(null, { status: 204 });
			}),
		);
		await renderImageList();

		await userEvent.click(
			screen.getAllByRole("button", { name: /delete docker\.io/i })[0],
		);
		await userEvent.click(screen.getByTestId("confirm-delete-button"));

		await waitFor(() =>
			expect(screen.queryByTestId("modal")).not.toBeInTheDocument(),
		);
		expect(deleteRequests.count).toBe(1);
		expect(deleteRequests.url.searchParams.get("force")).toBeNull();
		expect(await screen.findByTestId("images-empty")).toBeInTheDocument();
	});

	it("explains in-use rejection and force-deletes when the user opts in", async () => {
		resetCounters();
		const usedByContainers = errorJson(
			"IMAGE_USED_BY_CONTAINERS",
			"Image is used by one or more containers",
			"req-11",
		);
		let forceRequested = false;
		let remaining = [...imagesFixture];
		server.use(
			http.get("*/api/machines/m-1/images", () => HttpResponse.json(remaining)),
			http.delete("*/api/machines/m-1/images/*", ({ request }) => {
				deleteRequests.count++;
				deleteRequests.url = new URL(request.url);
				forceRequested =
					new URL(request.url).searchParams.get("force") === "true";
				if (!forceRequested)
					return HttpResponse.json(usedByContainers, { status: 400 });
				remaining = [];
				return new HttpResponse(null, { status: 204 });
			}),
		);
		await renderImageList();

		await userEvent.click(
			screen.getAllByRole("button", { name: /delete docker\.io/i })[0],
		);
		await userEvent.click(screen.getByTestId("confirm-delete-button"));

		expect(
			await screen.findByTestId("image-in-use-warning"),
		).toBeInTheDocument();
		expect(screen.getByText(/containers use this image/i)).toBeInTheDocument();

		await userEvent.click(screen.getByTestId("force-delete-button"));

		await waitFor(() =>
			expect(screen.queryByTestId("modal")).not.toBeInTheDocument(),
		);
		expect(deleteRequests.count).toBe(2);
		expect(forceRequested).toBe(true);
		expect(await screen.findByTestId("images-empty")).toBeInTheDocument();
	});

	it("renders non-in-use delete errors through the shared error display inside the modal", async () => {
		resetCounters();
		server.use(
			http.get("*/api/machines/m-1/images", () =>
				HttpResponse.json(imagesFixture),
			),
			http.delete("*/api/machines/m-1/images/*", () =>
				errorResponse(
					400,
					"IMAGE_REMOVE_FAILED",
					"Image removal failed on the host",
					"req-12",
				),
			),
		);
		await renderImageList();

		await userEvent.click(
			screen.getAllByRole("button", { name: /delete docker\.io/i })[0],
		);
		await userEvent.click(screen.getByTestId("confirm-delete-button"));

		expect(await screen.findByRole("alert")).toBeInTheDocument();
		expect(screen.getByText("IMAGE_REMOVE_FAILED")).toBeInTheDocument();
		expect(
			screen.getByText("Image removal failed on the host"),
		).toBeInTheDocument();
		expect(screen.getByTestId("modal")).toBeInTheDocument();
		expect(screen.queryByTestId("force-delete-button")).not.toBeInTheDocument();
	});
});
