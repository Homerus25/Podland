import { type Capability, can } from "@features/machines/permissions";
import type { Machine } from "@shared/types/api";
import { describe, expect, it } from "vitest";
import { machineFixture } from "@/test/factories";

const manageCapabilities: readonly Capability[] = [
	"manage-containers",
	"manage-networks",
	"manage-volumes",
	"manage-images",
	"manage-quadlets",
	"manage-secrets",
];

const revealCapability: Capability = "reveal-secrets";

const ownerCapabilities: readonly Capability[] = [
	"edit-machine",
	"share-machine",
	"delete-machine",
];

function callerWith(facts: Partial<Machine>): Machine {
	return machineFixture(facts);
}

describe("can() capability matrix", () => {
	it("grants level capabilities by rank and owner capabilities only to the owner", () => {
		const read = callerWith({ permission: "read", isOwner: false });
		const write = callerWith({ permission: "write", isOwner: false });
		const admin = callerWith({ permission: "admin", isOwner: false });
		const owner = callerWith({ permission: "admin", isOwner: true });

		for (const capability of manageCapabilities) {
			expect(can(read, capability)).toBe(false);
			expect(can(write, capability)).toBe(true);
			expect(can(admin, capability)).toBe(true);
			expect(can(owner, capability)).toBe(true);
		}
		for (const capability of [revealCapability]) {
			expect(can(read, capability)).toBe(false);
			expect(can(write, capability)).toBe(false);
			expect(can(admin, capability)).toBe(true);
			expect(can(owner, capability)).toBe(true);
		}
		for (const capability of ownerCapabilities) {
			expect(can(read, capability)).toBe(false);
			expect(can(write, capability)).toBe(false);
			expect(can(admin, capability)).toBe(false);
			expect(can(owner, capability)).toBe(true);
		}
	});
});
