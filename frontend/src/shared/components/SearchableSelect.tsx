import {
	Box,
	Input,
	Tag,
	TagCloseButton,
	TagLabel,
	Text,
} from "@chakra-ui/react";
import { type KeyboardEvent, useEffect, useId, useRef, useState } from "react";

// The single owner of the combobox ritual in the app: a search input, a
// filtered option list, keyboard navigation, an explicit empty state, and
// single- and multi-select modes. Chakra primitives only — no combobox
// library. Every combobox in the app consumes this core: the three #251
// dialogs (the create-network multi-select and the two connect dialogs,
// both multi-select since issue 320) as string-valued adapters, and the
// users feature's UserPicker as an object-valued single-select adapter.
// The behavior is pinned once in SearchableSelect.test.tsx at the rendered
// DOM; the adapters consume it without re-pinning.
//
// The core is generic over the option value: an option carries its value
// as an object when the caller needs more than a string (UserPicker hands
// the full user summary to its caller). Selection compares by the keyOf
// key when the adapter hands one — object-valued adapters rebuild their
// option objects on every fetch, so identity would strand a held
// selection — and by value identity otherwise. Per-option testids and
// React keys use the keyOf key when handed, the value itself when it is
// a string, and the label otherwise, so string-valued adapters keep
// their value-derived testids.
//
// Contract: the list opens on input focus and closes on Escape or on a
// pick in single mode (multi stays open). ArrowUp/ArrowDown move the
// active option (clamped at both ends), Enter selects it, and Enter is
// swallowed while the list is open so a dialog form never submits from
// the search field. Chosen options render as removable chips and leave
// the option list in both modes. The empty state renders whenever the
// filtered list is empty — no match for the search, or nothing
// eligible — so the list is never a silent blank. The input follows the
// ARIA 1.2 combobox pattern (aria-autocomplete="list", autoComplete off);
// aria-selected marks the selection, never the highlight.

export type SearchableSelectOption<T> = {
	value: T;
	label: string;
};

export type SearchableSelectProps<T> = {
	options: SearchableSelectOption<T>[];
	searchPlaceholder?: string;
	emptyMessage?: string;
	// Optional id for the search input, so a surrounding form label can
	// associate with it (htmlFor). The core's only pass-through prop.
	id?: string;
	// Optional identity for object-valued options: the key the selection
	// compares values by and the per-option testids key off. Adapters
	// that rebuild option objects on every fetch (UserPicker's directory
	// refetch) must hand one, or a held selection goes stale the moment
	// the options array is re-created.
	keyOf?: (value: T) => string;
} & (
	| { mode: "single"; value: T | null; onChange: (value: T | null) => void }
	| { mode: "multi"; value: T[]; onChange: (value: T[]) => void }
);

