import { AddIcon } from "@chakra-ui/icons";
import {
	Button,
	Checkbox,
	Divider,
	FormControl,
	FormErrorMessage,
	FormLabel,
	Heading,
	HStack,
	Input,
	Select,
	Text,
	useToast,
	VStack,
} from "@chakra-ui/react";
import {
	useCreateContainer,
	useStartContainer,
} from "@features/containers/hooks/useContainers";
import { usePullThenRetry } from "@features/containers/hooks/usePullThenRetry";
import {
	buildCreateContainerInput,
	type CreateContainerFormValues,
} from "@features/containers/services/containersApi";
import { PullProgressDisplay } from "@features/images/components/PullProgressDisplay";
import { useNetworks } from "@features/networks/hooks/useNetworks";
import { usePods } from "@features/pods/hooks/usePods";
import { useSecrets } from "@features/secrets/hooks/useSecrets";
import { CreateVolumeDialog } from "@features/volumes/components/CreateVolumeDialog";
import {
	useCreateVolume,
	useVolumes,
} from "@features/volumes/hooks/useVolumes";
import { CheckboxList } from "@shared/components/CheckboxList";
import { ErrorDisplay } from "@shared/components/ErrorDisplay";
import { KeyValueEditor } from "@shared/components/KeyValueEditor";
import {
	FormSection,
	MountFlags,
	RowInput,
	RowListEditor,
	RowSelect,
} from "@shared/components/RowEditor";
import { machineTabPath } from "@shared/routes/detailRoute";
import { getApiError } from "@shared/utils/api";
import { useState } from "react";
import { useFieldArray, useForm } from "react-hook-form";
import { useNavigate, useParams } from "react-router-dom";
import { ResourceLimitInputs } from "./ResourceLimitInputs";

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
	const startContainer = useStartContainer(machineId!);
	const toast = useToast();
	const createVolume = useCreateVolume(machineId!);
	const { data: networks } = useNetworks(machineId!);
	const { data: volumes } = useVolumes(machineId!);
	const { data: secrets } = useSecrets(machineId!);
	// The pod selector's option list (spec 356).
	const { data: pods } = usePods(machineId!);
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
			pod: "",
			environment: [],
			portBindings: [],
			mounts: [],
			volumes: [],
			networks: [],
			secrets: [],
			devices: [],
			privileged: false,
			capAdd: [],
			cpus: "",
			memory: "",
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
	// The pod select drives the Network section's disablement (spec 356).
	const selectedPod = watch("pod");
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

	// The image-not-found pull flow (spec 341 ticket #349, deepened into
	// a hook by spec #371 ticket #370): the submit button — relabeled
	// "Pull image & create container" once the create has failed with
	// IMAGE_NOT_FOUND — pulls the missing image through the images pull
	// stream; a pull that ends without an error re-submits the create
	// automatically with the preserved form values, and a failed pull
	// leaves its error inline. The hook owns the pull-then-retry-once
	// ritual; the page hands it the create re-submission and keeps only
	// the form and the displays below.
	const pull = usePullThenRetry(machineId!, submit);

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
					<FormSection title="Basic">
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
						<FormControl>
							<FormLabel htmlFor="container-pod">Pod</FormLabel>
							<Select
								id="container-pod"
								data-testid="container-pod-select"
								{...register("pod")}
							>
								<option value="">No pod</option>
								{(pods ?? []).map((pod) => (
									<option key={pod.id} value={pod.name}>
										{pod.name}
									</option>
								))}
							</Select>
						</FormControl>
					</FormSection>

					<Divider />

					<FormSection title="Environment">
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
					</FormSection>

					<Divider />

					<FormSection title="Ports">
						<RowListEditor
							rows={portRows}
							noun="port-binding"
							addLabel="Add port binding"
							removeAriaLabel={(rowNumber) =>
								`Remove port binding ${rowNumber}`
							}
							renderRow={(index) => (
								<>
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
								</>
							)}
							onAdd={() =>
								portRows.append({
									hostPort: "",
									containerPort: "",
									protocol: "tcp",
								})
							}
						/>
					</FormSection>

					<Divider />

					<FormSection title="Storage">
						<Text fontWeight="bold" fontSize="sm">
							Bind mounts
						</Text>
						<RowListEditor
							rows={mountRows}
							noun="bind-mount"
							addLabel="Add bind mount"
							removeAriaLabel={(rowNumber) => `Remove bind mount ${rowNumber}`}
							renderRow={(index) => (
								<>
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
										name="mounts"
										index={index}
										row={mountValues?.[index]}
										setValue={setValue}
									/>
								</>
							)}
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
						<RowListEditor
							rows={volumeRows}
							noun="named-volume"
							addLabel="Add volume"
							removeAriaLabel={(rowNumber) =>
								`Remove named volume ${rowNumber}`
							}
							renderRow={(index) => (
								<>
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
								</>
							)}
							onAdd={() =>
								volumeRows.append({
									name: "",
									destination: "",
									mode: "rw",
								})
							}
							// Inline volume create (spec 341, ticket #347):
							// the volume the form needs can be created without
							// leaving it; the dialog is the existing
							// create-volume one.
							addArea={
								<Button
									leftIcon={<AddIcon />}
									size="sm"
									variant="outline"
									onClick={() => setIsCreateVolumeOpen(true)}
									data-testid="create-volume-inline"
								>
									Create volume
								</Button>
							}
						/>
					</FormSection>

					<Divider />

					<FormSection title="Network">
						{selectedPod ? (
							<Text
								fontSize="sm"
								color="gray.500"
								data-testid="container-pod-network-hint"
							>
								The container joins a pod — the pod owns networking, so
								container-level networks are unavailable.
							</Text>
						) : null}
						<CheckboxList
							options={(networks ?? []).map((network) => ({
								key: network.name,
								label: network.name,
							}))}
							selected={selectedNetworks}
							isDisabled={!!selectedPod}
							onSelect={(nextSelection) => setValue("networks", nextSelection)}
						/>
					</FormSection>

					<Divider />

					<FormSection title="Secrets">
						<CheckboxList
							options={(secrets ?? []).map((secret) => ({
								key: secret.id,
								label: secret.name,
							}))}
							selected={selectedSecrets}
							onSelect={(nextSelection) => setValue("secrets", nextSelection)}
						/>
					</FormSection>

					<Divider />

					<FormSection title="Devices">
						<RowListEditor
							rows={deviceRows}
							noun="device"
							addLabel="Add device"
							removeAriaLabel={(rowNumber) => `Remove device ${rowNumber}`}
							renderRow={(index) => (
								<>
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
								</>
							)}
							onAdd={() =>
								deviceRows.append({ hostPath: "", containerPath: "" })
							}
						/>
					</FormSection>

					<Divider />

					<FormSection title="Security">
						<FormControl>
							<Checkbox {...register("privileged")}>Privileged</Checkbox>
						</FormControl>
						<RowListEditor
							rows={capAddRows}
							noun="capability"
							addLabel="Add capability"
							removeAriaLabel={(rowNumber) => `Remove capability ${rowNumber}`}
							renderRow={(index) => (
								<RowInput
									mono
									placeholder="NET_ADMIN"
									ariaLabel={`Capability ${index + 1}`}
									registration={register(`capAdd.${index}.capability` as const)}
								/>
							)}
							onAdd={() => capAddRows.append({ capability: "" })}
						/>
					</FormSection>

					<Divider />

					{/* The Resources section (issue #377): the shared
						resource-limit inputs (spec 412, ticket #414) —
						one owner for the labels, helper text, and
						client-side checks, shared with the
						edit-configuration dialog. */}
					<FormSection title="Resources">
						<ResourceLimitInputs register={register} errors={errors} />
					</FormSection>

					{createError ? (
						<VStack
							spacing={2}
							align="stretch"
							data-testid="container-form-error"
						>
							<ErrorDisplay error={createError} />
							{imageNotFound && pull.isPulling ? (
								// The shared pull-progress presentation (spec
								// #371, ticket #364) under the page's own
								// test id.
								<PullProgressDisplay
									progress={pull.progress}
									data-testid="container-pull-progress"
								/>
							) : null}
							{imageNotFound && pull.error ? (
								<ErrorDisplay
									error={pull.error}
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
						isLoading={createContainer.isPending || pull.isPulling}
						isDisabled={!isValid || isSubmitting || pull.isPulling}
						onClick={
							imageNotFound
								? () => pull.startPull(getValues("image").trim())
								: undefined
						}
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
