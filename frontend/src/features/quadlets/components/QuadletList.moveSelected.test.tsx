import { QuadletList } from "@features/quadlets/components/QuadletList";
import type {
	QuadletMovePreview,
	QuadletMoveResult,
	QuadletSummary,
} from "@features/quadlets/services/quadletsApi";
import type { Host } from "@shared/types/api";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http, type JsonBodyType } from "msw";
import { beforeEach, describe, expect, it } from "vitest";
import { errorJson } from "@/test/errors";
import {
	hostFixture,
	quadletFixture,
	quadletMoveCompanionVolumeFixture,
	quadletMovePreviewFixture,
	quadletMoveResultFixture,
} from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

// The Quadlets "Move/copy selected" adapter (spec 486, ticket #489) at the
// MSW/HTTP seam. The batch contract itself — one target and mode for the
// whole selection, per-item previews and warnings, sequential execution, the
// per-item report, the succeeded keys — is pinned once in the shared
// MultiTransferDialog suite. This suite keeps adapter smokes plus the
// quadlet domain the adapter owns: the per-item request bodies the payload
// builder folds, the fixed stop-before-moving default the batch states, and
// the warnings rendered under each quadlet's name.

const web = quadletFixture({ id: "web", name: "web", state: "stopped" });
const db = quadletFixture({
	id: "db",
	name: "db",
	unit: "db.service",
	state: "stopped",
});

const source = hostFixture();
const staging = hostFixture({ id: "m-2", name: "staging-01" });
const backup = hostFixture({ id: "m-4", name: "backup-01" });
const readOnly = hostFixture({
	id: "m-3",
	name: "laptop-01",
	permission: "read",
	isOwner: false,
});

let moveRequests: { id: string; body: Record<string, unknown> }[] = [];
let previewRequests: string[] = [];

function resetCounters() {
	moveRequests = [];
	previewRequests = [];
}

function hostsHandler(hosts: Host[]) {
	return http.get("*/api/hosts", () => HttpResponse.json(hosts));
}

function quadletsHandler(quadlets: () => QuadletSummary[]) {
	return http.get("*/api/hosts/m-1/quadlets", () =>
		HttpResponse.json(quadlets()),
	);
}

// One preview per selected quadlet, addressed by the quadlet's id — the
// existing single-item route, unchanged.
function previewHandler(overrides: Partial<QuadletMovePreview> = {}) {
	return http.get(
		"*/api/hosts/m-1/quadlets/:quadletId/move-preview",
		({ params }) => {
			previewRequests.push(String(params.quadletId));
			return HttpResponse.json(quadletMovePreviewFixture(overrides));
		},
	);
}

// The per-item transfer endpoint: one plain JSON envelope each, addressed by
// the quadlet's id — the existing single-item route, unchanged.
function moveHandler(
	result: (id: string) => QuadletMoveResult | HttpResponse<JsonBodyType>,
) {
	return http.post(
		"*/api/hosts/m-1/quadlets/:quadletId/move",
		async ({ request, params }) => {
			const id = String(params.quadletId);
			moveRequests.push({
				id,
				body: (await request.json()) as Record<string, unknown>,
			});
			const answer = result(id);
			return answer instanceof HttpResponse
				? answer
				: HttpResponse.json(answer);
		},
	);
}

function renderQuadletList(permission: "read" | "write" | "admin" = "admin") {
	return renderWithQuery(
		<QuadletList
			host={hostFixture({ permission, isOwner: permission === "admin" })}
		/>,
		// The row navigates on click, so the list renders inside a router.
		{ router: true },
	);
}

async function selectQuadlets(...names: string[]) {
	const user = userEvent.setup();
	for (const name of names) {
		await user.click(screen.getByRole("checkbox", { name: `Select ${name}` }));
	}
	return user;
}

