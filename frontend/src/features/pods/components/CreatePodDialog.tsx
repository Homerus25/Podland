import {
	Divider,
	FormControl,
	FormErrorMessage,
	FormLabel,
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
import { ResourceLimitInputs } from "@shared/components/ResourceLimitInputs";
import {
	FormSection,
	MountFlags,
	RowInput,
	RowListEditor,
	RowSelect,
} from "@shared/components/RowEditor";
import { useFieldArray, useForm } from "react-hook-form";

interface CreatePodDialogProps {
	mutation: MutationSlice<CreatePodInput>;
	hostId: string;
	onClose: () => void;
}

// Create-pod dialog: a form dialog (the shared mutation-dialog preset)
// with sections for the name, the networks the pod's infra container
// joins (the shared checkbox list over the host's networks), named
// volumes and bind mounts (the shared row-editor module — the dialog
// keeps the row shapes, options, and copy), labels (the shared key/value
// editor), and the resource limits (the shared resource-limit inputs,
// spec 437 ticket #432 — the module owns the fields' labels, helper
// text, and client-side checks, so an unparseable value disables the
// submit before a round trip; the backend's units parser stays the
// validator). The form-to-payload mapping lives in the services module
// (buildCreatePodInput); the dialog keeps form ownership only.
export function CreatePodDialog({
	mutation,
	hostId,
	onClose,
}: CreatePodDialogProps) {
	const { data: networks } = useNetworks(hostId);
	const { data: volumes } = useVolumes(hostId);
	const { register, control, handleSubmit, watch, setValue, formState } =
		useForm<PodFormValues>({
			defaultValues: {
				name: "",
				networks: [],
				volumes: [],
				mounts: [],
				labels: [],
				portMappings: [],
				cpus: "",
				memory: "",
			},
			mode: "onChange",
		});
	const { errors } = formState;
	const volumeRows = useFieldArray({ control, name: "volumes" });
	const mountRows = useFieldArray({ control, name: "mounts" });
	const labelRows = useFieldArray({ control, name: "labels" });
	const portRows = useFieldArray({ control, name: "portMappings" });
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

							<FormSection title="Ports">
								<RowListEditor
									rows={portRows}
									noun="port-mapping"
									addLabel="Add port mapping"
									removeAriaLabel={(rowNumber) =>
										`Remove port mapping ${rowNumber}`
									}
									renderRow={(index) => (
										<>
											<RowInput
												type="number"
												placeholder="Host port"
												ariaLabel={`Host port ${index + 1}`}
												registration={register(
													`portMappings.${index}.hostPort` as const,
												)}
											/>
											<RowInput
												type="number"
												placeholder="Container port"
												ariaLabel={`Container port ${index + 1}`}
												registration={register(
													`portMappings.${index}.containerPort` as const,
												)}
											/>
											<RowSelect
												ariaLabel={`Protocol ${index + 1}`}
												options={[
													{ value: "tcp", label: "tcp" },
													{ value: "udp", label: "udp" },
												]}
												registration={register(
													`portMappings.${index}.protocol` as const,
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
								<ResourceLimitInputs
									idPrefix="pod"
									register={register}
									errors={errors}
								/>
							</FormSection>
						</VStack>
					</form>
				}
			/>
		</FormValidityScope>
	);
}
