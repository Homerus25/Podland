import {
	buildUpdateContainerInput,
	type ContainerConfigFormValues,
	containerConfigPrefill,
} from "@features/containers/services/containersApi";
import { describe, expect, it } from "vitest";
import { containerDetailFixture } from "@/test/factories";

// Unit tests for the edit-configuration payload builder and prefill
// mapper (spec 412, the C3 settlement like containersApi.create.test.ts):
// the dialog's whole form-to-payload mapping contract, pinned here
// without a DOM. The DOM edit-configuration suite keeps the dialog
// wiring; the per-field mapping cases live here.

// The form's baseline: the prefilled state of the shared detail
// fixture's container — a 1.5 cpus / 512m limit and a full CMD-SHELL
// healthcheck.
const baseValues: ContainerConfigFormValues = {
	cpus: "1.5",
	memory: "512m",
	command: "curl -f http://localhost/health || exit 1",
	interval: "30s",
	timeout: "5s",
	startPeriod: "0m30s",
	retries: "3",
	disabled: false,
};

const values = (
	over: Partial<ContainerConfigFormValues>,
): ContainerConfigFormValues => ({ ...baseValues, ...over });

describe("containerConfigPrefill", () => {
	it("maps the detail DTO's limits and healthcheck onto the form values", () => {
		expect(containerConfigPrefill(containerDetailFixture())).toEqual(
			baseValues,
		);
	});

	it("renders unset limits and an absent healthcheck as the empty vocabulary", () => {
		expect(
			containerConfigPrefill(
				containerDetailFixture({ limits: undefined, healthcheck: undefined }),
			),
		).toEqual({
			cpus: "",
			memory: "",
			command: "",
			interval: "",
			timeout: "",
			startPeriod: "",
			retries: "",
			disabled: false,
		});
	});

	it("renders a zero cpus and a disabled healthcheck's empty command as empty strings", () => {
		expect(
			containerConfigPrefill(
				containerDetailFixture({
					limits: { cpus: 0, memory: "" },
					healthcheck: { command: "", disabled: true },
				}),
			),
		).toEqual({
			cpus: "",
			memory: "",
			command: "",
			interval: "",
			timeout: "",
			startPeriod: "",
			retries: "",
			disabled: true,
		});
	});
});

describe("buildUpdateContainerInput", () => {
	it("sends nothing when no field changed", () => {
		expect(buildUpdateContainerInput(values({}), baseValues)).toEqual({});
	});

	it("sends nothing when only whitespace changed", () => {
		expect(
			buildUpdateContainerInput(
				values({
					interval: " 30s ",
					command: " curl -f http://localhost/health || exit 1 ",
				}),
				baseValues,
			),
		).toEqual({});
	});

	it("sends only the changed fields (patch semantics)", () => {
		expect(
			buildUpdateContainerInput(
				values({ cpus: "2", interval: "1m" }),
				baseValues,
			),
		).toEqual({ cpus: 2, healthcheck: { interval: "1m" } });
	});

	it("parses a changed cpus to a float and omits an unchanged or empty one", () => {
		expect(
			buildUpdateContainerInput(values({ cpus: "0.75" }), baseValues).cpus,
		).toBe(0.75);
		expect(
			buildUpdateContainerInput(values({ cpus: "1.5" }), baseValues).cpus,
		).toBeUndefined();
		expect(
			buildUpdateContainerInput(values({ cpus: "" }), baseValues).cpus,
		).toBeUndefined();
	});

	it("sends a changed memory trimmed and omits an unchanged or empty one", () => {
		expect(
			buildUpdateContainerInput(values({ memory: " 1g " }), baseValues).memory,
		).toBe("1g");
		expect(
			buildUpdateContainerInput(values({ memory: "512m" }), baseValues).memory,
		).toBeUndefined();
		expect(
			buildUpdateContainerInput(values({ memory: "" }), baseValues).memory,
		).toBeUndefined();
	});

	it("forwards each changed healthcheck field and leaves the others out", () => {
		expect(
			buildUpdateContainerInput(
				values({ command: "pg_isready -U postgres" }),
				baseValues,
			),
		).toEqual({ healthcheck: { cmd: "pg_isready -U postgres" } });

		expect(
			buildUpdateContainerInput(values({ timeout: "10s" }), baseValues),
		).toEqual({
			healthcheck: { timeout: "10s" },
		});

		expect(
			buildUpdateContainerInput(values({ startPeriod: "1m" }), baseValues),
		).toEqual({
			healthcheck: { startPeriod: "1m" },
		});

		expect(
			buildUpdateContainerInput(values({ retries: "5" }), baseValues),
		).toEqual({
			healthcheck: { retries: 5 },
		});
	});

	it("sends a changed healthcheck field trimmed", () => {
		expect(
			buildUpdateContainerInput(values({ interval: " 1m " }), baseValues),
		).toEqual({
			healthcheck: { interval: "1m" },
		});
	});

	it("carries the disable flag alone, and only when it changed from the prefill", () => {
		expect(
			buildUpdateContainerInput(values({ disabled: true }), baseValues),
		).toEqual({ healthcheck: { disabled: true } });

		// An already-disabled healthcheck re-submitted with the toggle
		// still set changes nothing.
		const disabledPrefill = values({ disabled: true });
		expect(
			buildUpdateContainerInput(values({ disabled: true }), disabledPrefill),
		).toEqual({});
	});

	it("lets changed limits ride a disable request, but no other healthcheck field", () => {
		expect(
			buildUpdateContainerInput(
				values({ disabled: true, cpus: "2", interval: "1m" }),
				baseValues,
			),
		).toEqual({ cpus: 2, healthcheck: { disabled: true } });
	});
});
