import {
	buildMoveInput,
	moveQuadlet,
	moveQuadletStreamUrl,
	type QuadletMoveChoices,
	type QuadletMoveFacts,
	type QuadletMoveInput,
	type QuadletMoveMode,
	quadletMovePreview,
} from "@features/quadlets/services/quadletsApi";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import {
	quadletMoveCompanionVolumeFixture,
	quadletMoveDataVolumeFixture,
	quadletMovePreviewFixture,
	quadletMovePreviewSecretFixture,
	quadletMoveResultFixture,
	quadletMoveSecretOutcomeFixture,
} from "@/test/factories";
import { server } from "@/test/server";

// Service-level tests for the move/copy pair (issues 209/210/211): request
// shape and envelope passthrough, like the other quadletsApi suites.
describe("quadletMovePreview", () => {
	it("GETs the preview from the source quadlet with the target as a query parameter", async () => {
		const captured: { path: string; query: URLSearchParams | null } = {
			path: "",
			query: null,
		};
		server.use(
			http.get(
				"*/api/machines/m-1/quadlets/web/move-preview",
				({ request }) => {
					const url = new URL(request.url);
					captured.path = url.pathname;
					captured.query = url.searchParams;
					return HttpResponse.json(quadletMovePreviewFixture());
				},
			),
		);

		const preview = await quadletMovePreview("m-1", "web", "m-2");

		expect(captured.path).toBe("/api/machines/m-1/quadlets/web/move-preview");
		expect(captured.query?.get("targetMachineId")).toBe("m-2");
		expect(preview).toEqual(quadletMovePreviewFixture());
	});

	it("passes companion .volume fields through untouched (issue 217)", async () => {
		const companionPreview = quadletMovePreviewFixture({
			volumes: [quadletMoveCompanionVolumeFixture("differs")],
		});
		server.use(
			http.get("*/api/machines/m-1/quadlets/web/move-preview", () =>
				HttpResponse.json(companionPreview),
			),
		);

		const preview = await quadletMovePreview("m-1", "web", "m-2");

		expect(preview).toEqual(companionPreview);
	});

	it("passes the volume-data fields through untouched (issue 214)", async () => {
		const dataPreview = quadletMovePreviewFixture({
			volumes: [
				quadletMoveDataVolumeFixture({
					sizeBytes: 1572864,
					existsOnTarget: true,
					targetSizeBytes: 2048,
				}),
				quadletMoveDataVolumeFixture({
					source: "ghost",
					podmanVolume: "ghost",
					migratable: false,
					reason: "source volume does not exist",
				}),
			],
		});
		server.use(
			http.get("*/api/machines/m-1/quadlets/web/move-preview", () =>
				HttpResponse.json(dataPreview),
			),
		);

		const preview = await quadletMovePreview("m-1", "web", "m-2");

		expect(preview).toEqual(dataPreview);
	});

	it("passes the secret entries through untouched (issue 215)", async () => {
		const secretPreview = quadletMovePreviewFixture({
			secrets: [
				quadletMovePreviewSecretFixture(),
				quadletMovePreviewSecretFixture({
					name: "tls-cert",
					existsOnTarget: true,
				}),
				quadletMovePreviewSecretFixture({
					name: "ghost",
					existsOnSource: false,
				}),
				quadletMovePreviewSecretFixture({
					name: "/etc/secrets/api.key",
					kind: "hostPath",
					existsOnSource: false,
					existsOnTarget: false,
				}),
			],
		});
		server.use(
			http.get("*/api/machines/m-1/quadlets/web/move-preview", () =>
				HttpResponse.json(secretPreview),
			),
		);

		const preview = await quadletMovePreview("m-1", "web", "m-2");

		expect(preview).toEqual(secretPreview);
	});
});

