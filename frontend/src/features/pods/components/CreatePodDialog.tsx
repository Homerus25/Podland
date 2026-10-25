import {
	Divider,
	FormControl,
	FormErrorMessage,
	FormHelperText,
	FormLabel,
	HStack,
	Input,
	Text,
	VStack,
} from "@chakra-ui/react";
import { useNetworks } from "@features/networks/hooks/useNetworks";
import type {
	CreatePodInput,
	PodFormValues,
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
import {
	FormSection,
	MountFlags,
	RowInput,
	RowListEditor,
	RowSelect,
} from "@shared/components/RowEditor";
import { MEMORY_UNITS_PATTERN } from "@shared/utils/memoryUnits";
import { useFieldArray, useForm } from "react-hook-form";

interface CreatePodDialogProps {
	mutation: MutationSlice<CreatePodInput>;
	machineId: string;
	onClose: () => void;
}

// Create-pod dialog: a form dialog (the shared mutation-dialog preset)
// with sections for the name, the networks the pod's infra container
// joins (the shared checkbox list over the machine's networks), named
// volumes and bind mounts (the shared row-editor module — the dialog
// keeps the row shapes, options, and copy), labels (the shared key/value
// editor), and the resource limits. The form-to-payload mapping lives in
// the services module (buildCreatePodInput); the dialog keeps form
// ownership only. The memory field's client-side check (user story 11)
// rides the register validation, so an unparseable value disables the
// submit before a round trip; the backend's units parser stays the
// validator.
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
							<FormSection title="Name">
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
							</FormSection>

							<Divider />

							<FormSection title="Networks">
								<CheckboxList
									options={(networks ?? []).map((network) => ({
										key: network.name,
										label: network.name,
									}))}
									selected={selectedNetworks}
									onSelect={(nextSelection) =>
										setValue("networks", nextSelection)
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
									removeAriaLabel={(rowNumber) =>
										`Remove bind mount ${rowNumber}`
									}
									renderRow={(index) => (
										<>
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
										</>
									)}
									onAdd={() =>
										volumeRows.append({
											name: "",
											destination: "",
											mode: "rw",
										})
									}
								/>
							</FormSection>

							<Divider />

							<FormSection title="Labels">
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
							</FormSection>

							<Divider />

							<FormSection title="Resource limits">
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
													MEMORY_UNITS_PATTERN.test(value.trim())
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
							</FormSection>
						</VStack>
					</form>
				}
			/>
		</FormValidityScope>
	);
}
