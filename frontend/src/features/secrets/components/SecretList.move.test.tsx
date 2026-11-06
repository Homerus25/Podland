import { SecretList } from "@features/secrets/components/SecretList";
import type {
	SecretMovePreview,
	SecretMoveResult,
	SecretSummary,
} from "@features/secrets/services/secretsApi";
import type { Host } from "@shared/types/api";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { beforeEach, describe, expect, it } from "vitest";
import { errorJson } from "@/test/errors";
import {
	hostFixture,
	secretFixture,
	secretMovePreviewFixture,
	secretMoveResultFixture,
} from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

const dbPassword = secretFixture();

const source = hostFixture();
const staging = hostFixture({ id: "m-2", name: "staging-01" });
const backup = hostFixture({ id: "m-4", name: "backup-01" });
const readOnly = hostFixture({
	id: "m-3",
	name: "laptop-01",
	permission: "read",
	isOwner: false,
});

let moveRequests: { count: number; body: Record<string, unknown> } = {
	count: 0,
	body: {},
};
let previewTargets: string[] = [];
let secretFetches = 0;

function resetCounters() {
	moveRequests = { count: 0, body: {} };
	previewTargets = [];
	secretFetches = 0;
}

function secretsHandler(secrets: () => SecretSummary[]) {
	return http.get("*/api/hosts/m-1/secrets", () => {
		secretFetches++;
		return HttpResponse.json(secrets());
	});
}

function hostsHandler(hosts: Host[]) {
	return http.get("*/api/hosts", () => HttpResponse.json(hosts));
}

function previewHandler(overrides: Partial<SecretMovePreview> = {}) {
	return http.get(
		"*/api/hosts/m-1/secrets/db-password/move-preview",
		({ request }) => {
			previewTargets.push(
				new URL(request.url).searchParams.get("targetHostId") ?? "",
			);
			return HttpResponse.json(secretMovePreviewFixture(overrides));
		},
	);
}

// The transfer endpoint answers one plain JSON envelope — no NDJSON, unlike
// the volume pair.
function moveHandler(result: () => SecretMoveResult) {
	return http.post(
		"*/api/hosts/m-1/secrets/db-password/move",
		async ({ request }) => {
			moveRequests.count++;
			moveRequests.body = (await request.json()) as Record<string, unknown>;
			return HttpResponse.json(result());
		},
	);
}

async function openMoveDialog(
	previewOverrides: Partial<SecretMovePreview> = {},
) {
	server.use(
		secretsHandler(() => [dbPassword]),
		hostsHandler([source, staging, readOnly, backup]),
		previewHandler(previewOverrides),
		moveHandler(() => secretMoveResultFixture()),
	);
	renderWithQuery(<SecretList host={source} />, {
		chakra: true,
		// The row navigates on click, so the list renders inside a router.
		router: true,
	});
	const user = userEvent.setup();
	await user.click(
		await screen.findByRole("button", { name: "Move or copy db-password" }),
	);
	expect(await screen.findByTestId("modal")).toBeInTheDocument();
	return user;
}

// Ready-to-confirm state: the preview resolved (the default fixture is
// collision-free), so confirm unlocks once it arrives.
async function openMoveDialogReadyToConfirm(
	previewOverrides: Partial<SecretMovePreview> = {},
) {
	const user = await openMoveDialog(previewOverrides);
	await waitFor(() =>
		expect(screen.getByTestId("confirm-move-button")).toBeEnabled(),
	);
	return user;
}

