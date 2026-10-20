import { AddIcon, DeleteIcon } from "@chakra-ui/icons";
import {
	Button,
	FormControl,
	FormErrorMessage,
	HStack,
	IconButton,
	Input,
} from "@chakra-ui/react";
import type { KeyValueRow } from "@shared/utils/keyValueRows";
import type {
	FieldArrayPath,
	FieldValues,
	RegisterOptions,
	UseFieldArrayReturn,
	UseFormRegister,
	UseFormRegisterReturn,
} from "react-hook-form";

interface KeyValueEditorProps<
	TFieldValues extends FieldValues,
	TName extends FieldArrayPath<TFieldValues>,
> {
	// Field-array path of the key/value rows in the adapter's form.
	name: TName;
	// The adapter's useFieldArray return; the module never calls form hooks.
	rows: UseFieldArrayReturn<TFieldValues, TName, "id">;
	// The adapter's register, for the row inputs.
	register: UseFormRegister<TFieldValues>;
	// Per-row key error message, read from the adapter's formState.errors.
	keyErrorMessage: (index: number) => string | undefined;
	// Singular noun driving the testids: add-<noun>, remove-<noun>-<index>.
	noun: string;
	// Add button copy.
	addLabel: string;
	// Per-row aria labels, taking the 1-based row number.
	keyAriaLabel: (rowNumber: number) => string;
	valueAriaLabel: (rowNumber: number) => string;
	removeAriaLabel: (rowNumber: number) => string;
	// Input placeholders.
	keyPlaceholder?: string;
	valuePlaceholder?: string;
	// Require a key when the row's value is set; default off.
	requireKeyWithValue?: boolean;
}

// The key/value row-editor ritual, owned once: the mono key/value inputs,
// the per-row remove buttons, and the add button over a field array. The
// adapter owns the form and useFieldArray and hands over the return; the
// module renders the rows and owns the optional per-row key validation.
export function KeyValueEditor<
	TFieldValues extends FieldValues,
	TName extends FieldArrayPath<TFieldValues>,
>({
	name,
	rows,
	register,
	keyErrorMessage,
	noun,
	addLabel,
	keyAriaLabel,
	valueAriaLabel,
	removeAriaLabel,
	keyPlaceholder = "key",
	valuePlaceholder = "value",
	requireKeyWithValue = false,
}: KeyValueEditorProps<TFieldValues, TName>) {
	// The row paths are valid FieldPaths by construction (name is a
	// field-array path of TFieldValues); the widened register view keeps the
	// template-literal paths type-safe without casts at the call sites.
	const registerRow = register as (
		path: string,
		options?: RegisterOptions<TFieldValues>,
	) => UseFormRegisterReturn;
	// The adapter's field array holds key/value rows; the widened append
	// view types it over the module's row shape.
	const appendRow = rows.append as (row: KeyValueRow) => void;

	return (
		<>
			{rows.fields.map((field, index) => {
				const keyMessage = keyErrorMessage(index);
				return (
					<HStack key={field.id} spacing={2} align="flex-start">
						<FormControl isInvalid={!!keyMessage} flex="1">
							<Input
								fontFamily="mono"
								placeholder={keyPlaceholder}
								aria-label={keyAriaLabel(index + 1)}
								{...(requireKeyWithValue
									? registerRow(`${name}.${index}.key`, {
											setValueAs: (value: string) => value.trim(),
											validate: (value, formValues) =>
												value.trim() !== "" ||
												!rowHasValue(formValues, name, index) ||
												"Key is required when a value is set",
										})
									: registerRow(`${name}.${index}.key`))}
							/>
							<FormErrorMessage>{keyMessage}</FormErrorMessage>
						</FormControl>
						<FormControl flex="1">
							<Input
								fontFamily="mono"
								placeholder={valuePlaceholder}
								aria-label={valueAriaLabel(index + 1)}
								{...registerRow(`${name}.${index}.value`)}
							/>
						</FormControl>
						<IconButton
							aria-label={removeAriaLabel(index + 1)}
							icon={<DeleteIcon />}
							size="sm"
							variant="ghost"
							onClick={() => rows.remove(index)}
							data-testid={`remove-${noun}-${index}`}
						/>
					</HStack>
				);
			})}
			<Button
				leftIcon={<AddIcon />}
				size="sm"
				variant="ghost"
				onClick={() => appendRow({ key: "", value: "" })}
				data-testid={`add-${noun}`}
			>
				{addLabel}
			</Button>
		</>
	);
}

// Whether the row at index carries a value, read from the form values the
// validate callback receives — so the module never touches the form itself.
function rowHasValue<TFieldValues extends FieldValues>(
	formValues: TFieldValues,
	name: string,
	index: number,
): boolean {
	const rows = (formValues as Record<string, KeyValueRow[]>)[name];
	return (rows?.[index]?.value ?? "").trim() !== "";
}
