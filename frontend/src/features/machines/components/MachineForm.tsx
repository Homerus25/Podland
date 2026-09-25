import {
	FormControl,
	FormErrorMessage,
	FormLabel,
	HStack,
	Input,
	VStack,
} from "@chakra-ui/react";
import type {
	CreateMachineInput,
	UpdateMachineInput,
} from "@features/machines/services/machinesApi";
import { ErrorDisplay } from "@shared/components/ErrorDisplay";
import {
	FormDialog,
	FormFooter,
	FormValidityScope,
	type MutationSlice,
} from "@shared/components/MutationDialog";
import type { Machine } from "@shared/types/api";
import { getApiError } from "@shared/utils/api";
import { useForm } from "react-hook-form";

// What the form produces: all metadata fields plus the key only when a file
// was selected (create mode validates its presence before submit).
export type MachineFormPayload = Omit<UpdateMachineInput, "sshPrivateKey"> & {
	sshPrivateKey?: string;
};

interface MachineFormValues {
	name: string;
	description: string;
	hostname: string;
	port: string;
	sshUsername: string;
	podmanSocketPath: string;
	sshPrivateKeyFile?: FileList;
}

const portPattern = /^(0|[1-9]\d{0,4})$/;

function toPort(value: string): number {
	if (!portPattern.test(value)) return NaN;
	const port = Number(value);
	return port >= 1 && port <= 65535 ? port : NaN;
}

// File API read of the SSH private key into the request body. FileReader is
// used instead of File.text() for jsdom compatibility.
function readFile(file: File): Promise<string> {
	return new Promise((resolve, reject) => {
		const reader = new FileReader();
		reader.onload = () => resolve(reader.result as string);
		reader.onerror = () => reject(reader.error);
		reader.readAsText(file);
	});
}

// Create mode: the dialog's form — fire-and-forget, the module closes on
// success and renders the failure, so the form only forwards the payload.
interface MachineFormCreateProps {
	mode?: "create";
	mutation: MutationSlice<CreateMachineInput>;
	onClose: () => void;
}

// Edit mode: the settings tab owns its saved/error display and awaits the
// mutation — there is no host dialog to own the seams for it, so the form
// keeps a single `pending` prop for the footer's busy gate and an `onCancel`
// seam for the host's mutation reset (which clears the error text and the
// saved banner); the revert of the fields themselves is the form's own.
interface MachineFormEditProps {
	mode: "edit";
	machine: Machine;
	onSubmit: (input: MachineFormPayload) => Promise<void>;
	onCancel: () => void;
	submitError: unknown;
	pending: boolean;
}

export type MachineFormProps = MachineFormCreateProps | MachineFormEditProps;

