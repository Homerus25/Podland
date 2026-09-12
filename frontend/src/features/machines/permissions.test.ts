import {
	canDeleteMachine,
	canEditMachine,
	canManageSecrets,
	canRevealSecrets,
	canShareMachine,
	effectivePermission,
} from "@features/machines/permissions";
import { describe, expect, it } from "vitest";
import { machineFixture } from "@/test/factories";

describe("permissions module", () => {
	it("treats owner as admin via the backend-seeded permission and derives gating consistently", () => {
		const ownerMachine = machineFixture({ permission: "admin" });
		const permission = effectivePermission(ownerMachine);
		expect(permission).toBe("admin");
		expect(canEditMachine(permission)).toBe(true);
		expect(canShareMachine(permission)).toBe(true);
		expect(canDeleteMachine(permission)).toBe(true);

		expect(canEditMachine("read")).toBe(false);
		expect(canEditMachine("write")).toBe(true);
		expect(canShareMachine("write")).toBe(false);
		expect(canDeleteMachine("write")).toBe(false);
	});

	it("gates secret management at write and secret reveal at admin", () => {
		expect(canManageSecrets("read")).toBe(false);
		expect(canManageSecrets("write")).toBe(true);
		expect(canManageSecrets("admin")).toBe(true);

		expect(canRevealSecrets("read")).toBe(false);
		expect(canRevealSecrets("write")).toBe(false);
		expect(canRevealSecrets("admin")).toBe(true);
	});
});
