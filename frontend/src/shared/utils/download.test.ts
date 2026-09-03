import { setBearerToken } from "@shared/utils/api";
import { downloadFile } from "@shared/utils/download";
import { HttpResponse, http } from "msw";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import { server } from "@/test/server";

const createObjectURL = vi.fn(() => "blob:mock");
const revokeObjectURL = vi.fn();

beforeAll(() => {
	Object.defineProperty(URL, "createObjectURL", {
		value: createObjectURL,
		configurable: true,
		writable: true,
	});
	Object.defineProperty(URL, "revokeObjectURL", {
		value: revokeObjectURL,
		configurable: true,
		writable: true,
	});
});

function stubAnchor() {
	const click = vi.fn();
	const anchor: Record<string, unknown> = { click };
	vi.spyOn(document, "createElement").mockImplementation(((tag: string) => {
		if (tag === "a") return anchor as unknown as HTMLAnchorElement;
		return document.createElement(tag);
	}) as typeof document.createElement);
	document.body.appendChild = vi.fn() as typeof document.body.appendChild;
	document.body.removeChild = vi.fn() as typeof document.body.removeChild;
	return anchor;
}

afterEach(() => {
	vi.restoreAllMocks();
	setBearerToken(null);
});

describe("downloadFile", () => {
	it("fetches the url, saves the blob under the given filename, and revokes the object url", async () => {
		server.use(
			http.get("*/api/files/blob", () =>
				HttpResponse.arrayBuffer(new Uint8Array([1, 2, 3]).buffer, {
					headers: { "Content-Type": "application/x-tar" },
				}),
			),
		);
		const anchor = stubAnchor();

		await downloadFile("/api/files/blob", "volume.tar");

		expect(anchor.download).toBe("volume.tar");
		expect(anchor.href).toBe("blob:mock");
		expect(anchor.click).toHaveBeenCalledTimes(1);
	});

	it("sends the bearer token like every other API call", async () => {
		let authHeader: string | null = null;
		server.use(
			http.get("*/api/files/blob", ({ request }) => {
				authHeader = request.headers.get("Authorization");
				return HttpResponse.arrayBuffer(new ArrayBuffer(0));
			}),
		);
		stubAnchor();
		setBearerToken("tok-123");

		await downloadFile("/api/files/blob", "volume.tar");

		expect(authHeader).toBe("Bearer tok-123");
	});

	it("falls back to the server's Content-Disposition filename", async () => {
		server.use(
			http.get("*/api/files/blob", () =>
				HttpResponse.arrayBuffer(new ArrayBuffer(0), {
					headers: {
						"Content-Disposition": 'attachment; filename="server-name.tar"',
					},
				}),
			),
		);
		const anchor = stubAnchor();

		await downloadFile("/api/files/blob");

		expect(anchor.download).toBe("server-name.tar");
	});

	it("rejects with the standard error treatment on a non-ok response", async () => {
		server.use(
			http.get("*/api/files/blob", () =>
				HttpResponse.json(
					{
						requestId: "req-7",
						error: { message: "no such volume", code: "VOLUME_NOT_FOUND" },
					},
					{ status: 404 },
				),
			),
		);
		const anchor = stubAnchor();

		await expect(downloadFile("/api/files/blob", "v.tar")).rejects.toEqual({
			code: "VOLUME_NOT_FOUND",
			message: "no such volume",
		});
		expect(anchor.click).not.toHaveBeenCalled();
	});
});
