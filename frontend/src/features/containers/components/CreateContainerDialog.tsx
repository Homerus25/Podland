import { AddIcon, DeleteIcon } from "@chakra-ui/icons";
import {
	Box,
	Button,
	Checkbox,
	Divider,
	FormControl,
	FormErrorMessage,
	FormLabel,
	Heading,
	HStack,
	IconButton,
	Input,
	Select,
	Text,
	VStack,
} from "@chakra-ui/react";
import {
	buildCreateContainerInput,
	type CreateContainerFormValues,
	type CreateContainerInput,
} from "@features/containers/services/containersApi";
import { useNetworks } from "@features/networks/hooks/useNetworks";
import { useSecrets } from "@features/secrets/hooks/useSecrets";
import { useVolumes } from "@features/volumes/hooks/useVolumes";
import { KeyValueEditor } from "@shared/components/KeyValueEditor";
import {
	FormDialog,
	FormValidityScope,
	type MutationSlice,
} from "@shared/components/MutationDialog";
import type { ReactNode } from "react";
import {
	type UseFormRegisterReturn,
	useFieldArray,
	useForm,
} from "react-hook-form";

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

interface CreateContainerDialogProps {
	mutation: MutationSlice<CreateContainerInput>;
	machineId: string;
	onClose: () => void;
}

