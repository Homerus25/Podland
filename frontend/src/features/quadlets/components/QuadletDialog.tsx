import {
	FormControl,
	FormErrorMessage,
	FormLabel,
	Input,
	Select,
	Text,
	Textarea,
	VStack,
} from "@chakra-ui/react";
import { useQuadlet } from "@features/quadlets/hooks/useQuadlets";
import type { QuadletSummary } from "@features/quadlets/services/quadletsApi";
import {
	type CreateQuadletInput,
	QUADLET_RESOURCES,
	type UpdateQuadletInput,
} from "@features/quadlets/services/quadletsApi";
import { ErrorDisplay } from "@shared/components/ErrorDisplay";
import {
	FormDialog,
	FormValidityScope,
	type MutationSlice,
} from "@shared/components/MutationDialog";
import { getApiError } from "@shared/utils/api";
import { useEffect } from "react";
import {
	type FieldErrors,
	type UseFormRegister,
	useForm,
} from "react-hook-form";

interface QuadletFormValues {
	name: string;
	resource: string;
	content: string;
}

// The field controls shared by the create and edit dialogs. The backend
// derives the name from the URL and the resource type from the existing
// file path on update, so edit mode renders both read-only and only the
// content textarea is editable.
function QuadletFields({
	register,
	errors,
	isEdit,
}: {
	register: UseFormRegister<QuadletFormValues>;
	errors: FieldErrors<QuadletFormValues>;
	isEdit: boolean;
}) {
	return (
		<VStack spacing={4} align="stretch">
			{isEdit ? (
				<FormControl>
					<FormLabel htmlFor="quadlet-name">Name</FormLabel>
					<Input
						id="quadlet-name"
						isReadOnly
						fontFamily="mono"
						{...register("name")}
					/>
				</FormControl>
			) : (
				<FormControl isInvalid={!!errors.name} isRequired>
					<FormLabel htmlFor="quadlet-name">Name</FormLabel>
					<Input
						id="quadlet-name"
						placeholder="web (without .container extension)"
						{...register("name", {
							required: "Name is required",
							validate: (value) =>
								value.trim().length > 0 || "Name is required",
						})}
					/>
					<FormErrorMessage>{errors.name?.message}</FormErrorMessage>
				</FormControl>
			)}
			{isEdit ? (
				<FormControl>
					<FormLabel htmlFor="quadlet-resource">Resource type</FormLabel>
					<Input
						id="quadlet-resource"
						isReadOnly
						fontFamily="mono"
						{...register("resource")}
					/>
				</FormControl>
			) : (
				<FormControl isInvalid={!!errors.resource} isRequired>
					<FormLabel htmlFor="quadlet-resource">Resource type</FormLabel>
					<Select
						id="quadlet-resource"
						placeholder="Select resource type"
						{...register("resource", {
							required: "Resource type is required",
						})}
					>
						{QUADLET_RESOURCES.map((resource) => (
							<option key={resource} value={resource}>
								{resource}
							</option>
						))}
					</Select>
					<FormErrorMessage>{errors.resource?.message}</FormErrorMessage>
				</FormControl>
			)}
			<FormControl isInvalid={!!errors.content} isRequired>
				<FormLabel htmlFor="quadlet-content">Content</FormLabel>
				<Textarea
					id="quadlet-content"
					fontFamily="mono"
					rows={12}
					placeholder={"[Container]\nImage=docker.io/library/nginx:1.27"}
					{...register("content", {
						required: "Content is required",
						validate: (value) =>
							value.trim().length > 0 || "Content is required",
					})}
				/>
				<FormErrorMessage>{errors.content?.message}</FormErrorMessage>
			</FormControl>
		</VStack>
	);
}

interface CreateQuadletDialogProps {
	mutation: MutationSlice<CreateQuadletInput>;
	onClose: () => void;
}

// Create quadlet dialog: name, resource type, and raw INI content, all
// editable and all three submitted. This component is the form owner, so it
// mounts with the dialog (fields start empty on every open) and publishes
// its validity through the scope for the module footer; failure renders in
// the module's error region, success closes through it.
export function CreateQuadletDialog({
	mutation,
	onClose,
}: CreateQuadletDialogProps) {
	const { register, handleSubmit, formState } = useForm<QuadletFormValues>({
		defaultValues: { name: "", resource: "", content: "" },
		// onChange so the footer submit's disabled state tracks validity as
		// the user types, not only after a failed submit attempt.
		mode: "onChange",
	});
	const { errors } = formState;

	const submit = handleSubmit((values) => {
		// The select only offers QUADLET_RESOURCES; narrow the form string back
		// to the typed payload field.
		const resource = QUADLET_RESOURCES.find(
			(candidate) => candidate === values.resource,
		);
		if (!resource) return;
		mutation.mutate({
			name: values.name.trim(),
			resource,
			content: values.content,
		});
	});

	return (
		<FormValidityScope formState={formState}>
			<FormDialog
				mutation={mutation}
				title="Create quadlet"
				onClose={onClose}
				formId="create-quadlet-form"
				submitLabel="Create quadlet"
				submitTestId="quadlet-form-submit"
				body={
					<form id="create-quadlet-form" onSubmit={submit} noValidate>
						<QuadletFields register={register} errors={errors} isEdit={false} />
					</form>
				}
			/>
		</FormValidityScope>
	);
}

interface EditQuadletDialogProps {
	machineId: string;
	quadlet: QuadletSummary;
	mutation: MutationSlice<{ quadletId: string } & UpdateQuadletInput>;
	onClose: () => void;
}

// Edit quadlet dialog: only the content textarea is editable and only the
// content is submitted. The detail query lives here because the in-modal
// entity loading and error states are the body's own — the module has no
// query knowledge — and the arriving content seeds the form, which is what
// flips the footer submit from its disabled start to enabled. The query
// drops its cache on unmount (the freshness factory's one-shot flavor),
// so reopening after a save never serves the pre-edit file; keepDirtyValues
// keeps user edits if a refetch lands mid-edit.
export function EditQuadletDialog({
	machineId,
	quadlet,
	mutation,
	onClose,
}: EditQuadletDialogProps) {
	const { data, isPending, error } = useQuadlet(machineId, quadlet.id);
	const { register, handleSubmit, reset, formState } =
		useForm<QuadletFormValues>({
			defaultValues: { name: "", resource: "", content: "" },
			// onChange so the footer submit's disabled state tracks validity as
			// the user types, not only after a failed submit attempt.
			mode: "onChange",
		});
	const { errors } = formState;

	useEffect(() => {
		if (data) {
			reset(
				{ name: data.name, resource: data.resource, content: data.content },
				{ keepDirtyValues: true },
			);
		}
	}, [data, reset]);

	const submit = handleSubmit((values) => {
		mutation.mutate({ quadletId: quadlet.id, content: values.content });
	});

	return (
		<FormValidityScope formState={formState}>
			<FormDialog
				mutation={mutation}
				title={`Edit quadlet — ${quadlet.name}`}
				onClose={onClose}
				formId="edit-quadlet-form"
				submitLabel="Save changes"
				submitTestId="quadlet-form-submit"
				body={
					<form id="edit-quadlet-form" onSubmit={submit} noValidate>
						{isPending ? (
							<Text data-testid="quadlet-edit-loading">Loading content…</Text>
						) : error ? (
							<ErrorDisplay error={getApiError(error)} />
						) : data ? (
							<QuadletFields register={register} errors={errors} isEdit />
						) : null}
					</form>
				}
			/>
		</FormValidityScope>
	);
}
