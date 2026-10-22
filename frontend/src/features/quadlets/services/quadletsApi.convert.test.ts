import {
	convertCompose,
	importQuadlets,
} from "@features/quadlets/services/quadletsApi";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import { errorJson, errorResponse } from "@/test/errors";
import {
	quadletImportEntryFixture,
	quadletImportResponseFixture,
} from "@/test/factories";
import { server } from "@/test/server";

// Service-level tests for the import-compose pair (spec #336): request
// shape and envelope passthrough, like the other quadletsApi suites.
describe("convertCompose", () => {
	it("POSTs the compose content as a JSON body to the global convert endpoint", async () => {
		const captured: {
			path: string;
			body: unknown;
			contentType: string | null;
		} = { path: "", body: null, contentType: null };
		server.use(
			http.post("*/api/quadlets/convert-compose", async ({ request }) => {
				captured.path = new URL(request.url).pathname;
				captured.contentType = request.headers.get("content-type");
				captured.body = await request.json();
				return HttpResponse.json({
					quadlets: [quadletImportEntryFixture()],
				});
			}),
		);

		const compose = "services:\n  web:\n    image: nginx:1.27\n";
		const result = await convertCompose(compose);

		// Global endpoint: the literal path, not a machine-scoped one.
		expect(captured.path).toBe("/api/quadlets/convert-compose");
		// The request is always a JSON body, never multipart.
		expect(captured.contentType).toContain("application/json");
		expect(captured.body).toEqual({ content: compose });
		expect(result).toEqual({ quadlets: [quadletImportEntryFixture()] });
	});

	it("passes a conversion rejection's envelope through (podlet stderr verbatim)", async () => {
		const podletStderr =
			"error converting service `app`\nerror: `image` and `build` cannot both be set";
		server.use(
			http.post("*/api/quadlets/convert-compose", () =>
				errorResponse(400, "VALIDATION_FAILED", podletStderr),
			),
		);

		const failure = await convertCompose("services: {}").catch(
			(error) => error,
		);

		expect(failure.response.status).toBe(400);
		expect(failure.response.data).toEqual(
			errorJson("VALIDATION_FAILED", podletStderr),
		);
	});
});

describe("importQuadlets", () => {
	it("POSTs the entries to the machine-scoped import endpoint and returns the report", async () => {
		const captured: { path: string; body: unknown } = {
			path: "",
			body: null,
		};
		server.use(
			http.post("*/api/machines/m-1/quadlets/import", async ({ request }) => {
				captured.path = new URL(request.url).pathname;
				captured.body = await request.json();
				return HttpResponse.json(quadletImportResponseFixture(), {
					status: 201,
				});
			}),
		);

		const entries = [
			quadletImportEntryFixture(),
			quadletImportEntryFixture({
				name: "dbdata",
				resource: "volume",
				content: "[Volume]\nDriver=local\n",
			}),
		];
		const result = await importQuadlets("m-1", { quadlets: entries });

		expect(captured.path).toBe("/api/machines/m-1/quadlets/import");
		expect(captured.body).toEqual({ quadlets: entries });
		expect(result).toEqual(quadletImportResponseFixture());
	});

	it("passes the imported/failed report through per file", async () => {
		const report = quadletImportResponseFixture({
			imported: [
				{
					id: "web",
					name: "web",
					path: "/etc/containers/systemd/web.container",
					state: "unknown",
				},
			],
			failed: [{ name: "dbdata", error: "permission denied" }],
		});
		server.use(
			http.post("*/api/machines/m-1/quadlets/import", () =>
				HttpResponse.json(report, { status: 201 }),
			),
		);

		const result = await importQuadlets("m-1", {
			quadlets: [quadletImportEntryFixture()],
		});

		expect(result.imported).toEqual(report.imported);
		expect(result.failed).toEqual(report.failed);
	});
});
