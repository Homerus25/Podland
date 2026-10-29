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
	UseFormRegister,
	UseFormRegisterReturn,
} from "react-hook-form";

// The form-value slice the resource limit inputs own (spec 412, ticket
// #414): both the create page's form and the edit-configuration dialog's
// form carry these two fields, so the one component serving both cannot
// drift.
export interface ResourceLimitFormValues {
	cpus: string;
	memory: string;
}

interface ResourceLimitInputsProps<
	TFieldValues extends ResourceLimitFormValues,
> {
	// The adapter's register, for the two inputs.
	register: UseFormRegister<TFieldValues>;
	// The adapter's formState.errors, narrowed to the two fields the
	// inputs render.
	errors: {
		cpus?: FieldError;
		memory?: FieldError;
	};
}

// The container resource-limit inputs (issue #377), owned once for every
// container form: the create page's Resources section and the
// edit-configuration dialog's Resources section render this component, so
// labels, helper text, and client-side validation are the same everywhere.
// The checks only keep obvious typos from a round trip — the backend's
// units parser stays the validator. The adapter owns the form and hands
// over its register and errors; the module never calls form hooks.
export function ResourceLimitInputs<
	TFieldValues extends ResourceLimitFormValues,
>({ register, errors }: ResourceLimitInputsProps<TFieldValues>) {
	// The field names are valid paths of TFieldValues by construction
	// (the prop constraint carries them); the widened register view keeps
	// the calls type-safe without casts at the call sites.
	const registerField = register as (
		path: "cpus" | "memory",
		options?: RegisterOptions<TFieldValues>,
	) => UseFormRegisterReturn;

	return (
		<HStack spacing={4} align="flex-start">
			<FormControl isInvalid={!!errors.cpus}>
				<FormLabel htmlFor="container-cpus">CPUs</FormLabel>
				<Input
					id="container-cpus"
					type="number"
					step="0.1"
					min="0"
					placeholder="1.5"
					{...registerField("cpus", {
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
				<FormLabel htmlFor="container-memory">Memory</FormLabel>
				<Input
					id="container-memory"
					fontFamily="mono"
					placeholder="512m"
					{...registerField("memory", {
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
