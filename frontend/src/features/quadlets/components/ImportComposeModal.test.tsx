import { ImportComposeModal } from "@features/quadlets/components/ImportComposeModal";
import { QuadletList } from "@features/quadlets/components/QuadletList";
import type { QuadletImportEntry } from "@features/quadlets/services/quadletsApi";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { describe, expect, it, vi } from "vitest";
import { errorResponse } from "@/test/errors";
import {
	machineFixture,
	quadletFixture,
	quadletImportEntryFixture,
	quadletImportResponseFixture,
} from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

const composeText = "services:\n  web:\n    image: nginx:1.27\n";

const webEntry = quadletImportEntryFixture();
const dbdataEntry = quadletImportEntryFixture({
	name: "dbdata",
	resource: "volume",
	content: "[Volume]\nDriver=local\n",
});

function convertHandler(entries: QuadletImportEntry[]) {
	return http.post("*/api/quadlets/convert-compose", () =>
		HttpResponse.json({ quadlets: entries }),
	);
}

function renderModal(onClose = vi.fn()) {
	renderWithQuery(<ImportComposeModal machineId="m-1" onClose={onClose} />, {
		router: true,
	});
	return onClose;
}

async function convertFromTextarea(user: ReturnType<typeof userEvent.setup>) {
	await user.type(screen.getByTestId("compose-content-input"), composeText);
	await user.click(screen.getByTestId("convert-compose-button"));
}

