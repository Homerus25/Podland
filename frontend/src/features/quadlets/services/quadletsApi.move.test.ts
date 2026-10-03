import {
	moveQuadlet,
	moveQuadletStreamUrl,
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
