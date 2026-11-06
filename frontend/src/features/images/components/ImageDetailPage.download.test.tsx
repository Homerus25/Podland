import { ImageDetailPage } from "@features/images/components/ImageDetailPage";
import { setBearerToken } from "@shared/utils/api";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { Route, Routes } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { hostFixture, imageDetailFixture } from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

// The image detail page's Download action (spec 477, ticket #481): the
// header-actions slot, the read-level (ungated) render, the export
// endpoint, the busy state, and the error toast. The download helper does
// a real fetch + blob + anchor click, which jsdom cannot usefully observe,
// so the single API function is mocked (the sanctioned partial-mock
// pattern) and the page's own logic is what these tests assert.

vi.mock("@shared/utils/download", () => ({
	downloadFile: vi.fn().mockResolvedValue(undefined),
}));

import { downloadFile } from "@shared/utils/download";

const mockedDownload = vi.mocked(downloadFile);

const image = imageDetailFixture();
const encodedImageId = encodeURIComponent(image.id);
const exportUrl = `/api/hosts/m-1/images/${encodedImageId}/export`;

// The page reads the host's per-caller facts for the manage gate; Download
// itself is read-level, so a read grant must still render it.
function mockHost(permission: "read" | "write" | "admin" = "read") {
	server.use(
		http.get("*/api/hosts/m-1", () =>
			HttpResponse.json(
				hostFixture({ permission, isOwner: permission === "admin" }),
			),
		),
	);
}

function mockImageDetail() {
	server.use(
		http.get("*/api/hosts/m-1/images/:imageId", () => HttpResponse.json(image)),
	);
}

function renderDetailPage() {
	return renderWithQuery(
		<Routes>
			<Route
				path="/hosts/:hostId/images/:imageId"
				element={<ImageDetailPage />}
			/>
		</Routes>,
		{
			chakra: true,
			router: {
				initialEntries: [`/hosts/m-1/images/${encodedImageId}`],
			},
		},
	);
}

function downloadButton() {
	return screen.getByTestId("download-image-button");
}

beforeEach(() => {
	mockedDownload.mockClear();
	mockedDownload.mockResolvedValue(undefined);
});

afterEach(() => {
	setBearerToken(null);
});

describe("ImageDetailPage download", () => {
	it("renders the Download action for a read-level caller", async () => {
		mockHost("read");
		mockImageDetail();
		renderDetailPage();

		expect(
			await screen.findByTestId("download-image-button"),
		).toHaveTextContent("Download");
	});

	// The header action row's one owner renders an ungated action whatever
	// the gate says, so a pending host query must not take Download down
	// with the Copy action it is still deciding about.
	it("renders the Download action while the host query is pending", async () => {
		let release: () => void = () => {};
		const gate = new Promise<void>((resolve) => {
			release = resolve;
		});
		server.use(
			http.get("*/api/hosts/m-1", async () => {
				await gate;
				return HttpResponse.json(hostFixture({ permission: "write" }));
			}),
		);
		mockImageDetail();
		renderDetailPage();

		expect(
			await screen.findByTestId("download-image-button"),
		).toHaveTextContent("Download");
		expect(screen.queryByTestId("copy-image-button")).not.toBeInTheDocument();
		expect(
			screen.queryByTestId("images-permission-hint"),
		).not.toBeInTheDocument();

		release();
		await screen.findByTestId("copy-image-button");
	});

	it("streams the archive from the export endpoint under the server-provided filename", async () => {
		mockHost("read");
		mockImageDetail();
		renderDetailPage();
		const user = userEvent.setup();

		await user.click(await screen.findByTestId("download-image-button"));

		await waitFor(() => expect(mockedDownload).toHaveBeenCalledTimes(1));
		// No filename argument: the shared helper saves under the server's
		// Content-Disposition name, exactly as the list row's button does.
		expect(mockedDownload).toHaveBeenCalledWith(exportUrl);
	});

	it("shows a busy state while the download runs", async () => {
		mockHost("read");
		mockImageDetail();
		let resolveDownload: (() => void) | undefined;
		mockedDownload.mockImplementation(
			() =>
				new Promise<void>((resolve) => {
					resolveDownload = resolve;
				}),
		);
		renderDetailPage();
		const user = userEvent.setup();

		const button = await screen.findByTestId("download-image-button");
		await user.click(button);
		await waitFor(() => expect(button).toHaveAttribute("data-loading"));

		resolveDownload?.();
		await waitFor(() => expect(button).not.toHaveAttribute("data-loading"));
	});

	it("toasts an error naming the image when the download fails", async () => {
		mockHost("read");
		mockImageDetail();
		mockedDownload.mockRejectedValue({
			code: "IMAGE_NOT_FOUND",
			message: "no such image",
		});
		renderDetailPage();
		const user = userEvent.setup();

		await user.click(await screen.findByTestId("download-image-button"));

		expect(
			await screen.findByText(
				/failed to download image: docker\.io\/library\/nginx:1\.27, docker\.io\/library\/nginx:latest \(abc123def456\)/i,
			),
		).toBeInTheDocument();
		await waitFor(() => expect(downloadButton()).toBeEnabled());
	});
});
