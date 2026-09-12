import { AddIcon, DeleteIcon } from "@chakra-ui/icons";
import {
	Button,
	Collapse,
	FormControl,
	FormErrorMessage,
	FormHelperText,
	FormLabel,
	HStack,
	IconButton,
	Input,
	Stack,
	VStack,
} from "@chakra-ui/react";
import type { VolumeCreateInput } from "@features/volumes/services/volumesApi";
import { useState } from "react";
import {
	type UseFieldArrayReturn,
	useFieldArray,
	useForm,
} from "react-hook-form";

interface KeyValueEntry {
	key: string;
	value: string;
}

interface VolumeFormValues {
	name: string;
	labels: KeyValueEntry[];
	driver: string;
	options: KeyValueEntry[];
}

interface VolumeFormProps {
	// Fire-and-forget: the host dialog's MutationDialog closes on success and
	// renders the failure, so the form only forwards the payload.
	onSubmit: (input: VolumeCreateInput) => void;
	isSubmitting: boolean;
}

// Maps filled key/value rows to a record, skipping rows with an empty key
// (an empty key carries no metadata, so a half-typed row must not block
// submit). Duplicate keys collapse to the last value, matching podman.
function entriesToRecord(
	rows: KeyValueEntry[],
): Record<string, string> | undefined {
	const filled = rows.filter((row) => row.key.trim() !== "");
	if (filled.length === 0) return undefined;
	return Object.fromEntries(filled.map((row) => [row.key.trim(), row.value]));
}

// Create-volume form: an optional name (empty means podman generates one
// for an anonymous volume) and a labels editor up front; driver and driver
// options sit behind a collapsed "Advanced" section because they are only
// relevant for specialized storage backends.
export function VolumeForm({ onSubmit, isSubmitting }: VolumeFormProps) {
	const [advancedOpen, setAdvancedOpen] = useState(false);
	const {
		register,
		control,
		getValues,
		handleSubmit,
		formState: { errors, isSubmitting: isSubmittingForm, isValid },
	} = useForm<VolumeFormValues>({
		defaultValues: { name: "", labels: [], driver: "", options: [] },
		mode: "onChange",
	});
	const labels = useFieldArray({ control, name: "labels" });
	const options = useFieldArray({ control, name: "options" });

	// A row's key is required only when its value is set: a half-typed row
	// (value, no key yet) should not block submit, but a filled value with
	// no key cannot carry metadata.
	const keyRequired = (name: "labels" | "options", index: number) => {
		const rows = getValues(name);
		return (rows[index]?.value ?? "").trim() !== "";
	};

	const submit = handleSubmit((values) => {
		onSubmit({
			name: values.name.trim(),
			labels: entriesToRecord(values.labels),
			driver: values.driver.trim() || undefined,
			options: entriesToRecord(values.options),
		});
	});

	const keyValueEditor = (
		name: "labels" | "options",
		rows: UseFieldArrayReturn<VolumeFormValues, "labels" | "options", "id">,
		testIdPrefix: string,
	) => (
		<VStack spacing={2} align="stretch">
			{rows.fields.map((field, index) => (
				<HStack key={field.id} spacing={2} align="flex-start">
					<FormControl isInvalid={!!errors[name]?.[index]?.key} flex="1">
						<Input
							fontFamily="mono"
							placeholder="key"
							aria-label={`${testIdPrefix} key ${index + 1}`}
							{...register(`${name}.${index}.key` as const, {
								setValueAs: (value) => value.trim(),
								validate: (value) =>
									value.trim() !== "" ||
									!keyRequired(name, index) ||
									"Key is required when a value is set",
							})}
						/>
						<FormErrorMessage>
							{errors[name]?.[index]?.key?.message}
						</FormErrorMessage>
					</FormControl>
					<FormControl flex="1">
						<Input
							fontFamily="mono"
							placeholder="value"
							aria-label={`${testIdPrefix} value ${index + 1}`}
							{...register(`${name}.${index}.value` as const)}
						/>
					</FormControl>
					<IconButton
						aria-label={`Remove ${testIdPrefix} entry ${index + 1}`}
						icon={<DeleteIcon />}
						size="sm"
						variant="ghost"
						onClick={() => rows.remove(index)}
						data-testid={`remove-${testIdPrefix}-${index}`}
					/>
				</HStack>
			))}
			<Button
				leftIcon={<AddIcon />}
				size="sm"
				variant="ghost"
				onClick={() => rows.append({ key: "", value: "" })}
				data-testid={`add-${testIdPrefix}`}
			>
				Add entry
			</Button>
		</VStack>
	);

	return (
		<form onSubmit={submit} noValidate>
			<VStack spacing={4} align="stretch">
				<FormControl isInvalid={!!errors.name}>
					<FormLabel htmlFor="volume-name">Name (optional)</FormLabel>
					<Input
						id="volume-name"
						fontFamily="mono"
						placeholder="app-data"
						{...register("name", {
							setValueAs: (value) => value.trim(),
						})}
					/>
					<FormHelperText>
						Leave empty to create an anonymous volume; podman generates its
						name.
					</FormHelperText>
					<FormErrorMessage>{errors.name?.message}</FormErrorMessage>
				</FormControl>

				<FormControl>
					<FormLabel>Labels</FormLabel>
					{keyValueEditor("labels", labels, "label")}
				</FormControl>

				<Button
					size="sm"
					variant="link"
					onClick={() => setAdvancedOpen((open) => !open)}
					data-testid="volume-advanced-toggle"
				>
					{advancedOpen ? "Hide advanced" : "Advanced"}
				</Button>
				<Collapse in={advancedOpen}>
					<Stack spacing={4}>
						<FormControl>
							<FormLabel htmlFor="volume-driver">Driver</FormLabel>
							<Input
								id="volume-driver"
								fontFamily="mono"
								placeholder="local"
								{...register("driver")}
							/>
							<FormHelperText>
								Defaults to podman's "local" driver.
							</FormHelperText>
						</FormControl>
						<FormControl>
							<FormLabel>Driver options</FormLabel>
							{keyValueEditor("options", options, "option")}
						</FormControl>
					</Stack>
				</Collapse>

				<Button
					type="submit"
					colorScheme="blue"
					isLoading={isSubmittingForm || isSubmitting}
					isDisabled={!isValid || isSubmittingForm || isSubmitting}
					data-testid="volume-form-submit"
				>
					Create volume
				</Button>
			</VStack>
		</form>
	);
}
