import { useCapabilityGate } from "@features/machines/hooks/useCapabilityGate";
import { waitFor } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import { errorResponse } from "@/test/errors";
import { machineFixture } from "@/test/factories";
import { renderHookWithQuery } from "@/test/render";
import { server } from "@/test/server";

// The gate's classification, pinned once here: pending while the machine
// query is in flight, allowed/denied from the capability table, and a
// failed machine query classified as denied (the error-is-denied policy —
// a page can never drop the error branch the way a hand-rolled copy can).
// Feature suites keep adapter-wiring smokes only; this file owns the
// policy.
describe("useCapabilityGate", () => {
	it("answers pending while the machine query is in flight", async () => {
		let release: () => void = () => {};
		const gate = new Promise<void>((resolve) => {
			release = resolve;
		});
		server.use(
			http.get("*/api/machines/m-1", async () => {
				await gate;
				return HttpResponse.json(machineFixture({ permission: "write" }));
			}),
		);

		const { result } = renderHookWithQuery(() =>
			useCapabilityGate("m-1", "manage-containers"),
		);
		expect(result.current).toBe("pending");

		release();
		await waitFor(() => expect(result.current).toBe("allowed"));
	});

	it("answers allowed or denied from the capability table", async () => {
		const cases: {
			machine: ReturnType<typeof machineFixture>;
			capability: Parameters<typeof useCapabilityGate>[1];
			expected: "allowed" | "denied";
		}[] = [
			{
				machine: machineFixture({ permission: "write", isOwner: false }),
				capability: "manage-containers",
				expected: "allowed",
			},
			{
				machine: machineFixture({ permission: "read", isOwner: false }),
				capability: "manage-containers",
				expected: "denied",
			},
			{
				machine: machineFixture({ permission: "write", isOwner: false }),
				capability: "reveal-secrets",
				expected: "denied",
			},
			{
				machine: machineFixture({ permission: "admin", isOwner: true }),
				capability: "edit-machine",
				expected: "allowed",
			},
		];

		for (const { machine, capability, expected } of cases) {
			server.use(
				http.get("*/api/machines/m-1", () => HttpResponse.json(machine)),
			);
			const { result } = renderHookWithQuery(() =>
				useCapabilityGate("m-1", capability),
			);
			await waitFor(() => expect(result.current).toBe(expected));
		}
	});

	it("answers denied when the machine query fails", async () => {
		server.use(
			http.get("*/api/machines/m-1", () =>
				errorResponse(500, "MACHINE_UNREACHABLE", "Machine unreachable"),
			),
		);

		const { result } = renderHookWithQuery(() =>
			useCapabilityGate("m-1", "manage-containers"),
		);
		await waitFor(() => expect(result.current).toBe("denied"));
	});
});
