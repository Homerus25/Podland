import { MEMORY_UNITS_PATTERN } from "@shared/utils/memoryUnits";
import { describe, expect, it } from "vitest";

// The memory-units pattern's own pins (issue #377): the accepted shapes
// both create surfaces (the create-pod dialog and the create-container
// page) validate their memory fields against. The pattern mirrors
// podman's units parser loosely — the backend's parser stays the
// validator — so these pins cover the client-side shape only.

describe("MEMORY_UNITS_PATTERN", () => {
	it("accepts a bare byte count and counts with unit suffixes", () => {
		for (const value of [
			"512",
			"512m",
			"1g",
			"1.5g",
			"2G",
			"100k",
			"1t",
			"1b",
		]) {
			expect(MEMORY_UNITS_PATTERN.test(value)).toBe(true);
		}
	});

	it("accepts whitespace between the count and the unit", () => {
		expect(MEMORY_UNITS_PATTERN.test("512 m")).toBe(true);
	});

	it("rejects non-numeric values and missing counts", () => {
		for (const value of ["abc", "m", "512x", "-1g", "1.2.3g", ""]) {
			expect(MEMORY_UNITS_PATTERN.test(value)).toBe(false);
		}
	});
});
