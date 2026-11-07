import { SecretDetailPage } from "@features/secrets/components/SecretDetailPage";
import type {
	SecretMovePreview,
	SecretMoveResult,
} from "@features/secrets/services/secretsApi";
import type { Host } from "@shared/types/api";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { Route, Routes } from "react-router-dom";
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

// The transfer the page sends: the same endpoint and body the list row's
// dialog sends, addressed by the secret's name.
interface MoveRequests {
	count: number;
	body: Record<string, unknown>;
}

let moveRequests: MoveRequests;
let previewTargets: string[];
let detailFetches: number;

function resetCounters() {
	moveRequests = { count: 0, body: {} };
	previewTargets = [];
	detailFetches = 0;
}

const source = hostFixture();
const staging = hostFixture({ id: "m-2", name: "staging-01" });
const backup = hostFixture({ id: "m-4", name: "backup-01" });
const readOnly = hostFixture({
	id: "m-3",
	name: "laptop-01",
	permission: "read",
	isOwner: false,
});

function installHandlers(
	options: {
		permission?: "read" | "write" | "admin";
		preview?: Partial<SecretMovePreview>;
		moveResult?: () => SecretMoveResult;
	} = {},
) {
	const permission = options.permission ?? "admin";
	server.use(
		http.get("*/api/hosts/m-1", () =>
			HttpResponse.json(
				hostFixture({ permission, isOwner: permission === "admin" }),
			),
		),
		// The transfer dialog's target select reads the hosts list.
		http.get("*/api/hosts", () =>
			HttpResponse.json([source, staging, readOnly, backup] as Host[]),
		),
		http.get("*/api/hosts/m-1/secrets/db-password", () => {
			detailFetches++;
			return HttpResponse.json(secretFixture());
		}),
		http.get(
			"*/api/hosts/m-1/secrets/db-password/move-preview",
			({ request }) => {
				previewTargets.push(
					new URL(request.url).searchParams.get("targetHostId") ?? "",
				);
				return HttpResponse.json(
					secretMovePreviewFixture(options.preview ?? {}),
				);
			},
		),
		http.post(
			"*/api/hosts/m-1/secrets/db-password/move",
			async ({ request }) => {
				moveRequests.count++;
				moveRequests.body = (await request.json()) as Record<string, unknown>;
				return HttpResponse.json(
					(options.moveResult ?? secretMoveResultFixture)(),
				);
			},
		),
	);
}

function renderDetailPage() {
	return renderWithQuery(
		<Routes>
			<Route
				path="/hosts/:hostId/secrets/:secretId"
				element={<SecretDetailPage />}
			/>
		</Routes>,
		{
			chakra: true,
			router: { initialEntries: ["/hosts/m-1/secrets/db-password"] },
		},
	);
}

// The secret detail page's Move/Copy adapter (spec 477, ticket #483): the
// row's one action, on the page. The dialog, the mutation, and the gating
// are the list's — this suite keeps the adapter wiring only (the button
// opens the shared dialog fixed on this secret, the request matches the
// row's, the detail refetches) plus the gate smokes, which never re-pin the
// classification policy.
describe("SecretDetailPage move/copy", () => {
	beforeEach(resetCounters);

	it("opens the shared transfer dialog fixed on this secret for an admin caller", async () => {
		installHandlers();
		renderDetailPage();
		const user = userEvent.setup();

		await user.click(await screen.findByTestId("move-secret-button"));

		const modal = await screen.findByTestId("modal");
		expect(modal).toHaveTextContent("db-password");
		// The preview is addressed by the secret's name: the dialog is fixed
		// on the page's secret, not on a picked one.
		await waitFor(() => expect(previewTargets).toEqual(["m-2"]));
		// The page hands the dialog the real host, so the target list is the
		// capability-filtered one with the source excluded.
		const names = within(screen.getByTestId("move-target-select"))
			.getAllByRole("option")
			.map((option) => option.textContent);
		expect(names).toEqual(["staging-01", "backup-01"]);
	});

	it("sends the row's transfer request and refetches the detail", async () => {
		installHandlers();
		renderDetailPage();
		const user = userEvent.setup();

		await user.click(await screen.findByTestId("move-secret-button"));
		await waitFor(() =>
			expect(screen.getByTestId("confirm-move-button")).toBeEnabled(),
		);
		const before = detailFetches;

		await user.click(screen.getByTestId("confirm-move-button"));

		await waitFor(() => expect(moveRequests.count).toBe(1));
		expect(moveRequests.body).toEqual({
			targetHostId: "m-2",
			mode: "move",
		});
		// The transfer's both-hosts invalidation covers the detail key (it
		// extends the secrets list key), so the page refetches on its own.
		await waitFor(() => expect(detailFetches).toBeGreaterThan(before));
	});

	it("renders the admin hint instead of Move/Copy for a read-only caller", async () => {
		installHandlers({ permission: "read" });
		renderDetailPage();

		const hint = await screen.findByTestId("secret-move-permission-hint");
		expect(hint).toHaveAttribute("data-requirement", "admin");
		expect(screen.queryByTestId("move-secret-button")).not.toBeInTheDocument();
	});

	it("keeps Move/Copy behind the admin gate while Remove stays on write", async () => {
		// The transfer's source gate is admin — the value leaving the host is
		// a reveal — so a write-level caller keeps Remove and loses Move/Copy.
		installHandlers({ permission: "write" });
		renderDetailPage();

		expect(
			await screen.findByTestId("remove-secret-button"),
		).toBeInTheDocument();
		expect(screen.queryByTestId("move-secret-button")).not.toBeInTheDocument();
		expect(
			await screen.findByTestId("secret-move-permission-hint"),
		).toHaveAttribute("data-requirement", "admin");
	});

	it("renders neither Move/Copy nor its hint while the host query is pending", async () => {
		installHandlers();
		let release: () => void = () => {};
		const gate = new Promise<void>((resolve) => {
			release = resolve;
		});
		server.use(
			http.get("*/api/hosts/m-1", async () => {
				await gate;
				return HttpResponse.json(hostFixture());
			}),
		);
		renderDetailPage();

		await screen.findByTestId("secret-detail-name");
		expect(screen.queryByTestId("move-secret-button")).not.toBeInTheDocument();
		expect(
			screen.queryByTestId("secret-move-permission-hint"),
		).not.toBeInTheDocument();

		release();
		await screen.findByTestId("move-secret-button");
	});

	it("adds Move/Copy without duplicating the reveal or remove actions", async () => {
		installHandlers();
		renderDetailPage();

		await screen.findByTestId("move-secret-button");
		expect(screen.getAllByTestId("move-secret-button")).toHaveLength(1);
		expect(screen.getAllByTestId("remove-secret-button")).toHaveLength(1);
		expect(screen.getAllByTestId("reveal-secret-button")).toHaveLength(1);
	});
});

