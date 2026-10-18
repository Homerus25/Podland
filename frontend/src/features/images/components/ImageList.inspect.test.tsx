import { ImageList } from "@features/images/components/ImageList";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import { errorResponse } from "@/test/errors";
import {
	imageFixture,
	machineFixture,
	rawInspectFixture,
} from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

const image = imageFixture();

function imagesHandler() {
	return http.get("*/api/machines/m-1/images", () =>
		HttpResponse.json([image]),
	);
}

// The image id is "sha256:…", so machineApiPath percent-encodes the segment;
// the wildcard matches the encoded id.
function inspectHandler() {
	return http.get("*/api/machines/m-1/images/*/inspect", () =>
		HttpResponse.json(rawInspectFixture()),
	);
}

function renderImageList() {
	renderWithQuery(<ImageList machine={machineFixture()} />);
}

describe("ImageList inspect", () => {
	it("opens the inspect dialog titled with the image's tag and shows its JSON", async () => {
		const requestedPaths: string[] = [];
		server.use(
			imagesHandler(),
			http.get("*/api/machines/m-1/images/*/inspect", ({ request }) => {
				requestedPaths.push(new URL(request.url).pathname);
				return HttpResponse.json(rawInspectFixture());
			}),
		);
		renderImageList();
		const user = userEvent.setup();

		await user.click(await screen.findByTestId("image-row"));

		const modal = await screen.findByTestId("modal");
		expect(
			within(modal).getByText("docker.io/library/nginx:1.27"),
		).toBeInTheDocument();
		expect(screen.getByTestId("inspect-dialog-json").textContent).toBe(
			JSON.stringify(rawInspectFixture(), null, 2),
		);
		expect(requestedPaths).toEqual([
			`/api/machines/m-1/images/${encodeURIComponent(image.id)}/inspect`,
		]);
	});

	it("closing the dialog returns to the list", async () => {
		server.use(imagesHandler(), inspectHandler());
		renderImageList();
		const user = userEvent.setup();

		await user.click(await screen.findByTestId("image-row"));
		await screen.findByTestId("inspect-dialog-json");
		await user.click(screen.getByRole("button", { name: "Close dialog" }));

		await waitFor(() =>
			expect(screen.queryByTestId("modal")).not.toBeInTheDocument(),
		);
		expect(screen.getByTestId("image-row")).toBeInTheDocument();
	});

	it("renders the error region inside the dialog when the endpoint 404s", async () => {
		server.use(
			imagesHandler(),
			http.get("*/api/machines/m-1/images/*/inspect", () =>
				errorResponse(404, "IMAGE_NOT_FOUND", "Image not found"),
			),
		);
		renderImageList();
		const user = userEvent.setup();

		await user.click(await screen.findByTestId("image-row"));

		const alert = await screen.findByTestId("inspect-dialog-error");
		expect(alert).toHaveTextContent("IMAGE_NOT_FOUND");
		expect(alert).toHaveTextContent("Image not found");
		expect(screen.queryByTestId("inspect-dialog-json")).not.toBeInTheDocument();
	});

	it("does not open the inspect dialog from the row's delete button", async () => {
		let inspectRequests = 0;
		server.use(
			imagesHandler(),
			http.get("*/api/machines/m-1/images/*/inspect", () => {
				inspectRequests++;
				return HttpResponse.json(rawInspectFixture());
			}),
		);
		renderImageList();
		const user = userEvent.setup();

		const row = await screen.findByTestId("image-row");
		await user.click(
			within(row).getByRole("button", {
				name: "Delete docker.io/library/nginx:1.27",
			}),
		);

		// The delete confirm opens, not the inspect dialog.
		expect(
			await screen.findByTestId("confirm-delete-button"),
		).toBeInTheDocument();
		expect(inspectRequests).toBe(0);
		expect(
			screen.queryByTestId("inspect-dialog-loading"),
		).not.toBeInTheDocument();
		expect(screen.queryByTestId("inspect-dialog-json")).not.toBeInTheDocument();
		expect(
			screen.queryByTestId("inspect-dialog-error"),
		).not.toBeInTheDocument();
	});
});
