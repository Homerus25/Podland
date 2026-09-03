import { buildPopulateForm } from "@features/volumes/services/volumesApi";
import { describe, expect, it } from "vitest";

function file(name: string, contents = "x"): File {
	return new File([contents], name, { type: "text/plain" });
}

describe("buildPopulateForm", () => {
	it("appends each plain file as a files part keyed by its relative name", () => {
		const form = buildPopulateForm([file("etc/app.conf"), file("bin/run.sh")]);

		const parts = form.getAll("files");
		expect(parts).toHaveLength(2);
		expect(parts.map((part) => (part as File).name)).toEqual([
			"etc/app.conf",
			"bin/run.sh",
		]);
		expect((parts[0] as File).size).toBe(1);
		expect(form.getAll("archive")).toHaveLength(0);
	});

	it("appends a single tar as the archive part", () => {
		const form = buildPopulateForm([], file("seed.tar", "tar-bytes"));

		const parts = form.getAll("archive");
		expect(parts).toHaveLength(1);
		expect((parts[0] as File).name).toBe("seed.tar");
		expect((parts[0] as File).size).toBe(9);
		expect(form.getAll("files")).toHaveLength(0);
	});

	it("keeps directory-style relative paths when the browser provides them", () => {
		const dirFile = file("app.conf");
		Object.defineProperty(dirFile, "webkitRelativePath", {
			value: "etc/app.conf",
		});
		const form = buildPopulateForm([dirFile]);

		expect((form.getAll("files")[0] as File).name).toBe("etc/app.conf");
	});

	it("prefers the archive shape when a caller somehow passes both", () => {
		const form = buildPopulateForm([file("a.txt")], file("seed.tar"));

		expect(form.getAll("archive")).toHaveLength(1);
		expect(form.getAll("files")).toHaveLength(0);
	});
});