// One labeled block inside the scrollable form; sections are separated by
// Dividers.
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
	type,
	mono,
	registration,
}: {
	ariaLabel: string;
	placeholder?: string;
	type?: string;
	mono?: boolean;
	registration: UseFormRegisterReturn;
}) {
	return (
		<Input
			type={type}
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

// A multi-select checkbox list over a fetched option list (the Network
// and Secrets sections share this shape). The option's key is the
// selection identity — not its label, so two same-named options cannot
// toggle together. The checkbox is controlled by the watched selection
// and toggling rewrites it.
function CheckboxList({
	options,
	selected,
	onToggle,
}: {
	options: { key: string; label: string }[];
	selected: string[];
	onToggle: (key: string, checked: boolean) => void;
}) {
	return (
		<>
			{options.map((option) => (
				<Checkbox
					key={option.key}
					isChecked={selected.includes(option.key)}
					onChange={(event) => onToggle(option.key, event.target.checked)}
				>
					{option.label}
				</Checkbox>
			))}
		</>
	);
}

// Create-container dialog: 14 fields across 7 sections (Basic, Environment,
// Ports, Storage, Network, Secrets, Devices) in a scrollable form. The dialog
// fetches the machine's networks, volumes, and secrets to populate the
// dropdowns and checkbox lists. This component is the form owner, so it
// mounts with the dialog (fields start empty on every open) and publishes
// its validity through the scope for the module footer; failure renders in
// the module's error region, success closes through it.
export function CreateContainerDialog({
	mutation,
	machineId,
	onClose,
}: CreateContainerDialogProps) {
	const { data: networks } = useNetworks(machineId);
	const { data: volumes } = useVolumes(machineId);
	const { data: secrets } = useSecrets(machineId);

	const { register, control, handleSubmit, watch, setValue, formState } =
		useForm<CreateContainerFormValues>({
			defaultValues: {
				image: "",
				name: "",
				command: "",
				user: "",
				timezone: "",
				restartPolicy: "",
				remove: false,
				environment: [],
				portBindings: [],
				mounts: [],
				volumes: [],
				networks: [],
				secrets: [],
				devices: [],
			},
			// onChange so the footer submit's disabled state tracks validity as
			// the user types, not only after a failed submit attempt.
			mode: "onChange",
		});
	const { errors } = formState;

	const environmentRows = useFieldArray({ control, name: "environment" });
	const portRows = useFieldArray({ control, name: "portBindings" });
	const mountRows = useFieldArray({ control, name: "mounts" });
	const volumeRows = useFieldArray({ control, name: "volumes" });
	const deviceRows = useFieldArray({ control, name: "devices" });

	// The networks/secrets checkbox lists are multi-selects driven by
	// watch + setValue: the checkbox is controlled by the watched array and
	// toggling rewrites it.
	const selectedNetworks = watch("networks");
	const selectedSecrets = watch("secrets");

	// The form-to-payload mapping lives in the services module
	// (buildCreateContainerInput); the dialog keeps form ownership only.
	const submit = handleSubmit((values) => {
		mutation.mutate(buildCreateContainerInput(values, secrets));
	});

	return (
		<FormValidityScope formState={formState}>
			<FormDialog
				mutation={mutation}
				title="Create container"
				onClose={onClose}
				formId="create-container-form"
				submitLabel="Create container"
				submitTestId="container-form-submit"
				body={
					<form id="create-container-form" onSubmit={submit} noValidate>
						<Box maxH="60vh" overflowY="auto" pr={1}>
							<VStack spacing={6} align="stretch">
								<Section title="Basic">
									<FormControl isInvalid={!!errors.image} isRequired>
										<FormLabel htmlFor="container-image">Image</FormLabel>
										<Input
											id="container-image"
											placeholder="docker.io/library/nginx:latest"
											{...register("image", {
												required: "Image is required",
												validate: (value) =>
													value.trim().length > 0 || "Image is required",
											})}
										/>
										<FormErrorMessage>{errors.image?.message}</FormErrorMessage>
									</FormControl>
									<FormControl>
										<FormLabel htmlFor="container-name">
											Name (optional)
										</FormLabel>
										<Input id="container-name" {...register("name")} />
									</FormControl>
									<FormControl>
										<FormLabel htmlFor="container-command">
											Command (optional)
										</FormLabel>
										<Input
											id="container-command"
											placeholder="redis-server --appendonly yes"
											{...register("command")}
										/>
									</FormControl>
									<FormControl>
										<FormLabel htmlFor="container-user">
											User (optional)
										</FormLabel>
										<Input
											id="container-user"
											placeholder="1000:1000"
											{...register("user")}
										/>
									</FormControl>
									<FormControl>
										<FormLabel htmlFor="container-timezone">
											Timezone (optional)
										</FormLabel>
										<Input
											id="container-timezone"
											placeholder="Europe/Berlin"
											{...register("timezone")}
										/>
									</FormControl>
									<FormControl>
										<FormLabel htmlFor="container-restart-policy">
											Restart policy
										</FormLabel>
										<Select
											id="container-restart-policy"
											{...register("restartPolicy")}
										>
											<option value="">(none)</option>
											<option value="no">no</option>
											<option value="always">always</option>
											<option value="unless-stopped">unless-stopped</option>
											<option value="on-failure">on-failure</option>
										</Select>
									</FormControl>
									<FormControl>
										<Checkbox {...register("remove")}>Remove on exit</Checkbox>
									</FormControl>
								</Section>

								<Divider />

								<Section title="Environment">
									<KeyValueEditor
										name="environment"
										rows={environmentRows}
										register={register}
										keyErrorMessage={(index) =>
											errors.environment?.[index]?.key?.message
										}
										noun="env-variable"
										addLabel="Add variable"
										keyAriaLabel={(rowNumber) =>
											`Environment variable ${rowNumber} key`
										}
										valueAriaLabel={(rowNumber) =>
											`Environment variable ${rowNumber} value`
										}
										removeAriaLabel={(rowNumber) =>
											`Remove environment variable ${rowNumber}`
										}
										keyPlaceholder="KEY"
									/>
								</Section>

								<Divider />

								<Section title="Ports">
									{portRows.fields.map((field, index) => (
										<FieldRow
											key={field.id}
											removeLabel={`Remove port binding ${index + 1}`}
											removeTestId={`remove-port-binding-${index}`}
											onRemove={() => portRows.remove(index)}
										>
											<RowInput
												type="number"
												placeholder="Host port"
												ariaLabel={`Host port ${index + 1}`}
												registration={register(
													`portBindings.${index}.hostPort` as const,
												)}
											/>
											<RowInput
												type="number"
												placeholder="Container port"
												ariaLabel={`Container port ${index + 1}`}
												registration={register(
													`portBindings.${index}.containerPort` as const,
												)}
											/>
											<RowSelect
												ariaLabel={`Protocol ${index + 1}`}
												options={[
													{ value: "tcp", label: "tcp" },
													{ value: "udp", label: "udp" },
												]}
												registration={register(
													`portBindings.${index}.protocol` as const,
												)}
											/>
										</FieldRow>
									))}
									<AddRowButton
										label="Add port binding"
										testId="add-port-binding"
										onAdd={() =>
											portRows.append({
												hostPort: "",
												containerPort: "",
												protocol: "tcp",
											})
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
												registration={register(
													`mounts.${index}.source` as const,
												)}
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
												registration={register(
													`volumes.${index}.name` as const,
												)}
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
												registration={register(
													`volumes.${index}.mode` as const,
												)}
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

								<Section title="Network">
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

								<Section title="Secrets">
									<CheckboxList
										options={(secrets ?? []).map((secret) => ({
											key: secret.id,
											label: secret.name,
										}))}
										selected={selectedSecrets}
										onToggle={(name, checked) =>
											setValue(
												"secrets",
												toggleSelection(selectedSecrets, name, checked),
											)
										}
									/>
								</Section>

								<Divider />

								<Section title="Devices">
									{deviceRows.fields.map((field, index) => (
										<FieldRow
											key={field.id}
											removeLabel={`Remove device ${index + 1}`}
											removeTestId={`remove-device-${index}`}
											onRemove={() => deviceRows.remove(index)}
										>
											<RowInput
												mono
												placeholder="Host path"
												ariaLabel={`Device host path ${index + 1}`}
												registration={register(
													`devices.${index}.hostPath` as const,
												)}
											/>
											<RowInput
												mono
												placeholder="Container path"
												ariaLabel={`Device container path ${index + 1}`}
												registration={register(
													`devices.${index}.containerPath` as const,
												)}
											/>
										</FieldRow>
									))}
									<AddRowButton
										label="Add device"
										testId="add-device"
										onAdd={() =>
											deviceRows.append({ hostPath: "", containerPath: "" })
										}
									/>
								</Section>
							</VStack>
						</Box>
					</form>
				}
			/>
		</FormValidityScope>
	);
}
