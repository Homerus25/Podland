import {
	Checkbox,
	Divider,
	FormControl,
	FormErrorMessage,
	FormHelperText,
	FormLabel,
	Input,
	VStack,
} from "@chakra-ui/react";
import {
	buildUpdateContainerInput,
	type ContainerConfigFormValues,
	type ContainerDetail,
	containerConfigPrefill,
	type UpdateContainerInput,
} from "@features/containers/services/containersApi";
import {
	FormDialog,
	FormValidityScope,
	type MutationSlice,
} from "@shared/components/MutationDialog";
import { ResourceLimitInputs } from "@shared/components/ResourceLimitInputs";
import { FormSection } from "@shared/components/RowEditor";
import { useForm } from "react-hook-form";

// A Go duration string as podman accepts it: one or more unit-suffixed
// numbers ("30s", "1m30s", "0m30s"). No leading "-": a negative
// interval is no healthcheck configuration. The check only keeps
// obvious non-durations from a round trip — the backend's duration
// parse stays the validator.
const DURATION_PATTERN = /^(\d+(\.\d+)?(ns|us|µs|ms|s|m|h))+$/;

// A pattern-matched duration is positive when any component is
// non-zero: "0m30s" is positive, "0s" and "0m0s" are not — a zero
// interval would disable the probe timer, never the field's intent.
const isPositiveDuration = (value: string) =>
	[...value.matchAll(/\d+(\.\d+)?/g)].some((match) => Number(match[0]) > 0);

const durationRule = {
	validate: (value: string) => {
		const trimmed = value.trim();
		if (trimmed === "") return true;
		if (!DURATION_PATTERN.test(trimmed)) {
			return "Use a Go duration, e.g. 30s or 1m30s";
		}
		return isPositiveDuration(trimmed) || "Duration must be positive";
	},
};

const retriesRule = {
	validate: (value: string) =>
		value.trim() === "" || /^\d+$/.test(value.trim())
			? true
			: "Retries must be a non-negative integer",
};

interface EditConfigurationDialogProps {
	// The container's current detail: the form's prefill and the
	// changed-fields guard's reference.
	container: ContainerDetail;
	mutation: MutationSlice<UpdateContainerInput>;
	onClose: () => void;
}

// Edit-configuration dialog (spec 412, ticket #414): the container's
// resource limits and healthcheck, prefilled from the detail DTO's
// current values. The Resources section is the shared resource-limit
// inputs — the same component the create page renders, so the two cannot
// drift. Submitting sends only the changed fields (empty = unchanged);
// the disable toggle is exclusive — when set, it greys out the other
// healthcheck inputs and the request carries the disable flag alone.
// This component is the form owner, so it mounts with the dialog (the
// fields start at the current configuration on every open) and publishes
// its validity through the scope for the module footer; failure renders
// in the module's error region, success closes through it.
export function EditConfigurationDialog({
	container,
	mutation,
	onClose,
}: EditConfigurationDialogProps) {
	const prefill = containerConfigPrefill(container);
	const { register, handleSubmit, formState, watch } =
		useForm<ContainerConfigFormValues>({
			defaultValues: prefill,
			// onChange so the footer submit's disabled state tracks
			// validity as the user types, not only after a failed submit
			// attempt.
			mode: "onChange",
		});
	const { errors } = formState;
	const healthcheckDisabled = watch("disabled");

	const submit = handleSubmit((values) => {
		mutation.mutate(buildUpdateContainerInput(values, prefill));
	});

	return (
		<FormValidityScope formState={formState}>
			<FormDialog
				mutation={mutation}
				title="Edit configuration"
				onClose={onClose}
				formId="edit-configuration-form"
				submitLabel="Save changes"
				submitTestId="edit-configuration-submit"
				body={
					<form id="edit-configuration-form" onSubmit={submit} noValidate>
						<VStack spacing={4} align="stretch">
							<FormSection title="Resources">
								<ResourceLimitInputs
									idPrefix="container"
									register={register}
									errors={errors}
								/>
							</FormSection>
							<Divider />
							<FormSection title="Healthcheck">
								<Checkbox {...register("disabled")}>
									Disable healthcheck
								</Checkbox>
								{/* The toggle is exclusive: while set, the other
								healthcheck inputs grey out, so no other
								healthcheck field can ride the request. */}
								<FormControl isDisabled={healthcheckDisabled}>
									<FormLabel htmlFor="healthcheck-command">Command</FormLabel>
									<Input
										id="healthcheck-command"
										fontFamily="mono"
										placeholder="curl -f http://localhost/health || exit 1"
										{...register("command")}
									/>
									<FormHelperText>
										Podman CLI syntax: a bare command, or CMD/CMD-SHELL
										prefixed.
									</FormHelperText>
								</FormControl>
								<FormControl
									isInvalid={!!errors.interval}
									isDisabled={healthcheckDisabled}
								>
									<FormLabel htmlFor="healthcheck-interval">Interval</FormLabel>
									<Input
										id="healthcheck-interval"
										fontFamily="mono"
										placeholder="30s"
										{...register("interval", durationRule)}
									/>
									<FormErrorMessage>
										{errors.interval?.message}
									</FormErrorMessage>
								</FormControl>
								<FormControl
									isInvalid={!!errors.timeout}
									isDisabled={healthcheckDisabled}
								>
									<FormLabel htmlFor="healthcheck-timeout">Timeout</FormLabel>
									<Input
										id="healthcheck-timeout"
										fontFamily="mono"
										placeholder="5s"
										{...register("timeout", durationRule)}
									/>
									<FormErrorMessage>{errors.timeout?.message}</FormErrorMessage>
								</FormControl>
								<FormControl
									isInvalid={!!errors.startPeriod}
									isDisabled={healthcheckDisabled}
								>
									<FormLabel htmlFor="healthcheck-start-period">
										Start period
									</FormLabel>
									<Input
										id="healthcheck-start-period"
										fontFamily="mono"
										placeholder="0m30s"
										{...register("startPeriod", durationRule)}
									/>
									<FormErrorMessage>
										{errors.startPeriod?.message}
									</FormErrorMessage>
								</FormControl>
								<FormControl
									isInvalid={!!errors.retries}
									isDisabled={healthcheckDisabled}
								>
									<FormLabel htmlFor="healthcheck-retries">Retries</FormLabel>
									<Input
										id="healthcheck-retries"
										type="number"
										min="0"
										placeholder="3"
										{...register("retries", retriesRule)}
									/>
									<FormHelperText>
										Retries before the container is marked unhealthy.
									</FormHelperText>
									<FormErrorMessage>{errors.retries?.message}</FormErrorMessage>
								</FormControl>
							</FormSection>
						</VStack>
					</form>
				}
			/>
		</FormValidityScope>
	);
}
