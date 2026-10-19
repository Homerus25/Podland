import {
	Box,
	Input,
	Tag,
	TagCloseButton,
	TagLabel,
	Text,
} from "@chakra-ui/react";
import { type KeyboardEvent, useEffect, useId, useRef, useState } from "react";

// The single owner of the combobox ritual the three #251 dialogs need
// (the create-network multi-select and the two connect dialogs, both
// multi-select since issue 320): a search input, a filtered option list, keyboard
// navigation, an explicit empty state, and single- and multi-select
// modes. Chakra primitives only — no combobox library. The behavior is
// pinned once in SearchableSelect.test.tsx at the rendered DOM; the
// dialogs are adapters and consume it without re-pinning.
//
// Contract: the list opens on input focus and closes on Escape or on a
// pick in single mode (multi stays open). ArrowUp/ArrowDown move the
// active option (clamped at both ends), Enter selects it, and Enter is
// swallowed while the list is open so a dialog form never submits from
// the search field. Chosen options render as removable chips and leave
// the option list in both modes. The empty state renders whenever the
// filtered list is empty — no match for the search, or nothing
// eligible — so the list is never a silent blank.

export type SearchableSelectOption = {
	value: string;
	label: string;
};

export type SearchableSelectProps = {
	options: SearchableSelectOption[];
	searchPlaceholder?: string;
	emptyMessage?: string;
} & (
	| { mode: "single"; value: string; onChange: (value: string) => void }
	| { mode: "multi"; value: string[]; onChange: (value: string[]) => void }
);

export function SearchableSelect(props: SearchableSelectProps) {
	const {
		options,
		searchPlaceholder = "Search…",
		emptyMessage = "No matches found.",
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

	const selectedValues: string[] =
		props.mode === "multi"
			? props.value
			: props.value === ""
				? []
				: [props.value];
	const selectedSet = new Set(selectedValues);
	const selectedOptions = options.filter((option) =>
		selectedSet.has(option.value),
	);

	const query = search.trim().toLowerCase();
	const filtered = options.filter(
		(option) =>
			!selectedSet.has(option.value) &&
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

	const select = (value: string) => {
		if (props.mode === "single") {
			props.onChange(value);
			setOpen(false);
		} else {
			props.onChange([...props.value, value]);
		}
		setSearch("");
		setActiveIndex(0);
	};

	const remove = (value: string) => {
		if (props.mode === "single") {
			props.onChange("");
		} else {
			props.onChange(props.value.filter((v) => v !== value));
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
							key={option.value}
							data-testid={`searchable-select-chip-${option.value}`}
						>
							<TagLabel>{option.label}</TagLabel>
							<TagCloseButton
								aria-label={`Remove ${option.label}`}
								data-testid={`searchable-select-remove-${option.value}`}
								onClick={() => remove(option.value)}
							/>
						</Tag>
					))}
				</Box>
			)}
			<Input
				role="combobox"
				aria-expanded={open}
				aria-controls={listId}
				aria-activedescendant={open && active >= 0 ? itemId(active) : undefined}
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
							key={option.value}
							role="option"
							id={itemId(index)}
							data-testid={`searchable-select-option-${option.value}`}
							data-active={index === active}
							aria-selected={selectedSet.has(option.value)}
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