describe("ImportComposeModal", () => {
	it("walks input → preview → install report, forwarding the converted entries unchanged", async () => {
		const bodies: unknown[] = [];
		server.use(
			http.post("*/api/quadlets/convert-compose", async ({ request }) => {
				bodies.push(await request.json());
				return HttpResponse.json({ quadlets: [webEntry, dbdataEntry] });
			}),
			http.post("*/api/machines/m-1/quadlets/import", async ({ request }) => {
				bodies.push(await request.json());
				return HttpResponse.json(
					quadletImportResponseFixture({
						imported: [
							{
								id: "web",
								name: "web",
								path: "/etc/containers/systemd/web.container",
								state: "unknown",
							},
						],
						failed: [{ name: "dbdata", error: "permission denied" }],
					}),
					{ status: 201 },
				);
			}),
		);
		renderModal();
		const user = userEvent.setup();

		// Input phase: the convert button starts disabled on empty content.
		expect(screen.getByTestId("convert-compose-button")).toBeDisabled();
		await user.type(screen.getByTestId("compose-content-input"), composeText);
		expect(screen.getByTestId("convert-compose-button")).toBeEnabled();
		await user.click(screen.getByTestId("convert-compose-button"));

		// Preview phase: both generated files render read-only, with the
		// relative-path hint; the textarea is gone.
		const webFile = await screen.findByTestId("import-preview-file-web");
		expect(webFile).toHaveTextContent("web");
		expect(webFile).toHaveTextContent("[Container]");
		expect(webFile).toHaveTextContent("Image=docker.io/library/nginx:1.27");
		expect(screen.getByTestId("import-preview-file-dbdata")).toHaveTextContent(
			"[Volume]",
		);
		expect(screen.getByTestId("import-relative-path-hint")).toBeInTheDocument();
		expect(
			screen.queryByTestId("compose-content-input"),
		).not.toBeInTheDocument();

		// Install phase: the report renders per file.
		await user.click(screen.getByTestId("install-quadlets-button"));
		const imported = await screen.findByTestId("import-report-imported-web");
		expect(imported).toHaveTextContent("web");
		expect(imported).toHaveTextContent("/etc/containers/systemd/web.container");
		expect(imported).toHaveTextContent("unknown");
		const failed = screen.getByTestId("import-report-failed-dbdata");
		expect(failed).toHaveTextContent("dbdata");
		expect(failed).toHaveTextContent("permission denied");

		// The convert request carried the textarea text; the install request
		// carried the converted entries unchanged.
		expect(bodies).toEqual([
			{ content: composeText },
			{ quadlets: [webEntry, dbdataEntry] },
		]);
	});

	it("loads the picked file's content into the textarea and converts exactly that text as JSON", async () => {
		const captured: { body: unknown; contentType: string | null } = {
			body: null,
			contentType: null,
		};
		server.use(
			http.post("*/api/quadlets/convert-compose", async ({ request }) => {
				captured.contentType = request.headers.get("content-type");
				captured.body = await request.json();
				return HttpResponse.json({ quadlets: [webEntry] });
			}),
		);
		renderModal();
		const user = userEvent.setup();

		const file = new File([composeText], "docker-compose.yml", {
			type: "text/yaml",
		});
		await user.upload(screen.getByTestId("compose-file-input"), file);

		// One source of truth: the textarea shows exactly what gets converted.
		expect(screen.getByTestId("compose-content-input")).toHaveValue(
			composeText,
		);
		await user.click(screen.getByTestId("convert-compose-button"));
		await screen.findByTestId("import-preview-file-web");

		expect(captured.contentType).toContain("application/json");
		expect(captured.body).toEqual({ content: composeText });
	});

	it("renders a conversion rejection's message verbatim (podlet stderr)", async () => {
		const podletStderr =
			"error converting service `app`\nerror: `image` and `build` cannot both be set";
		server.use(
			http.post("*/api/quadlets/convert-compose", () =>
				errorResponse(400, "VALIDATION_FAILED", podletStderr),
			),
		);
		renderModal();
		const user = userEvent.setup();

		await convertFromTextarea(user);

		const error = await screen.findByTestId("import-convert-error");
		expect(error).toHaveTextContent("VALIDATION_FAILED");
		// The backend sends podlet's stderr verbatim (ANSI already stripped);
		// the modal renders it unchanged, newlines included.
		expect(error.textContent).toContain(podletStderr);
		// The failure keeps the modal in the input phase.
		expect(screen.getByTestId("compose-content-input")).toBeInTheDocument();
	});

	it("aborts from the preview without sending an import request", async () => {
		let importCalls = 0;
		server.use(
			convertHandler([webEntry]),
			http.post("*/api/machines/m-1/quadlets/import", () => {
				importCalls++;
				return HttpResponse.json(quadletImportResponseFixture(), {
					status: 201,
				});
			}),
		);
		const onClose = renderModal();
		const user = userEvent.setup();

		await convertFromTextarea(user);
		await screen.findByTestId("import-preview-file-web");
		await user.click(screen.getByTestId("import-compose-cancel-button"));

		expect(onClose).toHaveBeenCalledTimes(1);
		expect(importCalls).toBe(0);
	});

	it("renders an install failure in the modal and stays in the preview phase", async () => {
		server.use(
			convertHandler([webEntry]),
			http.post("*/api/machines/m-1/quadlets/import", () =>
				errorResponse(404, "MACHINE_NOT_FOUND", "machine not found"),
			),
		);
		renderModal();
		const user = userEvent.setup();

		await convertFromTextarea(user);
		await screen.findByTestId("import-preview-file-web");
		await user.click(screen.getByTestId("install-quadlets-button"));

		const error = await screen.findByTestId("import-install-error");
		expect(error).toHaveTextContent("MACHINE_NOT_FOUND");
		expect(error).toHaveTextContent("machine not found");
		// Still in the preview phase: the files render and install can retry.
		expect(screen.getByTestId("import-preview-file-web")).toBeInTheDocument();
		expect(screen.getByTestId("install-quadlets-button")).toBeEnabled();
	});

	it("starts clean when reopened — no leaked previous conversion", async () => {
		server.use(
			http.get("*/api/machines/m-1/quadlets", () =>
				HttpResponse.json([quadletFixture()]),
			),
			convertHandler([webEntry]),
		);
		renderWithQuery(<QuadletList machine={machineFixture()} />, {
			router: true,
		});
		const user = userEvent.setup();
		await screen.findByText("web");

		// First open: type, convert, reach the preview, then close.
		await user.click(screen.getByTestId("import-compose-button"));
		await convertFromTextarea(user);
		await screen.findByTestId("import-preview-file-web");
		await user.click(screen.getByTestId("import-compose-cancel-button"));
		expect(screen.queryByTestId("modal")).not.toBeInTheDocument();

		// Reopened: input phase, empty textarea, no leaked preview.
		await user.click(screen.getByTestId("import-compose-button"));
		expect(screen.getByTestId("compose-content-input")).toHaveValue("");
		expect(screen.getByTestId("convert-compose-button")).toBeDisabled();
		expect(
			screen.queryByTestId("import-preview-file-web"),
		).not.toBeInTheDocument();
	});

	it("refreshes the quadlets list after a successful install", async () => {
		let listCalls = 0;
		server.use(
			http.get("*/api/machines/m-1/quadlets", () => {
				listCalls++;
				return HttpResponse.json([quadletFixture()]);
			}),
			convertHandler([webEntry]),
			http.post("*/api/machines/m-1/quadlets/import", () =>
				HttpResponse.json(quadletImportResponseFixture(), { status: 201 }),
			),
		);
		renderWithQuery(<QuadletList machine={machineFixture()} />, {
			router: true,
		});
		const user = userEvent.setup();
		await screen.findByText("web");
		expect(listCalls).toBe(1);

		await user.click(screen.getByTestId("import-compose-button"));
		await convertFromTextarea(user);
		await screen.findByTestId("import-preview-file-web");
		await user.click(screen.getByTestId("install-quadlets-button"));
		await screen.findByTestId("import-report-imported-web");

		await waitFor(() => expect(listCalls).toBe(2));
	});
});
