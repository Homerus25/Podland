import { SecretList } from "@features/secrets/components/SecretList";
import type {
	SecretMovePreview,
	SecretMoveResult,
	SecretSummary,
} from "@features/secrets/services/secretsApi";
import type { Host } from "@shared/types/api";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http, type JsonBodyType } from "msw";
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

// The Secrets "Move/copy selected" adapter (spec 486, ticket #487) at the
// MSW/HTTP seam. The batch contract itself — one target and mode for the
// whole selection, per-item previews and warnings, sequential execution, the
// per-item report, the succeeded keys — is pinned once in the shared
// MultiTransferDialog suite. This suite keeps adapter smokes only: the
// toolbar button's presence and gate, the row button's removal, the dialog
// opening with the selected secrets, and the selection clearing only the
// successes.

const alpha = secretFixture({ id: "s-1", name: "alpha-secret" });
const beta = secretFixture({ id: "s-2", name: "beta-secret" });

const source = hostFixture();
const staging = hostFixture({ id: "m-2", name: "staging-01" });
const backup = hostFixture({ id: "m-4", name: "backup-01" });
const readOnly = hostFixture({
	id: "m-3",
	name: "laptop-01",
	permission: "read",
	isOwner: false,
});

let moveRequests: { name: string; body: Record<string, unknown> }[] = [];
let previewRequests: string[] = [];

function resetCounters() {
	moveRequests = [];
	previewRequests = [];
}

function hostsHandler(hosts: Host[]) {
	return http.get("*/api/hosts", () => HttpResponse.json(hosts));
}

function secretsHandler(secrets: () => SecretSummary[]) {
	return http.get("*/api/hosts/m-1/secrets", () =>
		HttpResponse.json(secrets()),
	);
}

function previewHandler(overrides: Partial<SecretMovePreview> = {}) {
	return http.get(
		"*/api/hosts/m-1/secrets/:secretName/move-preview",
		({ params }) => {
			previewRequests.push(String(params.secretName));
			return HttpResponse.json(secretMovePreviewFixture(overrides));
		},
	);
}

// The per-item transfer endpoint: one plain JSON envelope each, addressed by
// the secret's name — the existing single-item route, unchanged.
function moveHandler(
	result: (name: string) => SecretMoveResult | HttpResponse<JsonBodyType>,
) {
	return http.post(
		"*/api/hosts/m-1/secrets/:secretName/move",
		async ({ request, params }) => {
			const name = String(params.secretName);
			moveRequests.push({
				name,
				body: (await request.json()) as Record<string, unknown>,
			});
			const answer = result(name);
			return answer instanceof HttpResponse
				? answer
				: HttpResponse.json(answer);
		},
	);
}

function renderSecretList(permission: "read" | "write" | "admin" = "admin") {
	return renderWithQuery(
		<SecretList
			host={hostFixture({ permission, isOwner: permission === "admin" })}
		/>,
		// The row navigates on click, so the list renders inside a router.
		{ chakra: true, router: true },
	);
}

async function selectSecrets(...names: string[]) {
	const user = userEvent.setup();
	for (const name of names) {
		await user.click(screen.getByRole("checkbox", { name: `Select ${name}` }));
	}
	return user;
}