describe("moveQuadlet", () => {
	it("POSTs the transfer body to the move endpoint and returns the envelope", async () => {
		let body: unknown;
		server.use(
			http.post("*/api/machines/m-1/quadlets/web/move", async ({ request }) => {
				body = await request.json();
				return HttpResponse.json(quadletMoveResultFixture());
			}),
		);

		const result = await moveQuadlet("m-1", "web", {
			targetMachineId: "m-2",
			mode: "move",
			stop: true,
			force: false,
		});

		expect(body).toEqual({
			targetMachineId: "m-2",
			mode: "move",
			stop: true,
			force: false,
		});
		expect(result).toEqual(quadletMoveResultFixture());
	});

	it("sends no stop/force flags for a copy — they are move-mode vocabulary", async () => {
		let body: unknown;
		server.use(
			http.post("*/api/machines/m-1/quadlets/web/move", async ({ request }) => {
				body = await request.json();
				return HttpResponse.json(
					quadletMoveResultFixture({ status: "copied" }),
				);
			}),
		);

		const result = await moveQuadlet("m-1", "web", {
			targetMachineId: "m-2",
			mode: "copy",
		});

		expect(body).toEqual({ targetMachineId: "m-2", mode: "copy" });
		expect(result.status).toBe("copied");
	});

	it("sends transferVolumeQuadlets when the caller sets it (issue 217)", async () => {
		const bodies: unknown[] = [];
		server.use(
			http.post("*/api/machines/m-1/quadlets/web/move", async ({ request }) => {
				bodies.push(await request.json());
				return HttpResponse.json(
					quadletMoveResultFixture({ status: "copied" }),
				);
			}),
		);

		await moveQuadlet("m-1", "web", {
			targetMachineId: "m-2",
			mode: "copy",
			transferVolumeQuadlets: true,
		});
		await moveQuadlet("m-1", "web", {
			targetMachineId: "m-2",
			mode: "move",
			transferVolumeQuadlets: false,
		});

		expect(bodies).toEqual([
			{ targetMachineId: "m-2", mode: "copy", transferVolumeQuadlets: true },
			{ targetMachineId: "m-2", mode: "move", transferVolumeQuadlets: false },
		]);
	});

	it("sends migrateVolumeData and overwriteVolumes when the caller sets them (issue 214)", async () => {
		const bodies: unknown[] = [];
		server.use(
			http.post("*/api/machines/m-1/quadlets/web/move", async ({ request }) => {
				bodies.push(await request.json());
				return HttpResponse.json(
					quadletMoveResultFixture({
						status: "copied",
						volumeMigration: {
							volumes: [
								{
									ref: "pgdata",
									podmanVolume: "pgdata",
									outcome: "migrated",
								},
							],
						},
					}),
				);
			}),
		);

		const result = await moveQuadlet("m-1", "web", {
			targetMachineId: "m-2",
			mode: "copy",
			migrateVolumeData: true,
			overwriteVolumes: true,
		});

		expect(bodies).toEqual([
			{
				targetMachineId: "m-2",
				mode: "copy",
				migrateVolumeData: true,
				overwriteVolumes: true,
			},
		]);
		expect(result.volumeMigration?.volumes).toEqual([
			{ ref: "pgdata", podmanVolume: "pgdata", outcome: "migrated" },
		]);
	});

	it("sends transferSecrets when the caller sets it and passes the secretTransfer block back (issue 215)", async () => {
		const bodies: unknown[] = [];
		server.use(
			http.post("*/api/machines/m-1/quadlets/web/move", async ({ request }) => {
				bodies.push(await request.json());
				return HttpResponse.json(
					quadletMoveResultFixture({
						status: "copied",
						secretTransfer: {
							secrets: [
								quadletMoveSecretOutcomeFixture(),
								quadletMoveSecretOutcomeFixture({
									name: "tls-cert",
									outcome: "skipped",
									reason:
										"a secret with this name already exists on the target",
								}),
							],
						},
					}),
				);
			}),
		);

		const result = await moveQuadlet("m-1", "web", {
			targetMachineId: "m-2",
			mode: "copy",
			transferSecrets: true,
		});

		expect(bodies).toEqual([
			{ targetMachineId: "m-2", mode: "copy", transferSecrets: true },
		]);
		expect(result.secretTransfer?.secrets).toEqual([
			{ name: "db-password", outcome: "transferred" },
			{
				name: "tls-cert",
				outcome: "skipped",
				reason: "a secret with this name already exists on the target",
			},
		]);
	});
});

