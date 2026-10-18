import { VolumeList } from "@features/volumes/components/VolumeList";
import { setBearerToken } from "@shared/utils/api";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { machineFixture, volumeFixture } from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

vi.mock("@shared/utils/download", () => ({
	downloadFile: vi.fn().mockResolvedValue(undefined),
}));

import { downloadFile } from "@shared/utils/download";

const mockedDownload = vi.mocked(downloadFile);

let exportRequests: { count: number; pathname: string } = {
	count: 0,
	pathname: "",
};

function resetCounters() {
	exportRequests = { count: 0, pathname: "" };
}

function installHandlers(
	exportResponse: () => { status: number; body: ArrayBuffer } = () => ({
		status: 200,
		body: new ArrayBuffer(0),
	}),
) {
	server.use(
		http.get("*/api/machines/m-1/volumes", () =>
			HttpResponse.json([volumeFixture({ name: "shared-data", size: 1024 })]),
		),
		http.get("*/api/machines/m-1/volumes/shared-data/export", ({ request }) => {
			exportRequests.count += 1;
			exportRequests.pathname = new URL(request.url).pathname;
			const response = exportResponse();
			if (response.status !== 200) {
				return HttpResponse.json(response.body, { status: response.status });
			}
			return HttpResponse.arrayBuffer(response.body, {
				headers: { "Content-Type": "application/x-tar" },
			});
		}),
	);
}

function renderVolumeList(permission: "read" | "write" | "admin" = "read") {
	renderWithQuery(<VolumeList machine={machineFixture({ permission })} />, {
		chakra: true,
		router: true,
	});
}

async function clickExport(permission: "read" | "write" | "admin" = "read") {
	renderVolumeList(permission);
	await screen.findByText("shared-data");
	const user = userEvent.setup();
	await user.click(screen.getByRole("button", { name: /export shared-data/i }));
	await waitFor(() => expect(mockedDownload).toHaveBeenCalled());
	return user;
}

beforeEach(() => {
	resetCounters();
	mockedDownload.mockClear();
	mockedDownload.mockResolvedValue(undefined);
});

afterEach(() => {
	setBearerToken(null);
});

describe("VolumeList export", () => {
	it("shows the export action to read-level users and starts a download", async () => {
		installHandlers();
		await clickExport("read");

		expect(mockedDownload).toHaveBeenCalledTimes(1);
	});

	it("saves the download as <volumeName>.tar against the export endpoint", async () => {
		installHandlers();
		await clickExport("read");

		expect(mockedDownload).toHaveBeenCalledWith(
			"/api/machines/m-1/volumes/shared-data/export",
			"shared-data.tar",
		);
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
		renderVolumeList("read");
		await screen.findByText("shared-data");
		const user = userEvent.setup();

		const button = screen.getByRole("button", {
			name: /export shared-data/i,
		});
		await user.click(button);
		await waitFor(() => expect(button).toHaveAttribute("data-loading"));

		resolveDownload?.();
		await waitFor(() => expect(button).not.toHaveAttribute("data-loading"));
	});

	it("toasts the standard error treatment when the export fails", async () => {
		installHandlers(() => ({
			status: 404,
			body: new ArrayBuffer(0),
		}));
		mockedDownload.mockRejectedValue({
			code: "VOLUME_NOT_FOUND",
			message: "no such volume",
		});
		renderVolumeList("read");
		await screen.findByText("shared-data");
		const user = userEvent.setup();

		await user.click(
			screen.getByRole("button", { name: /export shared-data/i }),
		);

		expect(await screen.findByText(/failed to export/i)).toBeInTheDocument();
		await waitFor(() =>
			expect(
				screen.getByRole("button", { name: /export shared-data/i }),
			).toBeEnabled(),
		);
	});
});
