import {
	getSecretMovePreview,
	moveSecret,
} from "@features/secrets/services/secretsApi";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import {
	secretMovePreviewFixture,
	secretMoveResultFixture,
} from "@/test/factories";
import { server } from "@/test/server";

// Service-level tests for the secret copy/move pair (issue 239): request
// shape and passthrough, like the other secretsApi suites. The transfer is
// a plain JSON POST — no stream URL to pin, unlike the volume pair.
describe("getSecretMovePreview", () => {
	it("GETs the preview from the source secret with the target as a query parameter", async () => {
		const captured: { path: string; query: URLSearchParams | null } = {
			path: "",
			query: null,
		};
		server.use(
			http.get(
				"*/api/machines/m-1/secrets/db-password/move-preview",
				({ request }) => {
					const url = new URL(request.url);
					captured.path = url.pathname;
					captured.query = url.searchParams;
					return HttpResponse.json(secretMovePreviewFixture());
				},
			),
		);

		const preview = await getSecretMovePreview("m-1", "db-password", "m-2");

		expect(captured.path).toBe(
			"/api/machines/m-1/secrets/db-password/move-preview",
		);
		expect(captured.query?.get("targetMachineId")).toBe("m-2");
		expect(preview).toEqual(secretMovePreviewFixture());
	});

	it("passes the preview facts through untouched", async () => {
		const facts = secretMovePreviewFixture({
			collision: true,
			referencingQuadlets: [
				{ name: "web.container" },
				{ name: "db.container" },
			],
		});
		server.use(
			http.get("*/api/machines/m-1/secrets/db-password/move-preview", () =>
				HttpResponse.json(facts),
			),
		);

		const preview = await getSecretMovePreview("m-1", "db-password", "m-2");

		expect(preview).toEqual(facts);
	});
});

describe("moveSecret", () => {
	it("POSTs the transfer body to the source secret's move endpoint", async () => {
		let body: unknown;
		server.use(
			http.post(
				"*/api/machines/m-1/secrets/db-password/move",
				async ({ request }) => {
					body = await request.json();
					return HttpResponse.json(secretMoveResultFixture());
				},
			),
		);

		const result = await moveSecret("m-1", "db-password", {
			targetMachineId: "m-2",
			mode: "move",
		});

		expect(body).toEqual({ targetMachineId: "m-2", mode: "move" });
		expect(result).toEqual(secretMoveResultFixture());
	});

	it("passes the envelope through untouched, partial detail included", async () => {
		const envelope = secretMoveResultFixture({
			status: "partial",
			error: "failed to delete secret: secret is in use",
		});
		server.use(
			http.post("*/api/machines/m-1/secrets/db-password/move", () =>
				HttpResponse.json(envelope),
			),
		);

		const result = await moveSecret("m-1", "db-password", {
			targetMachineId: "m-2",
			mode: "move",
		});

		expect(result).toEqual(envelope);
	});
});