export function SearchableSelect<T>(props: SearchableSelectProps<T>) {
	const {
		options,
		searchPlaceholder = "Search…",
		emptyMessage = "No matches found.",
		id,
		keyOf,
	} = props;
	const [search, setSearch] = useState("");
	const [open, setOpen] = useState(false);
	const [activeIndex, setActiveIndex] = useState(0);
	const listRef = useRef<HTMLDivElement>(null);
	// Per-instance id prefix so two selects on one page never collide;
	// useId's colons are stripped to keep querySelector selectors simple.
	const idPrefix = useId().replace(/:/g, "");
	const listId = `${idPrefix}-list`;
	const itemId = (index: number) => `${idPrefix}-item-${index}`;
	// Per-option testid key: the keyOf key when handed, the value itself
	// when it is a string (the network dialogs' ids), the label for
	// object-valued options without a keyOf.
	const optionKey = (option: SearchableSelectOption<T>) =>
		keyOf
			? keyOf(option.value)
			: typeof option.value === "string"
				? option.value
				: option.label;

	const selectedValues: T[] =
		props.mode === "multi"
			? props.value
			: props.value === null
				? []
				: [props.value];
	const selectedSet = new Set(selectedValues);
	const selectedKeys = keyOf ? new Set(selectedValues.map(keyOf)) : null;
	const isSelected = (value: T) =>
		keyOf && selectedKeys
			? selectedKeys.has(keyOf(value))
			: selectedSet.has(value);
	const selectedOptions = options.filter((option) => isSelected(option.value));

	const query = search.trim().toLowerCase();
	const filtered = options.filter(
		(option) =>
			!isSelected(option.value) &&
			(query === "" || option.label.toLowerCase().includes(query)),
	);
	const active =
		filtered.length === 0 ? -1 : Math.min(activeIndex, filtered.length - 1);

	useEffect(() => {
		if (!open || active < 0) return;
		listRef.current
			?.querySelector(`#${idPrefix}-item-${active}`)
			?.scrollIntoView?.({ block: "nearest" });
	}, [open, active, idPrefix]);

	const select = (value: T) => {
		if (props.mode === "single") {
			props.onChange(value);
			setOpen(false);
		} else {
			props.onChange([...props.value, value]);
		}
		setSearch("");
		setActiveIndex(0);
	};

	const remove = (value: T) => {
		if (props.mode === "single") {
			props.onChange(null);
		} else {
			props.onChange(
				props.value.filter((v) =>
					keyOf ? keyOf(v) !== keyOf(value) : v !== value,
				),
			);
		}
	};

	const onKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
		if (event.key === "ArrowDown") {
			event.preventDefault();
			if (open) {
				setActiveIndex((index) => Math.min(index + 1, filtered.length - 1));
			} else {
				setOpen(true);
				setActiveIndex(0);
			}
		} else if (event.key === "ArrowUp") {
			event.preventDefault();
			if (open) {
				setActiveIndex((index) => Math.max(index - 1, 0));
			} else {
				setOpen(true);
				setActiveIndex(0);
			}
		} else if (event.key === "Enter") {
			if (open) {
				event.preventDefault();
				const option = filtered[active];
				if (option) select(option.value);
			}
		} else if (event.key === "Escape" && open) {
			event.preventDefault();
			event.stopPropagation();
			setOpen(false);
		}
	};

	return (
		<Box>
			{selectedOptions.length > 0 && (
				<Box
					display="flex"
					flexWrap="wrap"
					gap={1}
					mb={1}
					data-testid="searchable-select-chips"
				>
					{selectedOptions.map((option) => (
						<Tag
							key={optionKey(option)}
							data-testid={`searchable-select-chip-${optionKey(option)}`}
						>
							<TagLabel>{option.label}</TagLabel>
							<TagCloseButton
								aria-label={`Remove ${option.label}`}
								data-testid={`searchable-select-remove-${optionKey(option)}`}
								onClick={() => remove(option.value)}
							/>
						</Tag>
					))}
				</Box>
			)}
			<Input
				id={id}
				role="combobox"
				aria-autocomplete="list"
				aria-expanded={open}
				aria-controls={listId}
				aria-activedescendant={open && active >= 0 ? itemId(active) : undefined}
				autoComplete="off"
				data-testid="searchable-select-input"
				placeholder={searchPlaceholder}
				value={search}
				onChange={(event) => {
					setSearch(event.target.value);
					setActiveIndex(0);
					setOpen(true);
				}}
				onFocus={() => setOpen(true)}
				onClick={() => setOpen(true)}
				onKeyDown={onKeyDown}
			/>
			{open && (
				<Box
					ref={listRef}
					role="listbox"
					id={listId}
					data-testid="searchable-select-list"
					borderWidth="1px"
					borderRadius="md"
					mt={1}
					maxH="240px"
					overflowY="auto"
				>
					{filtered.map((option, index) => (
						<Box
							key={optionKey(option)}
							role="option"
							id={itemId(index)}
							data-testid={`searchable-select-option-${optionKey(option)}`}
							data-active={index === active}
							aria-selected={isSelected(option.value)}
							px={2}
							py={1}
							cursor="pointer"
							bg={index === active ? "gray.100" : undefined}
							_hover={{ bg: "gray.100" }}
							onClick={() => select(option.value)}
						>
							{option.label}
						</Box>
					))}
					{filtered.length === 0 && (
						<Text
							color="gray.500"
							px={2}
							py={1}
							data-testid="searchable-select-empty"
						>
							{emptyMessage}
						</Text>
					)}
				</Box>
			)}
		</Box>
	);
}
