import { AddIcon, DeleteIcon } from "@chakra-ui/icons";
import {
	Box,
	Button,
	Checkbox,
	Divider,
	FormControl,
	FormErrorMessage,
	FormHelperText,
	FormLabel,
	Heading,
	HStack,
	IconButton,
	Input,
	Select,
	Text,
	VStack,
} from "@chakra-ui/react";
import { useNetworks } from "@features/networks/hooks/useNetworks";
import type {
	CreatePodInput,
	PodFormValues,
	PodMountRow,
} from "@features/pods/services/podsApi";
import { buildCreatePodInput } from "@features/pods/services/podsApi";
import { useVolumes } from "@features/volumes/hooks/useVolumes";
import { CheckboxList } from "@shared/components/CheckboxList";
import { KeyValueEditor } from "@shared/components/KeyValueEditor";
import {
	FormDialog,
	FormValidityScope,
	type MutationSlice,
} from "@shared/components/MutationDialog";
import type { ReactNode } from "react";
import {
	type UseFormRegisterReturn,
	type UseFormSetValue,
	useFieldArray,
	useForm,
} from "react-hook-form";

// The row-editor chrome is the containers create page's, copied
// feature-local (second copy, per the duplication threshold): the pod
// create dialog is a form dialog, not a full page, so the page-level
// Section/Divider composition does not transfer wholesale.

// One labeled block inside the form; sections are separated by Dividers.
function Section({ title, children }: { title: string; children: ReactNode }) {
	return (
		<Box>
			<Heading as="h3" size="sm" mb={3}>
				{title}
			</Heading>
			<VStack spacing={3} align="stretch">
				{children}
			</VStack>
		</Box>
	);
}

// The shared chrome of one field-array row: the row's controls in an
// HStack with the remove button at the end.
function FieldRow({
	removeLabel,
	removeTestId,
	onRemove,
	children,
}: {
	removeLabel: string;
	removeTestId: string;
	onRemove: () => void;
	children: ReactNode;
}) {
	return (
		<HStack spacing={2} align="flex-start">
			{children}
			<IconButton
				aria-label={removeLabel}
				icon={<DeleteIcon />}
				size="sm"
				variant="ghost"
				onClick={onRemove}
				data-testid={removeTestId}
			/>
		</HStack>
	);
}

// The add button under a field-array row list.
function AddRowButton({
	label,
	testId,
	onAdd,
}: {
	label: string;
	testId: string;
	onAdd: () => void;
}) {
	return (
		<Button
			leftIcon={<AddIcon />}
			size="sm"
			variant="ghost"
			onClick={onAdd}
			data-testid={testId}
		>
			{label}
		</Button>
	);
}

// One text input inside a field-array row.
function RowInput({
	ariaLabel,
	placeholder,
	mono,
	registration,
}: {
	ariaLabel: string;
	placeholder?: string;
	mono?: boolean;
	registration: UseFormRegisterReturn;
}) {
	return (
		<Input
			fontFamily={mono ? "mono" : undefined}
			placeholder={placeholder}
			aria-label={ariaLabel}
			{...registration}
		/>
	);
}

// One select inside a field-array row.
function RowSelect({
	ariaLabel,
	options,
	registration,
}: {
	ariaLabel: string;
	options: { value: string; label: string }[];
	registration: UseFormRegisterReturn;
}) {
	return (
		<Select aria-label={ariaLabel} {...registration}>
			{options.map((option) => (
				<option key={option.value} value={option.value}>
					{option.label}
				</option>
			))}
		</Select>
	);
}

// Toggling a checkbox rewrites the watched selection: checked appends
// the option's key, unchecked removes it.
function toggleSelection(
	selected: string[],
	name: string,
	checked: boolean,
): string[] {
	return checked
		? [...selected, name]
		: selected.filter((item) => item !== name);
}

