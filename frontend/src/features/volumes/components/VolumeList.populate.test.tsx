import { VolumeList } from "@features/volumes/components/VolumeList";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { axiosError } from "@/test/errors";
import { machineFixture, volumeFixture } from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

// MSW cannot stream FormData bodies in this jsdom environment (the handler
// hangs on any body read), so the transport is mocked at the API-function
// seam; the multipart payload shape is pinned by volumesApi.populate.test.
const populateVolumeMock = vi.fn().mockResolvedValue(undefined);
vi.mock("@features/volumes/services/volumesApi", async (importOriginal) => {
	const actual =
		await importOriginal<
			typeof import("@features/volumes/services/volumesApi")
		>();
	return {
		...actual,
		populateVolume: (...args: unknown[]) => populateVolumeMock(...args),
	};
});

let populateRequests: { count: number; pathname: string } = {
	count: 0,
	pathname: "",
};

function resetCounters() {
	populateRequests = { count: 0, pathname: "" };
}

function installHandlers() {
	server.use(
		http.get("*/api/machines/m-1/volumes", () =>
			HttpResponse.json([volumeFixture({ name: "app-config" })]),
		),
		http.post("*/api/machines/m-1/volumes/app-config/populate", () => {
			populateRequests.count += 1;
			populateRequests.pathname =
				"/api/machines/m-1/volumes/app-config/populate";
			return new HttpResponse(null, { status: 204 });
		}),
	);
}

function renderVolumeList(permission: "read" | "write" | "admin" = "admin") {
	renderWithQuery(<VolumeList machine={machineFixture({ permission })} />, {
		chakra: true,
		router: true,
	});
}

function file(name: string, contents = "x"): File {
	return new File([contents], name, { type: "text/plain" });
}

async function openPopulateModal(
	permission: "read" | "write" | "admin" = "admin",
) {
	renderVolumeList(permission);
	await screen.findByText("app-config");
	const user = userEvent.setup();
	await user.click(
		screen.getByRole("button", { name: /populate app-config/i }),
	);
	expect(await screen.findByTestId("modal")).toBeInTheDocument();
	return user;
}

