import { ImageList } from "@features/images/components/ImageList";
import { setBearerToken } from "@shared/utils/api";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { useLocation } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { hostFixture, imageFixture } from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

vi.mock("@shared/utils/download", () => ({
	downloadFile: vi.fn().mockResolvedValue(undefined),
}));

import { downloadFile } from "@shared/utils/download";

const mockedDownload = vi.mocked(downloadFile);

const image = imageFixture({
	id: "sha256:abc123def4567890abc123def4567890abc123def4567890abc123def4567890",
});
const encodedImageId = encodeURIComponent(image.id);
const exportUrl = `/api/hosts/m-1/images/${encodedImageId}/export`;

function installHandlers() {
	server.use(
		http.get("*/api/hosts/m-1/images", () => HttpResponse.json([image])),
		http.get("*/api/hosts/m-1/images/*/export", () =>
			HttpResponse.arrayBuffer(new ArrayBuffer(0), {
				headers: {
					"Content-Type": "application/x-tar",
					"Content-Disposition": 'attachment; filename="abc123def456.tar"',
				},
			}),
		),
	);
}

function renderImageList(permission: "read" | "write" | "admin" = "read") {
	renderWithQuery(<ImageList host={hostFixture({ permission })} />, {
		chakra: true,
		router: { initialEntries: ["/hosts/m-1"] },
	});
}

// The row click navigates, so the stopPropagation test observes the
// location through the public interface: a probe reading the router's
// pathname (the ImageList suite's pattern).
function LocationProbe() {
	const location = useLocation();
	return <div data-testid="location">{location.pathname}</div>;
}

async function clickDownload(permission: "read" | "write" | "admin" = "read") {
	renderImageList(permission);
	await screen.findByText("docker.io/library/nginx:1.27");
	const user = userEvent.setup();
	await user.click(
		screen.getByRole("button", {
			name: /download docker.io\/library\/nginx:1\.27/i,
		}),
	);
	await waitFor(() => expect(mockedDownload).toHaveBeenCalled());
	return user;
}

function mockDownloadReset() {
	mockedDownload.mockClear();
	mockedDownload.mockResolvedValue(undefined);
}

beforeEach(() => {
	mockDownloadReset();
});

afterEach(() => {
	setBearerToken(null);
});

describe("ImageList download", () => {
	it("shows the download action to read-level users and starts a download", async () => {
		installHandlers();
		await clickDownload("read");

		expect(mockedDownload).toHaveBeenCalledTimes(1);
	});

	it("downloads against the export endpoint under the server-provided filename", async () => {
		installHandlers();
		await clickDownload("read");

		// No filename argument: the shared helper saves under the
		// server's Content-Disposition name.
		expect(mockedDownload).toHaveBeenCalledWith(exportUrl);
	});

	it("shows a per-row busy state while the download runs", async () => {
		installHandlers();
		let resolveDownload: (() => void) | undefined;
		mockedDownload.mockImplementation(
			() =>
				new Promise<void>((resolve) => {
					resolveDownload = resolve;
				}),
		);
		renderImageList("read");
		await screen.findByText("docker.io/library/nginx:1.27");
		const user = userEvent.setup();

		const button = screen.getByRole("button", {
			name: /download docker.io\/library\/nginx:1\.27/i,
		});
		await user.click(button);
		await waitFor(() => expect(button).toHaveAttribute("data-loading"));

		resolveDownload?.();
		await waitFor(() => expect(button).not.toHaveAttribute("data-loading"));
	});

	it("toasts an error naming the image when the download fails", async () => {
		installHandlers();
		mockDownloadReset();
		mockedDownload.mockRejectedValue({
			code: "IMAGE_NOT_FOUND",
			message: "no such image",
		});
		renderImageList("read");
		await screen.findByText("docker.io/library/nginx:1.27");
		const user = userEvent.setup();

		await user.click(
			screen.getByRole("button", {
				name: /download docker.io\/library\/nginx:1\.27/i,
			}),
		);

		expect(
			await screen.findByText(
				/failed to download image: docker.io\/library\/nginx:1\.27 \(abc123def456\)/i,
			),
		).toBeInTheDocument();
		await waitFor(() =>
			expect(
				screen.getByRole("button", {
					name: /download docker.io\/library\/nginx:1\.27/i,
				}),
			).toBeEnabled(),
		);
	});

	it("does not navigate to the detail page when the download button is clicked", async () => {
		installHandlers();
		renderWithQuery(
			<>
				<LocationProbe />
				<ImageList host={hostFixture({ permission: "read" })} />
			</>,
			{ chakra: true, router: { initialEntries: ["/hosts/m-1"] } },
		);
		const user = userEvent.setup();

		await user.click(
			await screen.findByRole("button", {
				name: /download docker.io\/library\/nginx:1\.27/i,
			}),
		);

		await waitFor(() => expect(mockedDownload).toHaveBeenCalled());
		expect(screen.getByTestId("location").textContent).toBe("/hosts/m-1");
	});
});