// The z/Z/U flag checkboxes of one bind-mount row — the containers create
// page's MountFlags, copied feature-local (second copy, per the
// duplication threshold). z (SELinux shared relabel) and Z (SELinux
// private relabel) are mutually exclusive — checking one unchecks the
// other — while U (chown to the container's user) is independent. The
// checkboxes are controlled by the watched row values; toggling rewrites
// them through setValue, the same watch+setValue shape the option
// checkbox lists use.
function MountFlags({
	index,
	row,
	setValue,
}: {
	index: number;
	row: PodMountRow | undefined;
	setValue: UseFormSetValue<PodFormValues>;
}) {
	return (
		<HStack spacing={3} pt={2}>
			<Checkbox
				isChecked={row?.selinuxShared ?? false}
				onChange={(event) => {
					const checked = event.target.checked;
					setValue(`mounts.${index}.selinuxShared`, checked);
					if (checked) {
						setValue(`mounts.${index}.selinuxPrivate`, false);
					}
				}}
				aria-label={`Bind mount ${index + 1} SELinux shared (z)`}
			>
				z
			</Checkbox>
			<Checkbox
				isChecked={row?.selinuxPrivate ?? false}
				onChange={(event) => {
					const checked = event.target.checked;
					setValue(`mounts.${index}.selinuxPrivate`, checked);
					if (checked) {
						setValue(`mounts.${index}.selinuxShared`, false);
					}
				}}
				aria-label={`Bind mount ${index + 1} SELinux private (Z)`}
			>
				Z
			</Checkbox>
			<Checkbox
				isChecked={row?.chownUser ?? false}
				onChange={(event) =>
					setValue(`mounts.${index}.chownUser`, event.target.checked)
				}
				aria-label={`Bind mount ${index + 1} chown to user (U)`}
			>
				U
			</Checkbox>
		</HStack>
	);
}

// Podman's units parser accepts a byte count with an optional unit
// suffix; the client-side check mirrors it loosely — the backend's units
// parser stays the validator, this only keeps obvious typos from a round
// trip (user story 11).
const MEMORY_PATTERN = /^\d+(\.\d+)?\s*(b|k|m|g|t|kb|mb|gb|tb)?$/i;

interface CreatePodDialogProps {
	mutation: MutationSlice<CreatePodInput>;
	machineId: string;
	onClose: () => void;
}