describe("QuadletList move/copy selected", () => {
	beforeEach(resetCounters);

	it("renders Move/copy selected permanently for a manage-quadlets caller, disabled on an empty selection", async () => {
		server.use(
			quadletsHandler(() => [web, db]),
			hostsHandler([source, staging, readOnly, backup]),
		);
		renderQuadletList();
		await screen.findByText(web.name);

		// Permanently rendered — the button disables itself rather than
		// appearing and disappearing with the selection.
		const button = screen.getByTestId("move-selected-quadlets-button");
		expect(button).toBeDisabled();

		await selectQuadlets(web.name);
		expect(screen.getByTestId("move-selected-quadlets-button")).toBeEnabled();
	});

	it("swaps the toolbar for the permission hint below manage-quadlets", async () => {
		server.use(
			quadletsHandler(() => [web, db]),
			hostsHandler([source, staging, readOnly, backup]),
		);
		renderQuadletList("read");
		await screen.findByText(web.name);

		expect(
			screen.queryByTestId("move-selected-quadlets-button"),
		).not.toBeInTheDocument();
		expect(screen.getByTestId("quadlets-permission-hint")).toBeInTheDocument();
	});

	it("renders no row Move/Copy button any more, keeping the lifecycle, Logs, and Edit actions", async () => {
		server.use(
			quadletsHandler(() => [web, db]),
			hostsHandler([source, staging, readOnly, backup]),
		);
		renderQuadletList();

		const rows = await screen.findAllByTestId("quadlet-row");
		expect(rows).toHaveLength(2);
		for (const row of rows) {
			expect(
				within(row).queryByRole("button", { name: /move/i }),
			).not.toBeInTheDocument();
			expect(
				within(row).queryByTestId("move-quadlet-button"),
			).not.toBeInTheDocument();
		}
		// The row keeps its surviving actions: lifecycle, Logs, and Edit.
		const first = rows[0];
		expect(
			within(first).getByRole("button", { name: `Start ${web.name}` }),
		).toBeInTheDocument();
		expect(
			within(first).getByRole("button", { name: `Restart ${web.name}` }),
		).toBeInTheDocument();
		expect(
			within(first).getByRole("button", { name: `Logs ${web.name}` }),
		).toBeInTheDocument();
		expect(
			within(first).getByRole("button", { name: `Edit ${web.name}` }),
		).toBeInTheDocument();
	});

	it("opens one dialog naming the selected quadlets, sending nothing yet", async () => {
		server.use(
			quadletsHandler(() => [web, db]),
			hostsHandler([source, staging, readOnly, backup]),
			previewHandler(),
			moveHandler((id) => quadletMoveResultFixture({ id, name: id })),
		);
		renderQuadletList();
		await screen.findByText(web.name);
		const user = await selectQuadlets(web.name, db.name);

		await user.click(screen.getByTestId("move-selected-quadlets-button"));

		const modal = await screen.findByTestId("modal");
		expect(modal).toHaveTextContent("Move or copy 2 quadlets");
		const blocks = screen.getAllByTestId("multi-transfer-item");
		expect(blocks).toHaveLength(2);
		expect(within(blocks[0]).getByText(web.name)).toBeInTheDocument();
		expect(within(blocks[1]).getByText(db.name)).toBeInTheDocument();
		// One target and one mode for the whole batch.
		expect(screen.getAllByTestId("move-target-select")).toHaveLength(1);
		expect(screen.getAllByTestId("move-mode-radio")).toHaveLength(1);
		expect(moveRequests).toHaveLength(0);
	});

	it("states that the batch stops each quadlet before moving it", async () => {
		server.use(
			quadletsHandler(() => [web, db]),
			hostsHandler([source, staging, readOnly, backup]),
			previewHandler(),
			moveHandler((id) => quadletMoveResultFixture({ id, name: id })),
		);
		renderQuadletList();
		await screen.findByText(web.name);
		const user = await selectQuadlets(web.name, db.name);

		await user.click(screen.getByTestId("move-selected-quadlets-button"));
		await screen.findByTestId("modal");

		// The batch offers no per-item option checkboxes, so the fixed
		// stop-before-moving default is stated rather than chosen.
		expect(screen.getByTestId("move-batch-stop-note")).toHaveTextContent(
			"Each selected quadlet is stopped before it moves.",
		);
	});

	it("fetches one preview per selected quadlet for the chosen target", async () => {
		server.use(
			quadletsHandler(() => [web, db]),
			hostsHandler([source, staging, readOnly, backup]),
			previewHandler(),
			moveHandler((id) => quadletMoveResultFixture({ id, name: id })),
		);
		renderQuadletList();
		await screen.findByText(web.name);
		const user = await selectQuadlets(web.name, db.name);

		await user.click(screen.getByTestId("move-selected-quadlets-button"));
		await screen.findByTestId("modal");

		await waitFor(() =>
			expect(previewRequests.sort()).toEqual([db.name, web.name]),
		);
	});

	it("renders each quadlet's volume and companion warnings under its own name", async () => {
		server.use(
			quadletsHandler(() => [web, db]),
			hostsHandler([source, staging, readOnly, backup]),
			previewHandler({
				volumes: [quadletMoveCompanionVolumeFixture("absent")],
			}),
			moveHandler((id) => quadletMoveResultFixture({ id, name: id })),
		);
		renderQuadletList();
		await screen.findByText(web.name);
		const user = await selectQuadlets(web.name, db.name);

		await user.click(screen.getByTestId("move-selected-quadlets-button"));
		await screen.findByTestId("modal");

		const blocks = await screen.findAllByTestId("multi-transfer-item");
		expect(blocks).toHaveLength(2);
		for (const block of blocks) {
			// The single-item dialog's warning lists, per quadlet: the
			// stranded-data volume list with its companion note, and the
			// acknowledgement checkbox the batch deliberately omits.
			expect(
				within(block).getByTestId("move-volume-warning"),
			).toHaveTextContent("it stays behind on prod-web-01");
			expect(
				within(block).getByTestId("move-companion-note"),
			).toHaveTextContent("definition will be copied");
			expect(
				within(block).queryByTestId("move-volume-confirm-option"),
			).not.toBeInTheDocument();
		}
	});

	it("clears only the successes from the selection after a run", async () => {
		server.use(
			quadletsHandler(() => [web, db]),
			hostsHandler([source, staging, readOnly, backup]),
			previewHandler({ volumes: [] }),
			moveHandler((id) =>
				id === db.id
					? HttpResponse.json(
							errorJson("QUADLET_ALREADY_EXISTS", "Quadlet already exists"),
							{ status: 409 },
						)
					: quadletMoveResultFixture({ id, name: id }),
			),
		);
		renderQuadletList();
		await screen.findByText(web.name);
		const user = await selectQuadlets(web.name, db.name);

		await user.click(screen.getByTestId("move-selected-quadlets-button"));
		await screen.findByTestId("modal");
		await waitFor(() =>
			expect(screen.getByTestId("confirm-multi-transfer-button")).toBeEnabled(),
		);

		await user.click(screen.getByTestId("confirm-multi-transfer-button"));

		// The report is the run's completion signal; what it says is the
		// shared dialog's contract, pinned in its own suite.
		await screen.findByTestId("multi-transfer-report");

		// The succeeded quadlet leaves the selection; the failed one stays
		// selected for the retry.
		await user.click(screen.getByTestId("close-multi-transfer-report-button"));
		await waitFor(() =>
			expect(screen.queryByTestId("modal")).not.toBeInTheDocument(),
		);
		expect(
			screen.getByRole("checkbox", { name: `Select ${web.name}` }),
		).not.toBeChecked();
		expect(
			screen.getByRole("checkbox", { name: `Select ${db.name}` }),
		).toBeChecked();
	});

	it("folds each quadlet's own preview facts into its request", async () => {
		server.use(
			quadletsHandler(() => [web, db]),
			hostsHandler([source, staging, readOnly, backup]),
			// web's preview reports a companion .volume definition, db's does
			// not — so the two requests differ under one target and mode.
			http.get(
				"*/api/hosts/m-1/quadlets/:quadletId/move-preview",
				({ params }) =>
					HttpResponse.json(
						quadletMovePreviewFixture({
							volumes:
								String(params.quadletId) === web.id
									? [quadletMoveCompanionVolumeFixture("absent")]
									: [],
						}),
					),
			),
			moveHandler((id) => quadletMoveResultFixture({ id, name: id })),
		);
		renderQuadletList();
		await screen.findByText(web.name);
		const user = await selectQuadlets(web.name, db.name);

		await user.click(screen.getByTestId("move-selected-quadlets-button"));
		await screen.findByTestId("modal");
		await waitFor(() =>
			expect(screen.getByTestId("confirm-multi-transfer-button")).toBeEnabled(),
		);
		await user.click(screen.getByTestId("confirm-multi-transfer-button"));

		await screen.findByTestId("multi-transfer-report");
		// Order is the batch's business; what the adapter owns is that each
		// quadlet's own preview facts land in its own request.
		const bodiesById = Object.fromEntries(
			moveRequests.map((request) => [request.id, request.body]),
		);
		expect(bodiesById).toEqual({
			[web.id]: {
				targetHostId: "m-2",
				mode: "move",
				stop: true,
				force: false,
				transferVolumeQuadlets: true,
			},
			[db.id]: {
				targetHostId: "m-2",
				mode: "move",
				stop: true,
				force: false,
			},
		});
	});

	it("sends the copy body without the move-only stop/force flags on a mode switch", async () => {
		server.use(
			quadletsHandler(() => [web, db]),
			hostsHandler([source, staging, readOnly, backup]),
			previewHandler({ volumes: [] }),
			moveHandler((id) => quadletMoveResultFixture({ id, name: id })),
		);
		renderQuadletList();
		await screen.findByText(web.name);
		const user = await selectQuadlets(web.name, db.name);

		await user.click(screen.getByTestId("move-selected-quadlets-button"));
		await screen.findByTestId("modal");
		await user.click(
			screen.getByRole("radio", { name: "Copy (keep this host's quadlet)" }),
		);
		await waitFor(() =>
			expect(screen.getByTestId("confirm-multi-transfer-button")).toBeEnabled(),
		);
		await user.click(screen.getByTestId("confirm-multi-transfer-button"));

		await screen.findByTestId("multi-transfer-report");
		for (const request of moveRequests) {
			expect(request.body).toEqual({ targetHostId: "m-2", mode: "copy" });
		}
	});

	it("treats a partial transfer as a failure, so the quadlet stays selected", async () => {
		server.use(
			quadletsHandler(() => [web]),
			hostsHandler([source, staging, readOnly, backup]),
			previewHandler({ volumes: [] }),
			moveHandler((id) =>
				quadletMoveResultFixture({
					id,
					name: id,
					status: "partial",
					error: "failed to remove quadlet: resource running",
				}),
			),
		);
		renderQuadletList();
		await screen.findByText(web.name);
		const user = await selectQuadlets(web.name);

		await user.click(screen.getByTestId("move-selected-quadlets-button"));
		await screen.findByTestId("modal");
		await waitFor(() =>
			expect(screen.getByTestId("confirm-multi-transfer-button")).toBeEnabled(),
		);
		await user.click(screen.getByTestId("confirm-multi-transfer-button"));

		await screen.findByTestId("multi-transfer-report");

		// The quadlet still exists on both hosts, so it stays selected.
		await user.click(screen.getByTestId("close-multi-transfer-report-button"));
		await waitFor(() =>
			expect(screen.queryByTestId("modal")).not.toBeInTheDocument(),
		);
		expect(
			screen.getByRole("checkbox", { name: `Select ${web.name}` }),
		).toBeChecked();
	});
});