describe("SecretList move/copy selected", () => {
	beforeEach(resetCounters);

	it("renders Move/copy selected permanently for a manage-secrets + reveal-secrets caller, disabled on an empty selection", async () => {
		server.use(
			secretsHandler(() => [alpha, beta]),
			hostsHandler([source, staging, readOnly, backup]),
		);
		renderSecretList();
		await screen.findByText(alpha.name);

		// Permanently rendered — the button disables itself rather than
		// appearing and disappearing with the selection.
		const button = screen.getByTestId("move-selected-secrets-button");
		expect(button).toBeDisabled();

		await selectSecrets(alpha.name);
		expect(screen.getByTestId("move-selected-secrets-button")).toBeEnabled();
	});

	it("renders for a write-level caller too, disabled with the admin reason the backend enforces", async () => {
		server.use(
			secretsHandler(() => [alpha, beta]),
			hostsHandler([source, staging, readOnly, backup]),
		);
		// Write level manages secrets but cannot reveal one, and a transfer
		// reveals the value on the way out — so the button renders (never
		// hidden) and stays disabled, naming the requirement instead of
		// baiting the caller into a 403.
		renderSecretList("write");
		await screen.findByText(alpha.name);

		const button = screen.getByTestId("move-selected-secrets-button");
		expect(button).toBeDisabled();
		expect(button).toHaveAttribute(
			"title",
			"Moving a secret requires admin access",
		);

		// Selecting a secret does not unlock it: the gate is the capability,
		// not the selection.
		await selectSecrets(alpha.name);
		expect(screen.getByTestId("move-selected-secrets-button")).toBeDisabled();
		// The toolbar itself is still there: manage-secrets governs it.
		expect(screen.getByTestId("create-secret-button")).toBeInTheDocument();
	});

	it("renders no row Move/Copy button any more", async () => {
		server.use(
			secretsHandler(() => [alpha, beta]),
			hostsHandler([source, staging, readOnly, backup]),
		);
		renderSecretList();

		const rows = await screen.findAllByTestId("secret-row");
		expect(rows).toHaveLength(2);
		for (const row of rows) {
			expect(
				within(row).queryByRole("button", { name: /move/i }),
			).not.toBeInTheDocument();
			expect(within(row).queryAllByRole("button")).toHaveLength(0);
		}
	});

	it("opens one dialog naming the selected secrets, sending nothing yet", async () => {
		server.use(
			secretsHandler(() => [alpha, beta]),
			hostsHandler([source, staging, readOnly, backup]),
			previewHandler(),
			moveHandler((name) => secretMoveResultFixture({ name })),
		);
		renderSecretList();
		await screen.findByText(alpha.name);
		const user = await selectSecrets(alpha.name, beta.name);

		await user.click(screen.getByTestId("move-selected-secrets-button"));

		const modal = await screen.findByTestId("modal");
		expect(modal).toHaveTextContent("Move or copy 2 secrets");
		const blocks = screen.getAllByTestId("multi-transfer-item");
		expect(blocks).toHaveLength(2);
		expect(within(blocks[0]).getByText(alpha.name)).toBeInTheDocument();
		expect(within(blocks[1]).getByText(beta.name)).toBeInTheDocument();
		// One target and one mode for the whole batch.
		expect(screen.getAllByTestId("move-target-select")).toHaveLength(1);
		expect(screen.getAllByTestId("move-mode-radio")).toHaveLength(1);
		expect(moveRequests).toHaveLength(0);
	});

	it("clears only the successes from the selection after a run", async () => {
		server.use(
			secretsHandler(() => [alpha, beta]),
			hostsHandler([source, staging, readOnly, backup]),
			previewHandler(),
			moveHandler((name) =>
				name === beta.name
					? HttpResponse.json(
							errorJson(
								"SECRET_ALREADY_EXISTS",
								"Secret with this name already exists",
							),
							{ status: 409 },
						)
					: secretMoveResultFixture({ name }),
			),
		);
		renderSecretList();
		await screen.findByText(alpha.name);
		const user = await selectSecrets(alpha.name, beta.name);

		await user.click(screen.getByTestId("move-selected-secrets-button"));
		await screen.findByTestId("modal");
		await waitFor(() =>
			expect(screen.getByTestId("confirm-multi-transfer-button")).toBeEnabled(),
		);

		await user.click(screen.getByTestId("confirm-multi-transfer-button"));
		// The report is the run's completion signal; what it says is the
		// shared dialog's contract, pinned in its own suite.
		await screen.findByTestId("multi-transfer-report");

		// The succeeded secret leaves the selection; the failed one stays
		// selected for the retry.
		await user.click(screen.getByTestId("close-multi-transfer-report-button"));
		await waitFor(() =>
			expect(screen.queryByTestId("modal")).not.toBeInTheDocument(),
		);
		expect(
			screen.getByRole("checkbox", { name: `Select ${alpha.name}` }),
		).not.toBeChecked();
		expect(
			screen.getByRole("checkbox", { name: `Select ${beta.name}` }),
		).toBeChecked();
	});

	it("treats a partial transfer as a failure, so the secret stays selected", async () => {
		server.use(
			secretsHandler(() => [alpha]),
			hostsHandler([source, staging, readOnly, backup]),
			previewHandler(),
			moveHandler((name) =>
				secretMoveResultFixture({
					name,
					status: "partial",
					error: "failed to delete secret: secret is in use",
				}),
			),
		);
		renderSecretList();
		await screen.findByText(alpha.name);
		const user = await selectSecrets(alpha.name);

		await user.click(screen.getByTestId("move-selected-secrets-button"));
		await screen.findByTestId("modal");
		await waitFor(() =>
			expect(screen.getByTestId("confirm-multi-transfer-button")).toBeEnabled(),
		);
		await user.click(screen.getByTestId("confirm-multi-transfer-button"));
		await screen.findByTestId("multi-transfer-report");

		// The value crossed but the source secret stayed, so the item is a
		// failure for the batch and keeps its selection for the retry.
		await user.click(screen.getByTestId("close-multi-transfer-report-button"));
		await waitFor(() =>
			expect(screen.queryByTestId("modal")).not.toBeInTheDocument(),
		);
		expect(
			screen.getByRole("checkbox", { name: `Select ${alpha.name}` }),
		).toBeChecked();
	});

	it("fetches one preview per selected secret for the chosen target", async () => {
		server.use(
			secretsHandler(() => [alpha, beta]),
			hostsHandler([source, staging, readOnly, backup]),
			previewHandler(),
			moveHandler((name) => secretMoveResultFixture({ name })),
		);
		renderSecretList();
		await screen.findByText(alpha.name);
		const user = await selectSecrets(alpha.name, beta.name);

		await user.click(screen.getByTestId("move-selected-secrets-button"));
		await screen.findByTestId("modal");

		await waitFor(() =>
			expect(previewRequests.sort()).toEqual([alpha.name, beta.name]),
		);
	});
});
