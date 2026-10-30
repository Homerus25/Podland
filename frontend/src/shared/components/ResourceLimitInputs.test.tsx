import type { ResourceLimitFormValues } from "@shared/components/ResourceLimitInputs";
import { ResourceLimitInputs } from "@shared/components/ResourceLimitInputs";
import { render, screen } from "@testing-library/react";
import type { RegisterOptions, UseFormRegister } from "react-hook-form";
import { describe, expect, it, vi } from "vitest";
import { renderWithQuery } from "@/test/render";
import { cssRulesFor } from "@/test/responsiveCss";

// The module never calls form hooks: the stub register below is the
// whole form interface under test. It captures the register options
// each field hands over, so the suite can drive the client-side checks
// exactly like an adapter's react-hook-form register would.
function stubRegister() {
	const options: {
		cpus?: RegisterOptions<ResourceLimitFormValues>;
		memory?: RegisterOptions<ResourceLimitFormValues>;
	} = {};
	const register: UseFormRegister<ResourceLimitFormValues> = (
		name,
		fieldOptions,
	) => {
		options[name] = fieldOptions;
		return {
			name,
			onChange: vi.fn(),
			onBlur: vi.fn(),
			ref: vi.fn(),
		};
	};
	return { register, options };
}

describe("ResourceLimitInputs", () => {
	it("derives the label and input ids from the id prefix", () => {
		const { register } = stubRegister();
		const { unmount } = render(
			<ResourceLimitInputs idPrefix="pod" register={register} errors={{}} />,
		);

		expect(screen.getByLabelText("CPUs")).toHaveAttribute("id", "pod-cpus");
		expect(screen.getByLabelText("Memory")).toHaveAttribute("id", "pod-memory");

		unmount();
		render(
			<ResourceLimitInputs
				idPrefix="container"
				register={register}
				errors={{}}
			/>,
		);
		expect(screen.getByLabelText("CPUs")).toHaveAttribute(
			"id",
			"container-cpus",
		);
		expect(screen.getByLabelText("Memory")).toHaveAttribute(
			"id",
			"container-memory",
		);
	});

	it("renders the labels, helper text, placeholders, and input attributes", () => {
		const { register } = stubRegister();
		render(
			<ResourceLimitInputs idPrefix="widget" register={register} errors={{}} />,
		);

		expect(screen.getByText("CPU limit in cores.")).toBeInTheDocument();
		expect(
			screen.getByText("Podman units syntax, e.g. 512m or 1g."),
		).toBeInTheDocument();

		const cpus = screen.getByLabelText("CPUs");
		expect(cpus).toHaveAttribute("type", "number");
		expect(cpus).toHaveAttribute("step", "0.1");
		expect(cpus).toHaveAttribute("min", "0");
		expect(cpus).toHaveAttribute("placeholder", "1.5");

		const memory = screen.getByLabelText("Memory");
		expect(memory).toHaveAttribute("placeholder", "512m");
		expect(memory).toHaveStyle({ fontFamily: "mono" });
	});

	it("hands both client-side checks to the register options", () => {
		const { register, options } = stubRegister();
		render(
			<ResourceLimitInputs idPrefix="widget" register={register} errors={{}} />,
		);

		// The checks ride the register options — the adapter's form runs
		// them, the module only declares them.
		const cpusCheck = options.cpus?.validate as (
			value: string,
		) => true | string;
		expect(cpusCheck("")).toBe(true);
		expect(cpusCheck("1.5")).toBe(true);
		expect(cpusCheck("0")).toBe("CPU limit must be a positive number");
		expect(cpusCheck("-1")).toBe("CPU limit must be a positive number");

		const memoryCheck = options.memory?.validate as (
			value: string,
		) => true | string;
		expect(memoryCheck("")).toBe(true);
		expect(memoryCheck("   ")).toBe(true);
		expect(memoryCheck("512m")).toBe(true);
		expect(memoryCheck("abc")).toBe("Use podman units syntax, e.g. 512m or 1g");
	});

	it("renders the handed-over errors under the fields", () => {
		const { register } = stubRegister();
		render(
			<ResourceLimitInputs
				idPrefix="widget"
				register={register}
				errors={{
					cpus: {
						type: "validate",
						message: "CPU limit must be a positive number",
					},
					memory: {
						type: "validate",
						message: "Use podman units syntax, e.g. 512m or 1g",
					},
				}}
			/>,
		);

		expect(
			screen.getByText("CPU limit must be a positive number"),
		).toBeInTheDocument();
		expect(
			screen.getByText("Use podman units syntax, e.g. 512m or 1g"),
		).toBeInTheDocument();
		expect(screen.getByLabelText("CPUs")).toHaveAttribute(
			"aria-invalid",
			"true",
		);
		expect(screen.getByLabelText("Memory")).toHaveAttribute(
			"aria-invalid",
			"true",
		);
	});

	it("stacks the fields below the md breakpoint and side by side from md up", () => {
		const { register } = stubRegister();
		// The responsive prop compiles through the theme the Chakra
		// provider installs (the MutationDialog shell precedent); the
		// unused QueryClientProvider is inert.
		renderWithQuery(
			<ResourceLimitInputs idPrefix="widget" register={register} errors={{}} />,
			{ chakra: true },
		);

		// jsdom has no layout engine, so the direction is pinned through
		// the compiled emotion rules (cssRulesFor): the base declarations
		// stack the two fields, Chakra's md breakpoint (768px) restores
		// the side-by-side row.
		const stack = screen
			.getByLabelText("CPUs")
			.closest<HTMLElement>(".chakra-stack");
		expect(stack).not.toBeNull();
		const rules = cssRulesFor(stack as HTMLElement);
		const base = rules.filter((rule) => !rule.startsWith("@media")).join("");
		const md = rules
			.filter((rule) => rule.includes("min-width: 48em"))
			.join("");
		expect(base).toMatch(/flex-direction:\s*column/);
		expect(base).not.toMatch(/flex-direction:\s*row/);
		expect(md).toMatch(/flex-direction:\s*row/);
	});
});