describe("moveQuadletStreamUrl", () => {
	it("builds the full move-stream URL for the fetch transport (issue 214)", () => {
		expect(moveQuadletStreamUrl("m-1", "web")).toBe(
			"/api/machines/m-1/quadlets/web/move",
		);
	});
});

// The transfer request's guard set, table-driven beside the builder (issue
// 228): the omission discipline the dialog used to carry inline, pinned here
// instead of through the full list→dialog DOM. Rows are preview facts ×
// checkbox choices — the shapes the dialog's fixtures preview, collapsed to
// the booleans the caller computes.
describe("buildMoveInput", () => {
	// Facts of the default preview (quadletMovePreviewFixture in
	// @/test/factories): one migratable podman volume fresh on the target,
	// no companions, no secrets.
	const baseFacts: QuadletMoveFacts = {
		targetMachineId: "m-2",
		companions: false,
		migratable: true,
		anyExistsOnTarget: false,
		secretTransferOffered: false,
	};

	const facts = (over: Partial<QuadletMoveFacts>): QuadletMoveFacts => ({
		...baseFacts,
		...over,
	});

	// The dialog's checkbox defaults (issues 212/217/214/222): stop on,
	// force off, companions on (the opt-out checkbox starts checked),
	// migration, overwrite, and secret transfer off.
	const defaults: QuadletMoveChoices = {
		stop: true,
		force: false,
		transferCompanions: true,
		migrate: false,
		overwrite: false,
		transferSecrets: false,
	};

	const choices = (over: Partial<QuadletMoveChoices>): QuadletMoveChoices => ({
		...defaults,
		...over,
	});

	// The body the default dialog state produces in move mode — every guard
	// row starts from it and expects only its flag deltas.
	const bareMove: QuadletMoveInput = {
		targetMachineId: "m-2",
		mode: "move",
		stop: true,
		force: false,
	};

	it("builds the bare move request from the dialog's default state", () => {
		const { input, stream } = buildMoveInput(baseFacts, "move", defaults);

		expect(stream).toBe(false);
		expect(input).toEqual(bareMove);
	});

	const modeFlagRows: [QuadletMoveMode, QuadletMoveInput][] = [
		[
			"move",
			{ targetMachineId: "m-2", mode: "move", stop: false, force: true },
		],
		["copy", { targetMachineId: "m-2", mode: "copy" }],
	];

	it.each(modeFlagRows)(
		"sends stop/force in %s mode's vocabulary only",
		(mode, expected) => {
			const { input, stream } = buildMoveInput(
				baseFacts,
				mode,
				choices({ stop: false, force: true }),
			);

			expect(stream).toBe(false);
			expect(input).toEqual(expected);
		},
	);

	it.each([
		[
			"is sent false when companions exist and the opt-out is unchecked",
			facts({ companions: true }),
			choices({ transferCompanions: false }),
			{ transferVolumeQuadlets: false },
		],
		[
			"is sent true when companions exist and the checkbox stays checked",
			facts({ companions: true }),
			defaults,
			{ transferVolumeQuadlets: true },
		],
		[
			"is omitted when the preview found no companions, whatever the stale checkbox says",
			baseFacts,
			choices({ transferCompanions: false }),
			{},
		],
	])(
		"transferVolumeQuadlets %s",
		(_name: string, previewFacts: QuadletMoveFacts, modeChoices: QuadletMoveChoices, flags: Partial<QuadletMoveInput>) => {
			const { input, stream } = buildMoveInput(
				previewFacts,
				"move",
				modeChoices,
			);

			expect(stream).toBe(false);
			expect(input).toEqual({ ...bareMove, ...flags });
		},
	);

	it.each([
		[
			"checked over a migratable preview switches to the NDJSON stream",
			choices({ migrate: true }),
			baseFacts,
			{ migrateVolumeData: true },
			true,
		],
		["unchecked keeps the plain request", defaults, baseFacts, {}, false],
		[
			"stale-checked over a preview that no longer offers it stays plain and omitting",
			choices({ migrate: true }),
			facts({ migratable: false }),
			{},
			false,
		],
	])(
		"migrateVolumeData %s",
		(_name: string, modeChoices: QuadletMoveChoices, previewFacts: QuadletMoveFacts, flags: Partial<QuadletMoveInput>, expectedStream: boolean) => {
			const { input, stream } = buildMoveInput(
				previewFacts,
				"move",
				modeChoices,
			);

			expect(stream).toBe(expectedStream);
			expect(input).toEqual({ ...bareMove, ...flags });
		},
	);

	it.each([
		[
			"rides only alongside checked migration onto a target that already holds a volume",
			choices({ migrate: true, overwrite: true }),
			facts({ anyExistsOnTarget: true }),
			{ migrateVolumeData: true, overwriteVolumes: true },
			true,
		],
		[
			"is omitted when the checked migration targets only fresh volumes",
			choices({ migrate: true, overwrite: true }),
			baseFacts,
			{ migrateVolumeData: true },
			true,
		],
		[
			"is omitted when migration is unchecked, whatever the stale overwrite checkbox says",
			choices({ overwrite: true }),
			facts({ anyExistsOnTarget: true }),
			{},
			false,
		],
	])(
		"overwriteVolumes %s",
		(_name: string, modeChoices: QuadletMoveChoices, previewFacts: QuadletMoveFacts, flags: Partial<QuadletMoveInput>, expectedStream: boolean) => {
			const { input, stream } = buildMoveInput(
				previewFacts,
				"move",
				modeChoices,
			);

			expect(stream).toBe(expectedStream);
			expect(input).toEqual({ ...bareMove, ...flags });
		},
	);

	it.each([
		[
			"is sent true when the option is checked and still offered",
			choices({ transferSecrets: true }),
			facts({ secretTransferOffered: true }),
			{ transferSecrets: true },
		],
		[
			"is omitted when the offered option stays unchecked",
			defaults,
			facts({ secretTransferOffered: true }),
			{},
		],
		[
			"is omitted when the preview no longer offers it, whatever the stale checkbox says",
			choices({ transferSecrets: true }),
			baseFacts,
			{},
		],
	])(
		"transferSecrets %s",
		(_name: string, modeChoices: QuadletMoveChoices, previewFacts: QuadletMoveFacts, flags: Partial<QuadletMoveInput>) => {
			const { input, stream } = buildMoveInput(
				previewFacts,
				"move",
				modeChoices,
			);

			expect(stream).toBe(false);
			expect(input).toEqual({ ...bareMove, ...flags });
		},
	);

	it("carries checked secrets on the streaming request when migration also runs", () => {
		const { input, stream } = buildMoveInput(
			facts({ anyExistsOnTarget: true, secretTransferOffered: true }),
			"move",
			choices({ migrate: true, overwrite: true, transferSecrets: true }),
		);

		expect(stream).toBe(true);
		expect(input).toEqual({
			targetMachineId: "m-2",
			mode: "move",
			stop: true,
			force: false,
			transferSecrets: true,
			migrateVolumeData: true,
			overwriteVolumes: true,
		});
	});

	it("switches to the stream for a checked copy migration without the move-only flags", () => {
		const { input, stream } = buildMoveInput(
			baseFacts,
			"copy",
			choices({ migrate: true }),
		);

		expect(stream).toBe(true);
		expect(input).toEqual({
			targetMachineId: "m-2",
			mode: "copy",
			migrateVolumeData: true,
		});
	});
});
