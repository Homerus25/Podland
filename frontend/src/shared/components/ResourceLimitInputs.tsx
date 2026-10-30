import {
	FormControl,
	FormErrorMessage,
	FormHelperText,
	FormLabel,
	HStack,
	Input,
} from "@chakra-ui/react";
import { MEMORY_UNITS_PATTERN } from "@shared/utils/memoryUnits";
import type {
	FieldError,
	RegisterOptions,
	UseFormRegisterReturn,
} from "react-hook-form";

// The form-value slice the resource-limit inputs own (spec 412, ticket
// #414): every form rendering these two fields carries them, so the one
// component serving them all cannot drift.
export interface ResourceLimitFormValues {
	cpus: string;
	memory: string;
}

// The register slice the inputs need: the two field names as a closed
// union, so the compiler proves path validity at the prop boundary —
// an adapter's UseFormRegister<TFieldValues> assigns only if "cpus"
// and "memory" are real paths of its form values.
type LimitFieldRegister = (
	name: "cpus" | "memory",
	options?: RegisterOptions<ResourceLimitFormValues, "cpus" | "memory">,
) => UseFormRegisterReturn;

interface ResourceLimitInputsProps {
	// The id prefix the label and input ids derive from:
	// `<prefix>-cpus` and `<prefix>-memory`.
	idPrefix: string;
	// The adapter's register, narrowed to the two fields the inputs
	// render.
	register: LimitFieldRegister;
	// The adapter's formState.errors, narrowed to the two fields the
	// inputs render.
	errors: {
		cpus?: FieldError;
		memory?: FieldError;
	};
}

// The resource-limit inputs (issue #377, promoted to shared by spec 437,
// ticket #431), owned once for every form that renders the two fields:
// each adapter's Resources section renders this component, so labels,
// helper text, and client-side validation are the same everywhere. The
// checks only keep obvious typos from a round trip — the backend's
// units parser stays the validator. The fields stack below the md
// breakpoint and sit side by side from md up (spec 393's mobile
// behavior, owned here). The adapter owns the form and hands over its
// register and errors; the module never calls form hooks.
export function ResourceLimitInputs({
	idPrefix,
	register,
	errors,
}: ResourceLimitInputsProps) {
	return (
		<HStack
			spacing={4}
			align="flex-start"
			flexDirection={{ base: "column", md: "row" }}
		>
			<FormControl isInvalid={!!errors.cpus}>
				<FormLabel htmlFor={`${idPrefix}-cpus`}>CPUs</FormLabel>
				<Input
					id={`${idPrefix}-cpus`}
					type="number"
					step="0.1"
					min="0"
					placeholder="1.5"
					{...register("cpus", {
						validate: (value) =>
							value === "" || Number.parseFloat(value) > 0
								? true
								: "CPU limit must be a positive number",
					})}
				/>
				<FormHelperText>CPU limit in cores.</FormHelperText>
				<FormErrorMessage>{errors.cpus?.message}</FormErrorMessage>
			</FormControl>
			<FormControl isInvalid={!!errors.memory}>
				<FormLabel htmlFor={`${idPrefix}-memory`}>Memory</FormLabel>
				<Input
					id={`${idPrefix}-memory`}
					fontFamily="mono"
					placeholder="512m"
					{...register("memory", {
						validate: (value) =>
							value.trim() === "" || MEMORY_UNITS_PATTERN.test(value.trim())
								? true
								: "Use podman units syntax, e.g. 512m or 1g",
					})}
				/>
				<FormHelperText>Podman units syntax, e.g. 512m or 1g.</FormHelperText>
				<FormErrorMessage>{errors.memory?.message}</FormErrorMessage>
			</FormControl>
		</HStack>
	);
}
