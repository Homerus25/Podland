import { Checkbox } from "@chakra-ui/react";

// A multi-select checkbox list over a fetched option list (the container
// create page's Network and Secrets sections, the pod create dialog's
// Networks section share this shape). The option's key is the selection
// identity — not its label, so two same-named options cannot toggle
// together. The checkbox is controlled by the watched selection and the
// list owns the toggle: the callback receives the next selection (checked
// appends the key, unchecked removes it), so no consumer re-writes the
// append/remove dance (spec #371, ticket #366).
export function CheckboxList({
	options,
	selected,
	onSelect,
	isDisabled = false,
}: {
	options: { key: string; label: string }[];
	selected: string[];
	onSelect: (nextSelection: string[]) => void;
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
					onChange={(event) =>
						onSelect(
							event.target.checked
								? [...selected, option.key]
								: selected.filter((item) => item !== option.key),
						)
					}
				>
					{option.label}
				</Checkbox>
			))}
		</>
	);
}
