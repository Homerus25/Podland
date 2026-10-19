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
import type {
	ContainerRestartPolicy,
	CreateContainerInput,
	MountMode,
	PortProtocol,
} from "@features/containers/services/containersApi";
import { useNetworks } from "@features/networks/hooks/useNetworks";
import { useSecrets } from "@features/secrets/hooks/useSecrets";
import { useVolumes } from "@features/volumes/hooks/useVolumes";
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

interface EnvRow {
	key: string;
	value: string;
}

interface PortRow {
	hostPort: string;
	containerPort: string;
	protocol: PortProtocol;
}

interface MountRow {
	source: string;
	destination: string;
	mode: MountMode;
}

interface VolumeRow {
	name: string;
	destination: string;
	mode: MountMode;
}

interface DeviceRow {
	hostPath: string;
	containerPath: string;
}

interface ContainerFormValues {
	image: string;
	name: string;
	command: string;
	user: string;
	timezone: string;
	// "" is the "(none)" select option; a chosen policy is one of the
	// backend's oneof values.
	restartPolicy: ContainerRestartPolicy | "";
	remove: boolean;
	environment: EnvRow[];
	portBindings: PortRow[];
	mounts: MountRow[];
	volumes: VolumeRow[];
	// The checkbox lists select by option key: the network's name and the
	// secret's id. The submit handler resolves keys back to payload names.
	networks: string[];
	secrets: string[];
	devices: DeviceRow[];
}

// The backend takes the command as a string array; the form offers one text
// input and splits it on whitespace ("redis-server --appendonly yes").
function parseCommand(text: string): string[] | undefined {
	const trimmed = text.trim();
	return trimmed ? trimmed.split(/\s+/) : undefined;
}

// Maps filled key/value rows to a record, skipping rows with an empty key
// (an empty key carries no variable, so a half-typed row must not block
// submit). Returns undefined when no row is filled, so the payload omits
// the field entirely.
function envToRecord(rows: EnvRow[]): Record<string, string> | undefined {
	const filled = rows.filter((row) => row.key.trim() !== "");
	if (filled.length === 0) return undefined;
	return Object.fromEntries(filled.map((row) => [row.key.trim(), row.value]));
}

// A port row is complete only when both ports parse to integers; empty or
// non-numeric rows are filtered out of the payload.
function parsePort(text: string): number | undefined {
	const trimmed = text.trim();
	if (trimmed === "") return undefined;
	const port = Number(trimmed);
	return Number.isInteger(port) ? port : undefined;
}

function portBindingsToInput(
	rows: PortRow[],
): CreateContainerInput["portBindings"] {
	const bindings = rows.flatMap((row) => {
		const hostPort = parsePort(row.hostPort);
		const containerPort = parsePort(row.containerPort);
		if (hostPort === undefined || containerPort === undefined) return [];
		return [{ hostPort, containerPort, protocol: row.protocol || "tcp" }];
	});
	return bindings.length > 0 ? bindings : undefined;
}

// A mount/volume row is complete only when its primary field (the mount
// source or the volume name) and its destination are filled; empty rows
// are filtered out of the payload and the access mode defaults to rw.
// The entry shape rides through buildEntry, so bind mounts and named
// volumes share this one mapping.
function storageRowsToInput<
	Row extends { destination: string; mode: MountMode },
	Entry,
>(
	rows: Row[],
	primaryOf: (row: Row) => string,
	buildEntry: (primary: string, destination: string, mode: MountMode) => Entry,
): Entry[] | undefined {
	const entries = rows.flatMap((row) => {
		const primary = primaryOf(row).trim();
		const destination = row.destination.trim();
		if (!primary || !destination) return [];
		return [buildEntry(primary, destination, row.mode || "rw")];
	});
	return entries.length > 0 ? entries : undefined;
}

// A device row is complete only when both paths are filled; empty rows
// are filtered out of the payload.
function devicesToInput(rows: DeviceRow[]): CreateContainerInput["devices"] {
	const devices = rows.flatMap((row) => {
		const hostPath = row.hostPath.trim();
		const containerPath = row.containerPath.trim();
		if (!hostPath || !containerPath) return [];
		return [{ hostPath, containerPath }];
	});
	return devices.length > 0 ? devices : undefined;
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
		useForm<ContainerFormValues>({
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

	const submit = handleSubmit((values) => {
		const command = parseCommand(values.command);
		const environment = envToRecord(values.environment);
		const portBindings = portBindingsToInput(values.portBindings);
		const mounts = storageRowsToInput(
			values.mounts,
			(row) => row.source,
			(source, destination, mode) => ({ source, destination, mode }),
		);
		const volumes = storageRowsToInput(
			values.volumes,
			(row) => row.name,
			(name, destination, mode) => ({ name, destination, mode }),
		);
		const devices = devicesToInput(values.devices);
		// Network keys are the network names, so the selection is the
		// payload already.
		const networks = values.networks.length > 0 ? values.networks : undefined;
		// Secret keys are the secret ids; the payload carries the name. A
		// secret mounts at /run/secrets/<name> unless a target is given;
		// the form always sends the default target. An id whose secret no
		// longer exists in the fetched list carries nothing and is skipped.
		const secretNameById = new Map(
			(secrets ?? []).map((secret) => [secret.id, secret.name]),
		);
		const secretEntries = values.secrets.flatMap((id) => {
			const name = secretNameById.get(id);
			return name ? [{ source: name, target: `/run/secrets/${name}` }] : [];
		});
		const secretPayload = secretEntries.length > 0 ? secretEntries : undefined;
		const user = values.user.trim();
		const timezone = values.timezone.trim();

		mutation.mutate({
			image: values.image.trim(),
			...(values.name.trim() ? { name: values.name.trim() } : {}),
			...(command ? { command } : {}),
			...(user ? { user } : {}),
			...(timezone ? { timezone } : {}),
			...(values.restartPolicy ? { restartPolicy: values.restartPolicy } : {}),
			...(values.remove ? { remove: true } : {}),
			...(environment ? { environment } : {}),
			...(portBindings ? { portBindings } : {}),
			...(mounts ? { mounts } : {}),
			...(volumes ? { volumes } : {}),
			...(networks ? { networks } : {}),
			...(secretPayload ? { secrets: secretPayload } : {}),
			...(devices ? { devices } : {}),
		});
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
									{environmentRows.fields.map((field, index) => (
										<FieldRow
											key={field.id}
											removeLabel={`Remove environment variable ${index + 1}`}
											removeTestId={`remove-env-variable-${index}`}
											onRemove={() => environmentRows.remove(index)}
										>
											<RowInput
												mono
												placeholder="KEY"
												ariaLabel={`Environment variable ${index + 1} key`}
												registration={register(
													`environment.${index}.key` as const,
												)}
											/>
											<RowInput
												mono
												placeholder="value"
												ariaLabel={`Environment variable ${index + 1} value`}
												registration={register(
													`environment.${index}.value` as const,
												)}
											/>
										</FieldRow>
									))}
									<AddRowButton
										label="Add variable"
										testId="add-env-variable"
										onAdd={() => environmentRows.append({ key: "", value: "" })}
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
