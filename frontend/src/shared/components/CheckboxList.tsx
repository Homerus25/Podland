import { Checkbox } from "@chakra-ui/react";

// A multi-select checkbox list over a fetched option list (the container
// create page's Network and Secrets sections, the pod create dialog's
// Networks section share this shape). The option's key is the selection
// identity — not its label, so two same-named options cannot toggle
// together. The checkbox is controlled by the watched selection and
// toggling rewrites it.
export function CheckboxList({
	options,
	selected,
	onToggle,
	isDisabled = false,
}: {
	options: { key: string; label: string }[];
	selected: string[];
	onToggle: (key: string, checked: boolean) => void;
	// The container create page's pod selector (spec 356) disables the
	// Network section's list while a pod is chosen — the pod owns networking.
	isDisabled?: boolean;
}) {
	return (
		<>
			{options.map((option) => (
				<Checkbox
					key={option.key}
					isChecked={selected.includes(option.key)}
					isDisabled={isDisabled}
					onChange={(event) => onToggle(option.key, event.target.checked)}
				>
					{option.label}
				</Checkbox>
			))}
		</>
	);
}
