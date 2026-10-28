import {
	LOG_ROUTE_PATTERN,
	type LogKind,
	logsPath,
} from "@shared/logs/logRoute";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { LogsPage } from "@/pages/LogsPage";
import { errorResponse } from "@/test/errors";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

// The Blob download is a DOM ritual pinned in download.test.ts; here it is
// mocked so the suite observes the click-to-download wiring (content and
// filename) without hand-rolled URL.createObjectURL stubs.
vi.mock("@shared/utils/download", async (importOriginal) => ({
	...(await importOriginal<typeof import("@shared/utils/download")>()),
	downloadText: vi.fn(),
}));

import { downloadText } from "@shared/utils/download";

const mockedDownloadText = vi.mocked(downloadText);

function renderLogsPage(
	kind: LogKind = "container",
	targetId = kind === "container" ? "c-1" : "q-1",
	name?: string,
) {
	renderWithQuery(
		<Routes>
			<Route path={LOG_ROUTE_PATTERN} element={<LogsPage />} />
		</Routes>,
		{
			chakra: true,
			router: {
				initialEntries: [
					{
						pathname: logsPath("m-1", kind, targetId),
						...(name === undefined ? {} : { state: { name } }),
					},
				],
			},
		},
	);
}

beforeEach(() => {
	mockedDownloadText.mockClear();
	mockedDownloadText.mockImplementation(() => undefined);
});

describe("LogsPage download", () => {
	it("fetches the full history and saves it as {name}-logs.txt", async () => {
		server.use(
			http.get("*/api/machines/m-1/containers/c-1/logs", () =>
				HttpResponse.json({
					id: "c-1",
					logs: "full-line-1\nfull-line-2\n",
				}),
			),
		);
		renderLogsPage("container", "c-1", "my-container");

		const user = userEvent.setup();
		await user.click(screen.getByRole("button", { name: /download/i }));

		await waitFor(() => expect(mockedDownloadText).toHaveBeenCalled());
		expect(mockedDownloadText).toHaveBeenCalledWith(
			"full-line-1\nfull-line-2\n",
			"my-container-logs.txt",
		);
	});

	it("falls back to the target id, sanitized, when no name traveled", async () => {
		server.use(
			http.get("*/api/machines/m-1/containers/*/logs", () =>
				HttpResponse.json({ id: "acme/app", logs: "line\n" }),
			),
		);
		// No navigation state: the raw target id is the name, and its slash
		// must not survive into the filename.
		renderLogsPage("container", "acme/app");

		const user = userEvent.setup();
		await user.click(screen.getByRole("button", { name: /download/i }));

		await waitFor(() => expect(mockedDownloadText).toHaveBeenCalled());
		expect(mockedDownloadText).toHaveBeenCalledWith(
			"line\n",
			"acme-app-logs.txt",
		);
	});

	it("serves the quadlet kind through the quadlet logs endpoint", async () => {
		server.use(
			http.get("*/api/machines/m-1/quadlets/q-1/logs", () =>
				HttpResponse.json({ id: "q-1", logs: "quadlet-line\n" }),
			),
		);
		renderLogsPage("quadlet", "q-1", "my-quadlet");

		const user = userEvent.setup();
		await user.click(screen.getByRole("button", { name: /download/i }));

		await waitFor(() => expect(mockedDownloadText).toHaveBeenCalled());
		expect(mockedDownloadText).toHaveBeenCalledWith(
			"quadlet-line\n",
			"my-quadlet-logs.txt",
		);
	});

	it("serves the pod kind through the pod logs endpoint", async () => {
		server.use(
			http.get("*/api/machines/m-1/pods/p-1/logs", () =>
				HttpResponse.json({ id: "p-1", logs: "pod-line\n" }),
			),
		);
		renderLogsPage("pod", "p-1", "my-pod");

		const user = userEvent.setup();
		await user.click(screen.getByRole("button", { name: /download/i }));

		await waitFor(() => expect(mockedDownloadText).toHaveBeenCalled());
		expect(mockedDownloadText).toHaveBeenCalledWith(
			"pod-line\n",
			"my-pod-logs.txt",
		);
	});

	it("disables the button while the fetch is in flight and re-enables after", async () => {
		// The viewer's tailed fetches answer immediately; the download's
		// fetch (no tail param) is held open until the test resolves it.
		let resolveDownload: ((logs: string) => void) | undefined;
		server.use(
			http.get("*/api/machines/m-1/containers/c-1/logs", ({ request }) => {
				if (new URL(request.url).searchParams.has("tail")) {
					return HttpResponse.json({ id: "c-1", logs: "tail-line\n" });
				}
				return new Promise((resolve) => {
					resolveDownload = (logs: string) =>
						resolve(HttpResponse.json({ id: "c-1", logs }));
				});
			}),
		);
		renderLogsPage("container", "c-1", "my-container");

		const user = userEvent.setup();
		const button = screen.getByRole("button", { name: /download/i });
		await user.click(button);

		await waitFor(() => expect(button).toBeDisabled());
		expect(mockedDownloadText).not.toHaveBeenCalled();

		resolveDownload?.("full-line\n");
		await waitFor(() => expect(mockedDownloadText).toHaveBeenCalled());
		await waitFor(() => expect(button).toBeEnabled());
	});

	it("toasts the API error and downloads nothing when the fetch fails", async () => {
		server.use(
			http.get("*/api/machines/m-1/containers/c-1/logs", () =>
				errorResponse(404, "CONTAINER_NOT_FOUND", "no such container"),
			),
		);
		renderLogsPage("container", "c-1", "my-container");

		const user = userEvent.setup();
		const button = screen.getByRole("button", { name: /download/i });
		await user.click(button);

		// The toast (role=status) carries the title and the API error
		// message; the viewer's own error display (role=alert) is separate.
		const toast = await screen.findByRole("status");
		expect(
			within(toast).getByText(/failed to download logs/i),
		).toBeInTheDocument();
		expect(within(toast).getByText("no such container")).toBeInTheDocument();
		expect(mockedDownloadText).not.toHaveBeenCalled();
		await waitFor(() => expect(button).toBeEnabled());
	});

	it("leaves the viewer's tail view untouched by the download", async () => {
		// The tailed fetch shows one line; the fetch-all returns the whole
		// history. The download saves the full text while the viewer keeps
		// displaying its tail.
		server.use(
			http.get("*/api/machines/m-1/containers/c-1/logs", ({ request }) =>
				HttpResponse.json(
					new URL(request.url).searchParams.has("tail")
						? { id: "c-1", logs: "tail-line\n" }
						: { id: "c-1", logs: "full-line-1\nfull-line-2\nfull-line-3\n" },
				),
			),
		);
		renderLogsPage("container", "c-1", "my-container");

		const user = userEvent.setup();
		await user.click(screen.getByRole("button", { name: /download/i }));

		await waitFor(() => expect(mockedDownloadText).toHaveBeenCalled());
		expect(mockedDownloadText).toHaveBeenCalledWith(
			"full-line-1\nfull-line-2\nfull-line-3\n",
			"my-container-logs.txt",
		);
		expect(await screen.findAllByTestId("log-viewer-line")).toHaveLength(1);
		expect(screen.getByText("tail-line")).toBeInTheDocument();
		expect(screen.queryByText("full-line-1")).not.toBeInTheDocument();
	});
});