// MachineForm serves both create and edit mode. In create mode the SSH
// private key file is required; in edit mode it is optional — omitting it
// leaves the stored key untouched, uploading a new file re-sends it. The key
// file is read client-side via the File API.
export function MachineForm(props: MachineFormProps) {
	const isEdit = props.mode === "edit";
	const machine = props.mode === "edit" ? props.machine : undefined;
	const { register, handleSubmit, setError, reset, formState } =
		useForm<MachineFormValues>({
			defaultValues: machine
				? {
						name: machine.name,
						description: machine.description ?? "",
						hostname: machine.hostname,
						port: String(machine.port),
						sshUsername: machine.sshUsername,
						podmanSocketPath: machine.podmanSocketPath ?? "",
					}
				: { port: "22", description: "", podmanSocketPath: "" },
			// The dialog's footer submit gates on validity, so the create form
			// re-validates as the user types; the settings tab's footer button
			// keeps submit-time validation (no validity scope is published).
			mode: isEdit ? "onSubmit" : "onChange",
		});
	const { errors } = formState;

	const submit = handleSubmit(async (values) => {
		const keyFile = values.sshPrivateKeyFile?.[0];
		let sshPrivateKey: string | undefined;
		if (keyFile) {
			try {
				sshPrivateKey = await readFile(keyFile);
			} catch {
				setError("sshPrivateKeyFile", {
					message: "Could not read the selected key file",
				});
				return;
			}
		} else if (!isEdit) {
			setError("sshPrivateKeyFile", {
				message: "SSH private key file is required",
			});
			return;
		}
		const payload: MachineFormPayload = {
			name: values.name,
			description: values.description,
			hostname: values.hostname,
			port: toPort(values.port),
			sshUsername: values.sshUsername,
			...(sshPrivateKey !== undefined ? { sshPrivateKey } : {}),
			podmanSocketPath: values.podmanSocketPath,
		};
		if (props.mode === "edit") {
			await props.onSubmit(payload);
			return;
		}
		// Create-mode validation requires the key file, so the payload always
		// carries the key here; the payload type cannot see that.
		props.mutation.mutate(payload as CreateMachineInput);
	});

	const fields = (
		<>
			<FormControl isInvalid={!!errors.name} isRequired>
				<FormLabel htmlFor="machine-name">Name</FormLabel>
				<Input
					id="machine-name"
					{...register("name", { required: "Name is required" })}
				/>
				<FormErrorMessage>{errors.name?.message}</FormErrorMessage>
			</FormControl>
			<FormControl isInvalid={!!errors.description}>
				<FormLabel htmlFor="machine-description">Description</FormLabel>
				<Input id="machine-description" {...register("description")} />
				<FormErrorMessage>{errors.description?.message}</FormErrorMessage>
			</FormControl>
			<FormControl isInvalid={!!errors.hostname} isRequired>
				<FormLabel htmlFor="machine-hostname">Hostname or IP</FormLabel>
				<Input
					id="machine-hostname"
					{...register("hostname", { required: "Hostname is required" })}
				/>
				<FormErrorMessage>{errors.hostname?.message}</FormErrorMessage>
			</FormControl>
			<FormControl isInvalid={!!errors.port} isRequired>
				<FormLabel htmlFor="machine-port">SSH port</FormLabel>
				<Input
					id="machine-port"
					inputMode="numeric"
					{...register("port", {
						required: "Port is required",
						validate: (value) =>
							!Number.isNaN(toPort(value)) ||
							"Port must be a number between 1 and 65535",
					})}
				/>
				<FormErrorMessage>{errors.port?.message}</FormErrorMessage>
			</FormControl>
			<FormControl isInvalid={!!errors.sshUsername} isRequired>
				<FormLabel htmlFor="machine-ssh-username">SSH username</FormLabel>
				<Input
					id="machine-ssh-username"
					{...register("sshUsername", {
						required: "SSH username is required",
					})}
				/>
				<FormErrorMessage>{errors.sshUsername?.message}</FormErrorMessage>
			</FormControl>
			<FormControl>
				<FormLabel htmlFor="machine-podman-socket">
					Podman socket path (optional)
				</FormLabel>
				<Input id="machine-podman-socket" {...register("podmanSocketPath")} />
				<FormErrorMessage>{errors.podmanSocketPath?.message}</FormErrorMessage>
			</FormControl>
			<FormControl isInvalid={!!errors.sshPrivateKeyFile} isRequired={!isEdit}>
				<FormLabel htmlFor="machine-ssh-key">
					{isEdit
						? "SSH private key (optional — leave empty to keep the current key)"
						: "SSH private key"}
				</FormLabel>
				<Input
					id="machine-ssh-key"
					type="file"
					{...register("sshPrivateKeyFile", {
						required: !isEdit ? "SSH private key file is required" : false,
					})}
				/>
				<FormErrorMessage>{errors.sshPrivateKeyFile?.message}</FormErrorMessage>
			</FormControl>
		</>
	);

	// The settings tab's non-modal form joins the module's footer: the footer
	// button submits by form id and gates on the host mutation's pending —
	// the one `pending` prop this site keeps because its mutation lives in
	// the host tab. The submit-error seam and the saved banner stay with the
	// host tab, above the fields; cancel reverts the fields and fires the
	// host's onCancel.
	if (props.mode === "edit") {
		// Revert the fields to the machine's stored values (the form's seed);
		// the host's onCancel resets its mutation, clearing the error text
		// and the saved banner so neither sits over the reverted fields.
		const handleCancel = () => {
			reset();
			props.onCancel();
		};
		return (
			<form id="machine-settings-form" onSubmit={submit} noValidate>
				<VStack spacing={4} align="stretch">
					{props.submitError ? (
						<ErrorDisplay error={getApiError(props.submitError)} />
					) : null}
					{fields}
					<HStack justify="flex-end">
						<FormFooter
							formId="machine-settings-form"
							submitLabel="Save changes"
							submitTestId="machine-form-submit"
							pending={props.pending}
							onCancel={handleCancel}
							cancelTestId="machine-form-cancel"
						/>
					</HStack>
				</VStack>
			</form>
		);
	}

	// The add-machine dialog: this component is the form owner, so it mounts
	// with the dialog (fields start empty on every open) and publishes its
	// validity through the scope for the module footer; failure renders in
	// the module's error region, success closes through it.
	return (
		<FormValidityScope formState={formState}>
			<FormDialog
				mutation={props.mutation}
				title="Add Machine"
				onClose={props.onClose}
				formId="add-machine-form"
				submitLabel="Add machine"
				submitTestId="machine-form-submit"
				body={
					<form id="add-machine-form" onSubmit={submit} noValidate>
						<VStack spacing={4} align="stretch">
							{fields}
						</VStack>
					</form>
				}
			/>
		</FormValidityScope>
	);
}
