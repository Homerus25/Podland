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
	Progress,
	Select,
	Text,
	useToast,
	VStack,
} from "@chakra-ui/react";
import {
	useCreateContainer,
	useStartContainer,
} from "@features/containers/hooks/useContainers";
import {
	buildCreateContainerInput,
	type CreateContainerFormValues,
	type MountRow,
} from "@features/containers/services/containersApi";
import { usePullImage } from "@features/images/hooks/usePullImage";
import { useNetworks } from "@features/networks/hooks/useNetworks";
import { useSecrets } from "@features/secrets/hooks/useSecrets";
import { CreateVolumeDialog } from "@features/volumes/components/CreateVolumeDialog";
import {
	useCreateVolume,
	useVolumes,
} from "@features/volumes/hooks/useVolumes";
import { ErrorDisplay } from "@shared/components/ErrorDisplay";
import { KeyValueEditor } from "@shared/components/KeyValueEditor";
import { machineTabPath } from "@shared/routes/detailRoute";
import { getApiError } from "@shared/utils/api";
import { formatBytes } from "@shared/utils/format";
import type { ReactNode } from "react";
import { useEffect, useRef, useState } from "react";
import {
	type UseFormRegisterReturn,
	type UseFormSetValue,
	useFieldArray,
	useForm,
} from "react-hook-form";
import { useNavigate, useParams } from "react-router-dom";

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

