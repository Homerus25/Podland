import {
	Button,
	Collapse,
	FormControl,
	FormErrorMessage,
	FormHelperText,
	FormLabel,
	Input,
	Stack,
	VStack,
} from "@chakra-ui/react";
import type { VolumeCreateInput } from "@features/volumes/services/volumesApi";
import { KeyValueEditor } from "@shared/components/KeyValueEditor";
import {
	FormDialog,
	FormValidityScope,
	type MutationSlice,
} from "@shared/components/MutationDialog";
import { keyValueRowsToRecord } from "@shared/utils/keyValueRows";
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

interface CreateVolumeDialogProps {
	mutation: MutationSlice<VolumeCreateInput>;
	onClose: () => void;
	// Fired once on success, before the module closes: the host toasts the
	// created volume's name from the mutation result here.
	onSuccess?: () => void;
}

// Create-volume dialog: an optional name (empty means podman generates one
// for an anonymous volume) and a labels editor up front; driver and driver
// options sit behind a collapsed "Advanced" section because they are only
// relevant for specialized storage backends. This component is the form
// owner, so it mounts with the dialog (fields start empty on every open)
// and publishes its validity through the scope for the module footer;
// failure renders in the module's error region, success closes through it.
export function CreateVolumeDialog({
	mutation,
	onClose,
	onSuccess,
}: CreateVolumeDialogProps) {
	const [advancedOpen, setAdvancedOpen] = useState(false);
	const { register, control, handleSubmit, formState } =
		useForm<VolumeFormValues>({
			defaultValues: { name: "", labels: [], driver: "", options: [] },
			mode: "onChange",
		});
	const { errors } = formState;
	const labels = useFieldArray({ control, name: "labels" });
	const options = useFieldArray({ control, name: "options" });

	const submit = handleSubmit((values) => {
		mutation.mutate({
			name: values.name.trim(),
			labels: keyValueRowsToRecord(values.labels),
			driver: values.driver.trim() || undefined,
			options: keyValueRowsToRecord(values.options),
		});
	});

	// Thin adapter over the shared key/value editor: the dialog owns the
	// form and the field arrays and hands over their returns; the module
	// renders the rows and owns the per-row key validation (a key is
	// required when the row's value is set).
	const keyValueEditor = (
		name: "labels" | "options",
		rows: UseFieldArrayReturn<VolumeFormValues, "labels" | "options", "id">,
		noun: "label" | "option",
	) => (
		<VStack spacing={2} align="stretch">
			<KeyValueEditor
				name={name}
				rows={rows}
				register={register}
				keyErrorMessage={(index) => errors[name]?.[index]?.key?.message}
				noun={noun}
				addLabel="Add entry"
				keyAriaLabel={(rowNumber) => `${noun} key ${rowNumber}`}
				valueAriaLabel={(rowNumber) => `${noun} value ${rowNumber}`}
				removeAriaLabel={(rowNumber) => `Remove ${noun} entry ${rowNumber}`}
				requireKeyWithValue
			/>
		</VStack>
	);

	return (
		<FormValidityScope formState={formState}>
			<FormDialog
				mutation={mutation}
				title="Create volume"
				onClose={onClose}
				onSuccess={onSuccess}
				formId="create-volume-form"
				submitLabel="Create volume"
				submitTestId="volume-form-submit"
				body={
					<form id="create-volume-form" onSubmit={submit} noValidate>
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
									Leave empty to create an anonymous volume; podman generates
									its name.
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
						</VStack>
					</form>
				}
			/>
		</FormValidityScope>
	);
}