// The secret transfer slice's dialog tests (issue 239), mirroring the volume
// move suite minus the stream: the endpoint answers one plain JSON
// envelope, so there is no progress line and no in-use/force vocabulary.
describe("SecretList move/copy", () => {
	beforeEach(resetCounters);

	it("hides the Move/Copy action below the reveal-secrets capability (admin)", async () => {
		server.use(
			secretsHandler(() => [dbPassword]),
			hostsHandler([source, staging, readOnly, backup]),
		);
		// Write on the source: the transfer's source gate is admin — the
		// value leaving the host is a reveal — so Move/Copy must not render.
		// Spec 461, ticket #470: the row's delete button is gone too, so a
		// write-level row offers no action at all and the action column
		// disappears with it.
		renderWithQuery(
			<SecretList
				host={hostFixture({ permission: "write", isOwner: false })}
			/>,
			// The row navigates on click, so the list renders inside a router.
			{ chakra: true, router: true },
		);

		const row = await screen.findByTestId("secret-row");
		expect(
			screen.queryByRole("button", { name: "Move or copy db-password" }),
		).not.toBeInTheDocument();
		expect(within(row).queryAllByRole("button")).toHaveLength(0);
	});

	it("offers one dialog listing only manageable hosts, source excluded", async () => {
		await openMoveDialog();

		const select = screen.getByTestId("move-target-select");
		const names = within(select)
			.getAllByRole("option")
			.map((option) => option.textContent);
		expect(names).toEqual(["staging-01", "backup-01"]);
		expect(names).not.toContain("prod-web-01");
		expect(names).not.toContain("laptop-01");
	});

	it("refetches the preview when the target changes", async () => {
		const user = await openMoveDialogReadyToConfirm();

		await user.selectOptions(screen.getByTestId("move-target-select"), "m-4");

		await waitFor(() => expect(previewTargets).toEqual(["m-2", "m-4"]));
	});

	it("sends the move body, and the copy body on a mode switch", async () => {
		const user = await openMoveDialogReadyToConfirm();

		await user.click(screen.getByTestId("confirm-move-button"));
		await waitFor(() => expect(moveRequests.count).toBe(1));
		expect(moveRequests.body).toEqual({
			targetHostId: "m-2",
			mode: "move",
		});

		// A copy keeps the source secret: same endpoint, different mode.
		await user.click(screen.getByTestId("close-move-report-button"));
		await waitFor(() =>
			expect(screen.queryByTestId("modal")).not.toBeInTheDocument(),
		);
		await user.click(
			screen.getByRole("button", { name: "Move or copy db-password" }),
		);
		expect(await screen.findByTestId("modal")).toBeInTheDocument();
		await user.click(
			screen.getByRole("radio", { name: "Copy (keep this host's secret)" }),
		);
		await waitFor(() =>
			expect(screen.getByTestId("confirm-move-button")).toBeEnabled(),
		);
		await user.click(screen.getByTestId("confirm-move-button"));
		await waitFor(() => expect(moveRequests.count).toBe(2));
		expect(moveRequests.body).toEqual({
			targetHostId: "m-2",
			mode: "copy",
		});
	});

	it("disables confirm on a collision with a clear message", async () => {
		await openMoveDialog({ collision: true });

		expect(
			await screen.findByTestId("move-collision-warning"),
		).toHaveTextContent("already exists on staging-01");
		expect(screen.getByTestId("confirm-move-button")).toBeDisabled();
		expect(moveRequests.count).toBe(0);
	});

	it("keeps confirm disabled while the preview is pending", async () => {
		server.use(
			secretsHandler(() => [dbPassword]),
			hostsHandler([source, staging, readOnly, backup]),
			http.get(
				"*/api/hosts/m-1/secrets/db-password/move-preview",
				() => new Promise<Response>(() => {}),
			),
		);
		renderWithQuery(<SecretList host={source} />, {
			chakra: true,
			// The row navigates on click, so the list renders inside a router.
			router: true,
		});
		const user = userEvent.setup();
		await user.click(
			await screen.findByRole("button", { name: "Move or copy db-password" }),
		);

		expect(
			await screen.findByTestId("move-preview-pending"),
		).toBeInTheDocument();
		expect(screen.getByTestId("confirm-move-button")).toBeDisabled();
	});

	it("shows a failed preview and keeps confirm disabled", async () => {
		server.use(
			secretsHandler(() => [dbPassword]),
			hostsHandler([source, staging, readOnly, backup]),
			http.get("*/api/hosts/m-1/secrets/db-password/move-preview", () =>
				HttpResponse.json(
					errorJson("ACCESS_DENIED", "Access denied", "req-71"),
					{ status: 403 },
				),
			),
		);
		renderWithQuery(<SecretList host={source} />, {
			chakra: true,
			// The row navigates on click, so the list renders inside a router.
			router: true,
		});
		const user = userEvent.setup();
		await user.click(
			await screen.findByRole("button", { name: "Move or copy db-password" }),
		);

		expect(await screen.findByText("ACCESS_DENIED")).toBeInTheDocument();
		expect(screen.getByTestId("confirm-move-button")).toBeDisabled();
	});

	it("lists referencing quadlets as a warning, with the dangling note in move mode only", async () => {
		const user = await openMoveDialogReadyToConfirm({
			referencingQuadlets: [
				{ name: "web.container" },
				{ name: "db.container" },
			],
		});

		const warning = screen.getByTestId("move-referencing-warning");
		expect(warning).toHaveTextContent("web.container");
		expect(warning).toHaveTextContent("db.container");
		expect(warning).toHaveTextContent("keep pointing at prod-web-01");
		expect(warning).toHaveTextContent("references dangle");

		await user.click(
			screen.getByRole("radio", { name: "Copy (keep this host's secret)" }),
		);
		expect(
			screen.getByTestId("move-referencing-warning"),
		).not.toHaveTextContent("references dangle");
	});

	it("runs the transfer, narrates the envelope in place, and refetches the source list", async () => {
		server.use(
			secretsHandler(() => [dbPassword]),
			hostsHandler([source, staging, readOnly, backup]),
			previewHandler(),
			moveHandler(() => secretMoveResultFixture()),
		);
		renderWithQuery(<SecretList host={source} />, {
			chakra: true,
			// The row navigates on click, so the list renders inside a router.
			router: true,
		});
		const user = userEvent.setup();
		await user.click(
			await screen.findByRole("button", { name: "Move or copy db-password" }),
		);
		await waitFor(() =>
			expect(screen.getByTestId("confirm-move-button")).toBeEnabled(),
		);
		const before = secretFetches;

		await user.click(screen.getByTestId("confirm-move-button"));

		await waitFor(() => expect(moveRequests.count).toBe(1));
		expect(moveRequests.body).toEqual({
			targetHostId: "m-2",
			mode: "move",
		});

		// The envelope narrated in place instead of closing — the
		// stay-open-report pattern.
		const report = await screen.findByTestId("move-success-report");
		expect(report).toHaveTextContent("Moved");
		expect(report).toHaveTextContent("db-password");
		expect(report).toHaveTextContent("staging-01");
		expect(screen.queryByTestId("confirm-move-button")).not.toBeInTheDocument();
		await waitFor(() => expect(secretFetches).toBeGreaterThan(before));

		await user.click(screen.getByTestId("close-move-report-button"));
		await waitFor(() =>
			expect(screen.queryByTestId("modal")).not.toBeInTheDocument(),
		);
	});

	it("keeps the dialog open on a partial result, naming the both-hosts state", async () => {
		const user = await openMoveDialogReadyToConfirm();
		server.use(
			moveHandler(() =>
				secretMoveResultFixture({
					status: "partial",
					error: "failed to delete secret: secret is in use",
				}),
			),
		);

		await user.click(screen.getByTestId("confirm-move-button"));

		const message = await screen.findByTestId("move-partial-message");
		expect(message).toHaveTextContent("both hosts");
		expect(message).toHaveTextContent("failed to delete secret");
		expect(screen.getByTestId("modal")).toBeInTheDocument();
		expect(screen.queryByTestId("confirm-move-button")).not.toBeInTheDocument();

		await user.click(screen.getByTestId("close-move-report-button"));
		await waitFor(() =>
			expect(screen.queryByTestId("modal")).not.toBeInTheDocument(),
		);
	});

	it("maps a SECRET_ALREADY_EXISTS refusal to the collision hint", async () => {
		const user = await openMoveDialogReadyToConfirm();
		server.use(
			http.post("*/api/hosts/m-1/secrets/db-password/move", () =>
				HttpResponse.json(
					errorJson(
						"SECRET_ALREADY_EXISTS",
						"Secret with this name already exists",
						"req-81",
					),
					{ status: 409 },
				),
			),
		);

		await user.click(screen.getByTestId("confirm-move-button"));

		expect(
			await screen.findByText("SECRET_ALREADY_EXISTS"),
		).toBeInTheDocument();
		const hint = screen.getByTestId("move-collision-hint");
		expect(hint).toHaveTextContent("staging-01");
		expect(hint).toHaveTextContent("remove it there first");
		expect(screen.getByTestId("confirm-move-button")).toBeEnabled();
	});
});