// The z/Z/U flag checkboxes of one bind-mount row (spec 341, ticket
// #348): z (SELinux shared relabel) and Z (SELinux private relabel) are
// mutually exclusive — checking one unchecks the other — while U (chown
// to the container's user) is independent. The checkboxes are
// controlled by the watched row values; toggling rewrites them through
// setValue, the same watch+setValue shape the option checkbox lists
// use. Named-volume rows never render these: the flags are bind-mount
// only.
function MountFlags({
	index,
	row,
	setValue,
}: {
	index: number;
	row: MountRow | undefined;
	setValue: UseFormSetValue<CreateContainerFormValues>;
}) {
	return (
		<HStack spacing={3} pt={2}>
			<Checkbox
				isChecked={row?.selinuxShared ?? false}
				onChange={(event) => {
					const checked = event.target.checked;
					setValue(`mounts.${index}.selinuxShared` as const, checked);
					if (checked) {
						setValue(`mounts.${index}.selinuxPrivate` as const, false);
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
					setValue(`mounts.${index}.selinuxPrivate` as const, checked);
					if (checked) {
						setValue(`mounts.${index}.selinuxShared` as const, false);
					}
				}}
				aria-label={`Bind mount ${index + 1} SELinux private (Z)`}
			>
				Z
			</Checkbox>
			<Checkbox
				isChecked={row?.chownUser ?? false}
				onChange={(event) =>
					setValue(`mounts.${index}.chownUser` as const, event.target.checked)
				}
				aria-label={`Bind mount ${index + 1} chown to user (U)`}
			>
				U
			</Checkbox>
		</HStack>
	);
}

// The create-container page (spec 341, ticket #344): the full-page
// replacement for the former create dialog — 14 fields across 7 sections
// (Basic, Environment, Ports, Storage, Network, Secrets, Devices) plus the
// Security section (privileged + cap-add, spec 341 ticket #345) with
// room to breathe instead of a 60vh scroll window. The page fetches the
// machine's networks, volumes, and secrets to populate the dropdowns and
// checkbox lists. Success and cancel both navigate back to the machine's
// containers tab (?tab=containers); navigating away discards the form
// silently — there is no unsaved-changes guard. The form-to-payload
// mapping lives in the services module (buildCreateContainerInput); the
// page keeps form ownership only.
export function CreateContainerPage() {
	const { machineId } = useParams<{ machineId: string }>();
	const navigate = useNavigate();
	const createContainer = useCreateContainer(machineId!);
	const pullImage = usePullImage(machineId!);
	const startContainer = useStartContainer(machineId!);
	const toast = useToast();
	const createVolume = useCreateVolume(machineId!);
	const { data: networks } = useNetworks(machineId!);
	const { data: volumes } = useVolumes(machineId!);
	const { data: secrets } = useSecrets(machineId!);
	const [isCreateVolumeOpen, setIsCreateVolumeOpen] = useState(false);

	// Run-after-create (spec 341, ticket #346): checked by default, the
	// successful create chains the start endpoint so the container lands
	// running instead of "created". It is page state, not a form value —
	// it never reaches the create payload.
	const [runAfterCreate, setRunAfterCreate] = useState(true);

	const backToContainers = () =>
		navigate(machineTabPath(machineId!, "containers"));

	const {
		register,
		control,
		getValues,
		handleSubmit,
		watch,
		setValue,
		formState,
	} = useForm<CreateContainerFormValues>({
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
			privileged: false,
			capAdd: [],
		},
		// onChange so the submit's disabled state tracks validity as
		// the user types, not only after a failed submit attempt.
		mode: "onChange",
	});
	const { errors, isValid, isSubmitting } = formState;

	// The create failure, as the shared envelope shape — the pull flow
	// keys off its error code.
	const createError = createContainer.error
		? getApiError(createContainer.error)
		: null;
	const imageNotFound = createError?.code === "IMAGE_NOT_FOUND";

	// The pull progress display mirrors PullImageForm's: status text
	// plus a byte progress bar once the stream reports amounts.
	const pullCurrent = pullImage.progress?.state?.current;
	const pullTotal = pullImage.progress?.state?.total;
	const pullPercent =
		pullCurrent !== undefined && pullTotal !== undefined && pullTotal > 0
			? Math.round((pullCurrent / pullTotal) * 100)
			: undefined;

	const environmentRows = useFieldArray({ control, name: "environment" });
	const portRows = useFieldArray({ control, name: "portBindings" });
	const mountRows = useFieldArray({ control, name: "mounts" });
	const volumeRows = useFieldArray({ control, name: "volumes" });
	const deviceRows = useFieldArray({ control, name: "devices" });
	const capAddRows = useFieldArray({ control, name: "capAdd" });

	// The networks/secrets checkbox lists are multi-selects driven by
	// watch + setValue: the checkbox is controlled by the watched array and
	// toggling rewrites it.
	const selectedNetworks = watch("networks");
	const selectedSecrets = watch("secrets");
	// The bind-mount rows' z/Z/U checkboxes read the watched rows so
	// toggling one re-renders the row with the new checked state.
	const mountValues = watch("mounts");

	// The form-to-payload mapping lives in the services module
	// (buildCreateContainerInput); the page keeps form ownership only.
	// Success returns to the containers tab — directly when
	// run-after-create is off, or through the start endpoint when it is
	// on. A start failure after a successful create still returns (the
	// container exists) and toasts what happened; the mutation's list
	// invalidation plus the tab's refetchOnMount refresh the list there.
	const submit = handleSubmit((values) => {
		createContainer.mutate(buildCreateContainerInput(values, secrets), {
			onSuccess: (created) => {
				if (!runAfterCreate) {
					backToContainers();
					return;
				}
				startContainer.mutate(created.id, {
					onError: (error) => {
						toast({
							title: `Container was created but failed to start: ${created.name}`,
							description: getApiError(error).message,
							status: "error",
							isClosable: true,
						});
					},
					onSettled: backToContainers,
				});
			},
		});
	});

	// The image-not-found pull flow (spec 341 ticket #349): the submit
	// button — relabeled "Pull image & create container" once the create has
	// failed with IMAGE_NOT_FOUND — pulls the missing image through the
	// images pull stream; a pull that ends without an error re-submits
	// the create automatically with the preserved form values, and a
	// failed pull leaves its error inline. The form values are never
	// touched either way.
	// A ref, not state: the retry decision must not depend on render
	// timing. submit() re-renders the page (the mutation's pending state),
	// and a state guard would re-fire the effect mid-storm before its
	// own reset lands — re-submitting in a loop. The ref flips
	// synchronously, so the effect fires the retry exactly once.
	const pullRequestedRef = useRef(false);

	const startPull = () => {
		pullRequestedRef.current = true;
		pullImage.pull(getValues("image").trim());
	};

	useEffect(() => {
		if (!pullRequestedRef.current || pullImage.isPulling) return;
		pullRequestedRef.current = false;
		if (pullImage.error) return;
		submit();
	}, [pullImage.isPulling, pullImage.error, submit]);

	return (
		<VStack align="stretch" spacing={4}>
			<HStack spacing={4}>
				<Button
					variant="outline"
					onClick={backToContainers}
					data-testid="container-form-cancel"
				>
					Cancel
				</Button>
				<Heading size="lg">Create container</Heading>
			</HStack>
			<form onSubmit={submit} noValidate>
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
									// An edited image reference invalidates the
									// not-found verdict: the stale create error is
									// cleared so the submit button reverts to a plain
									// "Create container" and the next submit goes
									// straight to the create endpoint instead of
									// forcing a pull of the old reference.
									onChange: () => {
										if (createContainer.error) createContainer.reset();
									},
								})}
							/>
							<FormErrorMessage>{errors.image?.message}</FormErrorMessage>
						</FormControl>
						<FormControl>
							<FormLabel htmlFor="container-name">Name (optional)</FormLabel>
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
							<FormLabel htmlFor="container-user">User (optional)</FormLabel>
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
						<HStack spacing={2}>
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
							{/* Inline volume create (spec 341, ticket #347):
							the volume the form needs can be created without
							leaving it; the dialog is the existing
							create-volume one. */}
							<Button
								leftIcon={<AddIcon />}
								size="sm"
								variant="outline"
								onClick={() => setIsCreateVolumeOpen(true)}
								data-testid="create-volume-inline"
							>
								Create volume
							</Button>
						</HStack>
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
									registration={register(`devices.${index}.hostPath` as const)}
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

					<Divider />

					<Section title="Security">
						<FormControl>
							<Checkbox {...register("privileged")}>Privileged</Checkbox>
						</FormControl>
						{capAddRows.fields.map((field, index) => (
							<FieldRow
								key={field.id}
								removeLabel={`Remove capability ${index + 1}`}
								removeTestId={`remove-capability-${index}`}
								onRemove={() => capAddRows.remove(index)}
							>
								<RowInput
									mono
									placeholder="NET_ADMIN"
									ariaLabel={`Capability ${index + 1}`}
									registration={register(`capAdd.${index}.capability` as const)}
								/>
							</FieldRow>
						))}
						<AddRowButton
							label="Add capability"
							testId="add-capability"
							onAdd={() => capAddRows.append({ capability: "" })}
						/>
					</Section>

					{createError ? (
						<VStack
							spacing={2}
							align="stretch"
							data-testid="container-form-error"
						>
							<ErrorDisplay error={createError} />
							{imageNotFound && pullImage.isPulling ? (
								<Box data-testid="container-pull-progress">
									<Text fontSize="sm">
										{pullImage.progress?.state?.status ?? "Starting pull…"}
									</Text>
									{pullPercent !== undefined ? (
										<>
											<Progress value={pullPercent} size="sm" />
											<Text fontSize="sm" color="gray.500">
												{`${formatBytes(pullCurrent ?? 0)} / ${formatBytes(pullTotal ?? 0)}`}
											</Text>
										</>
									) : null}
								</Box>
							) : null}
							{imageNotFound && pullImage.error ? (
								<ErrorDisplay
									error={pullImage.error}
									data-testid="container-pull-error"
								/>
							) : null}
						</VStack>
					) : null}

					<Checkbox
						isChecked={runAfterCreate}
						onChange={(event) => setRunAfterCreate(event.target.checked)}
						data-testid="container-form-run-after-create"
					>
						Run after create
					</Checkbox>

					{/* The dialog footer's submit policy, on the page: disabled
					while the form is invalid or submitting, or while the
					create request or the image-not-found pull is in flight.
					Once the create has failed with IMAGE_NOT_FOUND the
					button becomes "Pull image & create container": clicking it
					pulls the missing image, and the successful pull
					re-submits the create (spec 341, ticket #349). */}
					<Button
						type={imageNotFound ? "button" : "submit"}
						colorScheme="blue"
						isLoading={createContainer.isPending || pullImage.isPulling}
						isDisabled={!isValid || isSubmitting || pullImage.isPulling}
						onClick={imageNotFound ? startPull : undefined}
						data-testid="container-form-submit"
						alignSelf="flex-start"
					>
						{imageNotFound
							? "Pull image & create container"
							: "Create container"}
					</Button>
				</VStack>
			</form>
			{isCreateVolumeOpen ? (
				<CreateVolumeDialog
					mutation={createVolume}
					onClose={() => setIsCreateVolumeOpen(false)}
					// The created volume lands in the first volume row with
					// an empty dropdown; a row is appended when none exists
					// (or none is empty). The mutation's list invalidation
					// refreshes the page's volumes query, so the dropdown's
					// options carry the new volume by the time it is
					// selected.
					onSuccess={() => {
						const created = createVolume.data;
						if (!created) return;
						const rows = getValues("volumes");
						const targetIndex = rows.findIndex((row) => !row.name);
						if (targetIndex === -1) {
							volumeRows.append({
								name: created.name,
								destination: "",
								mode: "rw",
							});
						} else {
							setValue(`volumes.${targetIndex}.name` as const, created.name, {
								shouldValidate: true,
							});
						}
					}}
				/>
			) : null}
		</VStack>
	);
}