describe("VolumeList populate", () => {
	beforeEach(() => {
		resetCounters();
		populateVolumeMock.mockClear();
		populateVolumeMock.mockResolvedValue(undefined);
	});

	it("hides the populate action from read-level users", async () => {
		installHandlers();
		renderVolumeList("read");

		await screen.findByText("app-config");

		expect(
			screen.queryByRole("button", { name: /populate app-config/i }),
		).not.toBeInTheDocument();
		expect(populateRequests.count).toBe(0);
	});

	it("states the merge semantics before submit", async () => {
		installHandlers();
		await openPopulateModal();

		expect(screen.getByText(/merge/i)).toBeInTheDocument();
		expect(screen.getByText(/overwrite/i)).toBeInTheDocument();
		expect(screen.getByText(/kept/i)).toBeInTheDocument();
		expect(populateRequests.count).toBe(0);
	});

	it("uploads selected files through the API with a progress callback", async () => {
		installHandlers();
		const user = await openPopulateModal();

		const input = screen
			.getByTestId("populate-files-input")
			.querySelector<HTMLInputElement>("input[type=file]")!;
		await user.upload(input, [file("etc/app.conf"), file("bin/run.sh")]);
		await user.click(screen.getByTestId("confirm-populate-button"));

		await waitFor(() => expect(populateVolumeMock).toHaveBeenCalledTimes(1));
		const [machineId, volumeName, form, onProgress] =
			populateVolumeMock.mock.calls[0];
		expect(machineId).toBe("m-1");
		expect(volumeName).toBe("app-config");
		const parts = (form as FormData).getAll("files") as File[];
		expect(parts.map((part) => part.name)).toEqual([
			"etc/app.conf",
			"bin/run.sh",
		]);
		expect(onProgress).toBeTypeOf("function");
	});

	it("uploads a single tar via the archive input", async () => {
		installHandlers();
		const user = await openPopulateModal();

		const input = screen
			.getByTestId("populate-archive-input")
			.querySelector<HTMLInputElement>("input[type=file]")!;
		await user.upload(input, file("seed.tar"));
		await user.click(screen.getByTestId("confirm-populate-button"));

		await waitFor(() => expect(populateVolumeMock).toHaveBeenCalledTimes(1));
		const form = populateVolumeMock.mock.calls[0][2] as FormData;
		expect(form.getAll("archive").map((part) => (part as File).name)).toEqual([
			"seed.tar",
		]);
		expect(form.getAll("files")).toHaveLength(0);
	});

	it("prevents submitting both files and an archive at once", async () => {
		installHandlers();
		const user = await openPopulateModal();

		const filesInput = screen
			.getByTestId("populate-files-input")
			.querySelector<HTMLInputElement>("input[type=file]")!;
		const archiveInput = screen
			.getByTestId("populate-archive-input")
			.querySelector<HTMLInputElement>("input[type=file]")!;
		await user.upload(filesInput, file("a.txt"));
		await user.upload(archiveInput, file("seed.tar"));

		expect(screen.getByTestId("populate-shape-conflict")).toBeInTheDocument();
		expect(screen.getByTestId("confirm-populate-button")).toBeDisabled();

		await user.click(screen.getByTestId("confirm-populate-button"));
		expect(populateRequests.count).toBe(0);
	});

	it("shows upload progress while submitting", async () => {
		installHandlers();
		let resolveUpload: (() => void) | undefined;
		populateVolumeMock.mockImplementation(
			() =>
				new Promise<void>((resolve) => {
					resolveUpload = resolve;
				}),
		);
		const user = await openPopulateModal();

		const input = screen
			.getByTestId("populate-files-input")
			.querySelector<HTMLInputElement>("input[type=file]")!;
		await user.upload(input, file("slow.txt"));
		await user.click(screen.getByTestId("confirm-populate-button"));

		await waitFor(() => expect(populateVolumeMock).toHaveBeenCalled());
		const onProgress = populateVolumeMock.mock.calls[0][3] as (
			fraction: number,
		) => void;
		onProgress(0.5);
		expect(await screen.findByText(/populating 50%/i)).toBeInTheDocument();

		resolveUpload?.();
		await waitFor(() =>
			expect(screen.queryByTestId("modal")).not.toBeInTheDocument(),
		);
	});

	it("renders an actionable size-limit message on 413", async () => {
		installHandlers();
		populateVolumeMock.mockRejectedValue(
			axiosError(
				413,
				"VOLUME_UPLOAD_TOO_LARGE",
				"upload exceeds the configured limit",
				"req-9",
			),
		);
		const user = await openPopulateModal();

		const input = screen
			.getByTestId("populate-files-input")
			.querySelector<HTMLInputElement>("input[type=file]")!;
		await user.upload(input, file("huge.bin"));
		await user.click(screen.getByTestId("confirm-populate-button"));

		expect(await screen.findByRole("alert")).toBeInTheDocument();
		expect(screen.getByText(/split or shrink/i)).toBeInTheDocument();
		expect(
			screen.getByText(/upload exceeds the configured limit/i),
		).toBeInTheDocument();
	});

	it("closes the modal and toasts on success", async () => {
		installHandlers();
		const user = await openPopulateModal();

		const input = screen
			.getByTestId("populate-files-input")
			.querySelector<HTMLInputElement>("input[type=file]")!;
		await user.upload(input, file("ok.txt"));
		await user.click(screen.getByTestId("confirm-populate-button"));

		await waitFor(() =>
			expect(screen.queryByTestId("modal")).not.toBeInTheDocument(),
		);
		expect(await screen.findByText(/populated/i)).toBeInTheDocument();
	});
});