// The single-item dialog's secret domain (issue 239), relocated here when
// spec 486, ticket #487 retired the list row's Move/Copy button: the detail
// page is now the only surface that opens MoveSecretDialog, so its domain
// assertions — the preview states, the referencing warning, the envelope
// narration, the partial both-hosts state, and the SECRET_ALREADY_EXISTS
// hint — live with the surface that reaches it. The skeleton itself stays
// pinned once in the shared TransferDialog suite.
describe("SecretDetailPage move/copy dialog domain", () => {
	beforeEach(resetCounters);

	async function openDialog() {
		renderDetailPage();
		const user = userEvent.setup();
		await user.click(await screen.findByTestId("move-secret-button"));
		await screen.findByTestId("modal");
		return user;
	}

	async function openDialogReadyToConfirm() {
		const user = await openDialog();
		await waitFor(() =>
			expect(screen.getByTestId("confirm-move-button")).toBeEnabled(),
		);
		return user;
	}

	it("refetches the preview when the target changes", async () => {
		installHandlers();
		const user = await openDialogReadyToConfirm();

		await user.selectOptions(screen.getByTestId("move-target-select"), "m-4");

		await waitFor(() => expect(previewTargets).toEqual(["m-2", "m-4"]));
	});

	it("sends the copy body on a mode switch", async () => {
		installHandlers();
		const user = await openDialogReadyToConfirm();

		await user.click(
			screen.getByRole("radio", { name: "Copy (keep this host's secret)" }),
		);
		await user.click(screen.getByTestId("confirm-move-button"));

		await waitFor(() => expect(moveRequests.count).toBe(1));
		expect(moveRequests.body).toEqual({
			targetHostId: "m-2",
			mode: "copy",
		});
	});

	it("disables confirm on a collision with a clear message", async () => {
		installHandlers({ preview: { collision: true } });
		await openDialog();

		expect(
			await screen.findByTestId("move-collision-warning"),
		).toHaveTextContent("already exists on staging-01");
		expect(screen.getByTestId("confirm-move-button")).toBeDisabled();
		expect(moveRequests.count).toBe(0);
	});

	it("keeps confirm disabled while the preview is pending", async () => {
		installHandlers();
		server.use(
			http.get(
				"*/api/hosts/m-1/secrets/db-password/move-preview",
				() => new Promise<Response>(() => {}),
			),
		);
		await openDialog();

		expect(
			await screen.findByTestId("move-preview-pending"),
		).toBeInTheDocument();
		expect(screen.getByTestId("confirm-move-button")).toBeDisabled();
	});

	it("shows a failed preview and keeps confirm disabled", async () => {
		installHandlers();
		server.use(
			http.get("*/api/hosts/m-1/secrets/db-password/move-preview", () =>
				HttpResponse.json(
					errorJson("ACCESS_DENIED", "Access denied", "req-71"),
					{ status: 403 },
				),
			),
		);
		await openDialog();

		expect(await screen.findByText("ACCESS_DENIED")).toBeInTheDocument();
		expect(screen.getByTestId("confirm-move-button")).toBeDisabled();
	});

	it("lists referencing quadlets as a warning, with the dangling note in move mode only", async () => {
		installHandlers({
			preview: {
				referencingQuadlets: [
					{ name: "web.container" },
					{ name: "db.container" },
				],
			},
		});
		const user = await openDialogReadyToConfirm();

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

	it("narrates the envelope in place instead of closing", async () => {
		installHandlers();
		const user = await openDialogReadyToConfirm();

		await user.click(screen.getByTestId("confirm-move-button"));

		const report = await screen.findByTestId("move-success-report");
		expect(report).toHaveTextContent("Moved");
		expect(report).toHaveTextContent("db-password");
		expect(report).toHaveTextContent("staging-01");
		expect(screen.queryByTestId("confirm-move-button")).not.toBeInTheDocument();

		await user.click(screen.getByTestId("close-move-report-button"));
		await waitFor(() =>
			expect(screen.queryByTestId("modal")).not.toBeInTheDocument(),
		);
	});

	it("keeps the dialog open on a partial result, naming the both-hosts state", async () => {
		installHandlers({
			moveResult: () =>
				secretMoveResultFixture({
					status: "partial",
					error: "failed to delete secret: secret is in use",
				}),
		});
		const user = await openDialogReadyToConfirm();

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
		installHandlers();
		const user = await openDialogReadyToConfirm();
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
