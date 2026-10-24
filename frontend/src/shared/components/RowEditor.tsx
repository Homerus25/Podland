import { AddIcon, DeleteIcon } from "@chakra-ui/icons";
import {
	Box,
	Button,
	Checkbox,
	Heading,
	HStack,
	IconButton,
	Input,
	Select,
	VStack,
} from "@chakra-ui/react";
import type { ReactNode } from "react";
import type {
	FieldArrayPath,
	FieldValues,
	UseFieldArrayReturn,
	UseFormRegisterReturn,
	UseFormSetValue,
} from "react-hook-form";

// The structured row-editor ritual, owned once (spec #371, ticket #366):
// the sibling the key/value editor has for non-key/value rows. The module
// owns the row chrome — the row list editor over a field array (the
// per-row remove button, the add button, the noun-derived testids, the
// remove-aria derivation), the row input and select controls, the labeled
// section block, and the bind-mount z/Z/U flags with their mutual
// exclusion. The adapter owns the form and hands over the field-array
// return, register results, and copy; the module never calls form hooks
// (the key/value editor's contract).

// One text input inside a field-array row.
export function RowInput({
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
export function RowSelect({
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

// One labeled block inside a form; sections are separated by Dividers.
export function FormSection({
	title,
	children,
}: {
	title: string;
	children: ReactNode;
}) {
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

interface RowListEditorProps<
	TFieldValues extends FieldValues,
	TName extends FieldArrayPath<TFieldValues>,
> {
	// The adapter's useFieldArray return; the module never calls form hooks.
	rows: UseFieldArrayReturn<TFieldValues, TName, "id">;
	// Singular noun driving the testids: add-<noun>, remove-<noun>-<index>.
	noun: string;
	// Add button copy.
	addLabel: string;
	// Per-row remove aria label, taking the 1-based row number.
	removeAriaLabel: (rowNumber: number) => string;
	// The row's controls — the row shape and its copy stay the adapter's.
	renderRow: (index: number) => ReactNode;
	// Appends the adapter's row shape; the module owns the button chrome.
	onAdd: () => void;
	// Optional slot beside the add button for sibling actions (the inline
	// volume create is the consumer).
	addArea?: ReactNode;
}

// The row list editor over a field array: the per-row remove button, the
// add button, the noun-derived testids, and the remove-aria derivation.
// The adapter hands over the field-array return, the copy, and the row's
// controls through the render seam.
export function RowListEditor<
	TFieldValues extends FieldValues,
	TName extends FieldArrayPath<TFieldValues>,
>({
	rows,
	noun,
	addLabel,
	removeAriaLabel,
	renderRow,
	onAdd,
	addArea,
}: RowListEditorProps<TFieldValues, TName>) {
	const addButton = (
		<Button
			leftIcon={<AddIcon />}
			size="sm"
			variant="ghost"
			onClick={onAdd}
			data-testid={`add-${noun}`}
		>
			{addLabel}
		</Button>
	);
	return (
		<>
			{rows.fields.map((field, index) => (
				<HStack key={field.id} spacing={2} align="flex-start">
					{renderRow(index)}
					<IconButton
						aria-label={removeAriaLabel(index + 1)}
						icon={<DeleteIcon />}
						size="sm"
						variant="ghost"
						onClick={() => rows.remove(index)}
						data-testid={`remove-${noun}-${index}`}
					/>
				</HStack>
			))}
			{addArea ? (
				<HStack spacing={2}>
					{addButton}
					{addArea}
				</HStack>
			) : (
				addButton
			)}
		</>
	);
}

// The watched flag values of one bind-mount row; both create surfaces'
// mount rows structurally satisfy this shape.
export interface MountFlagValues {
	selinuxShared?: boolean;
	selinuxPrivate?: boolean;
	chownUser?: boolean;
}

// The z/Z/U flag checkboxes of one bind-mount row (spec 341, ticket
// #348): z (SELinux shared relabel) and Z (SELinux private relabel) are
// mutually exclusive — checking one unchecks the other — while U (chown
// to the container's user) is independent. The checkboxes are controlled
// by the watched row values; toggling rewrites them through the
// adapter's setValue. The flags' copy is module-owned (identical on both
// create surfaces). Generic over the surface's form values: the module
// absorbs the cast itself — setValue is widened once here to the
// string-path/boolean shape (the widened-setValue precedent, the
// key/value editor's register-widening sibling), so the call sites
// stay clean — at the cost of path type-safety for these flag writes:
// the template-literal paths are not checked against the adapter's
// form type.
export function MountFlags<TFieldValues extends FieldValues>({
	name,
	index,
	row,
	setValue,
}: {
	// Field-array path of the mount rows in the adapter's form.
	name: string;
	index: number;
	row: MountFlagValues | undefined;
	setValue: UseFormSetValue<TFieldValues>;
}) {
	const setFlag = setValue as (path: string, value: boolean) => void;
	return (
		<HStack spacing={3} pt={2}>
			<Checkbox
				isChecked={row?.selinuxShared ?? false}
				onChange={(event) => {
					const checked = event.target.checked;
					setFlag(`${name}.${index}.selinuxShared`, checked);
					if (checked) {
						setFlag(`${name}.${index}.selinuxPrivate`, false);
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
					setFlag(`${name}.${index}.selinuxPrivate`, checked);
					if (checked) {
						setFlag(`${name}.${index}.selinuxShared`, false);
					}
				}}
				aria-label={`Bind mount ${index + 1} SELinux private (Z)`}
			>
				Z
			</Checkbox>
			<Checkbox
				isChecked={row?.chownUser ?? false}
				onChange={(event) =>
					setFlag(`${name}.${index}.chownUser`, event.target.checked)
				}
				aria-label={`Bind mount ${index + 1} chown to user (U)`}
			>
				U
			</Checkbox>
		</HStack>
	);
}