// Create-pod dialog: a form dialog (the shared mutation-dialog preset)
// with sections for the name, the networks the pod's infra container
// joins (the shared checkbox list over the machine's networks), named
// volumes and bind mounts (the containers create page's row chrome,
// copied feature-local — second copy, per the duplication threshold),
// labels (the shared key/value editor), and the resource limits. The
// form-to-payload mapping lives in the services module
// (buildCreatePodInput); the dialog keeps form ownership only. The
// memory field's client-side check (user story 11) rides the register
// validation, so an unparseable value disables the submit before a
// round trip; the backend's units parser stays the validator.
export function CreatePodDialog({
	mutation,
	machineId,
	onClose,
}: CreatePodDialogProps) {
	const { data: networks } = useNetworks(machineId);
	const { data: volumes } = useVolumes(machineId);
	const { register, control, handleSubmit, watch, setValue, formState } =
		useForm<PodFormValues>({
			defaultValues: {
				name: "",
				networks: [],
				volumes: [],
				mounts: [],
				labels: [],
				cpus: "",
				memory: "",
			},
			mode: "onChange",
		});
	const { errors } = formState;
	const volumeRows = useFieldArray({ control, name: "volumes" });
	const mountRows = useFieldArray({ control, name: "mounts" });
	const labelRows = useFieldArray({ control, name: "labels" });
	const selectedNetworks = watch("networks");
	const mountValues = watch("mounts");

	const submit = handleSubmit((values) => {
		mutation.mutate(buildCreatePodInput(values));
	});

	return (
		<FormValidityScope formState={formState}>
			<FormDialog
				mutation={mutation}
				title="Create pod"
				onClose={onClose}
				formId="create-pod-form"
				submitLabel="Create pod"
				submitTestId="pod-form-submit"
				body={
					<form id="create-pod-form" onSubmit={submit} noValidate>
						<VStack spacing={4} align="stretch">
							<Section title="Name">
								<FormControl isInvalid={!!errors.name}>
									<FormLabel htmlFor="pod-name">Name</FormLabel>
									<Input
										id="pod-name"
										fontFamily="mono"
										placeholder="app-pod"
										{...register("name", {
											required: "Name is required",
											setValueAs: (value) => value.trim(),
										})}
									/>
									<FormErrorMessage>{errors.name?.message}</FormErrorMessage>
								</FormControl>
							</Section>

							<Divider />

							<Section title="Networks">
								<CheckboxList
									options={(networks ?? []).map((network) => ({
										key: network.name,
										label: network.name,
									}))}
									selected={selectedNetworks}
									onToggle={(name, checked) =>
										setValue(
											"networks",
											toggleSelection(selectedNetworks, name, checked),
										)
									}
								/>
							</Section>

							<Divider />

							<Section title="Storage">
								<Text fontWeight="bold" fontSize="sm">
									Bind mounts
								</Text>
								{mountRows.fields.map((field, index) => (
									<FieldRow
										key={field.id}
										removeLabel={`Remove bind mount ${index + 1}`}
										removeTestId={`remove-bind-mount-${index}`}
										onRemove={() => mountRows.remove(index)}
									>
										<RowInput
											mono
											placeholder="Source"
											ariaLabel={`Mount source ${index + 1}`}
											registration={register(`mounts.${index}.source` as const)}
										/>
										<RowInput
											mono
											placeholder="Destination"
											ariaLabel={`Mount destination ${index + 1}`}
											registration={register(
												`mounts.${index}.destination` as const,
											)}
										/>
										<RowSelect
											ariaLabel={`Mount mode ${index + 1}`}
											options={[
												{ value: "rw", label: "rw" },
												{ value: "ro", label: "ro" },
											]}
											registration={register(`mounts.${index}.mode` as const)}
										/>
										<MountFlags
											index={index}
											row={mountValues?.[index]}
											setValue={setValue}
										/>
									</FieldRow>
								))}
								<AddRowButton
									label="Add bind mount"
									testId="add-bind-mount"
									onAdd={() =>
										mountRows.append({
											source: "",
											destination: "",
											mode: "rw",
											selinuxShared: false,
											selinuxPrivate: false,
											chownUser: false,
										})
									}
								/>
								<Text fontWeight="bold" fontSize="sm" mt={3}>
									Named volumes
								</Text>
								{volumeRows.fields.map((field, index) => (
									<FieldRow
										key={field.id}
										removeLabel={`Remove named volume ${index + 1}`}
										removeTestId={`remove-named-volume-${index}`}
										onRemove={() => volumeRows.remove(index)}
									>
										<RowSelect
											ariaLabel={`Volume name ${index + 1}`}
											options={[
												{ value: "", label: "Select a volume" },
												...(volumes ?? []).map((volume) => ({
													value: volume.name,
													label: volume.name,
												})),
											]}
											registration={register(`volumes.${index}.name` as const)}
										/>
										<RowInput
											mono
											placeholder="Destination"
											ariaLabel={`Volume destination ${index + 1}`}
											registration={register(
												`volumes.${index}.destination` as const,
											)}
										/>
										<RowSelect
											ariaLabel={`Volume mode ${index + 1}`}
											options={[
												{ value: "rw", label: "rw" },
												{ value: "ro", label: "ro" },
											]}
											registration={register(`volumes.${index}.mode` as const)}
										/>
									</FieldRow>
								))}
								<AddRowButton
									label="Add volume"
									testId="add-named-volume"
									onAdd={() =>
										volumeRows.append({
											name: "",
											destination: "",
											mode: "rw",
										})
									}
								/>
							</Section>

							<Divider />

							<Section title="Labels">
								<KeyValueEditor
									name="labels"
									rows={labelRows}
									register={register}
									keyErrorMessage={(index) =>
										errors.labels?.[index]?.key?.message
									}
									noun="label"
									addLabel="Add label"
									keyAriaLabel={(rowNumber) => `Label key ${rowNumber}`}
									valueAriaLabel={(rowNumber) => `Label value ${rowNumber}`}
									removeAriaLabel={(rowNumber) =>
										`Remove label entry ${rowNumber}`
									}
									requireKeyWithValue
								/>
							</Section>

							<Divider />

							<Section title="Resource limits">
								<HStack spacing={4} align="flex-start">
									<FormControl isInvalid={!!errors.cpus}>
										<FormLabel htmlFor="pod-cpus">CPUs</FormLabel>
										<Input
											id="pod-cpus"
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
										<FormLabel htmlFor="pod-memory">Memory</FormLabel>
										<Input
											id="pod-memory"
											fontFamily="mono"
											placeholder="512m"
											{...register("memory", {
												validate: (value) =>
													value.trim() === "" ||
													MEMORY_PATTERN.test(value.trim())
														? true
														: "Use podman units syntax, e.g. 512m or 1g",
											})}
										/>
										<FormHelperText>
											Podman units syntax, e.g. 512m or 1g.
										</FormHelperText>
										<FormErrorMessage>
											{errors.memory?.message}
										</FormErrorMessage>
									</FormControl>
								</HStack>
							</Section>
						</VStack>
					</form>
				}
			/>
		</FormValidityScope>
	);
}
