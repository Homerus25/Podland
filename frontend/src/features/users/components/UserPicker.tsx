import {
	Box,
	FormControl,
	FormLabel,
	Input,
	List,
	ListItem,
	Text,
} from "@chakra-ui/react";
import { useUsers } from "@features/users/hooks/useUsers";
import type { UserSummary } from "@features/users/services/usersApi";
import { ErrorDisplay } from "@shared/components/ErrorDisplay";
import { getApiError } from "@shared/utils/api";
import { type KeyboardEvent, useId, useMemo, useState } from "react";

interface UserPickerProps {
	// The currently picked user, or null while nothing is picked. Editing
	// the text discards the selection (onChange(null)) so the displayed
	// text can never disagree with the caller's state.
	value: UserSummary | null;
	onChange: (user: UserSummary | null) => void;
	// Users the picker must not offer (e.g. the machine owner and users
	// who already hold a grant on the share form).
	excludeUserIds?: string[];
}

// Searchable username picker over the users directory, hand-rolled on the
// UI kit as an ARIA combobox: typing filters the directory client-side,
// ArrowDown/ArrowUp/Enter pick with the keyboard, a click picks with the
// mouse. It hands the full UserSummary to the caller, so consumers submit
// the user's id without ever parsing the text input.
export function UserPicker({
	value,
	onChange,
	excludeUserIds,
}: UserPickerProps) {
	const { data: users, isPending, error } = useUsers();
	// null means "not editing": the input then displays the picked
	// username, so a selection survives re-renders without duplicated
	// state. Any keystroke replaces it with the query text.
	const [query, setQuery] = useState<string | null>(null);
	const [open, setOpen] = useState(false);
	const [activeIndex, setActiveIndex] = useState(0);
	const idPrefix = useId().replace(/:/g, "");
	const inputId = `${idPrefix}-input`;
	const listboxId = `${idPrefix}-listbox`;

	const choices = useMemo(() => {
		const excluded = new Set(excludeUserIds);
		const needle = (query ?? "").trim().toLowerCase();
		return (users ?? []).filter(
			(user) =>
				!excluded.has(user.id) && user.username.toLowerCase().includes(needle),
		);
	}, [users, excludeUserIds, query]);

	const clampedIndex = Math.min(activeIndex, Math.max(choices.length - 1, 0));
	const active = choices[clampedIndex];

	const select = (user: UserSummary) => {
		onChange(user);
		setQuery(null);
		setOpen(false);
		setActiveIndex(0);
	};

	const handleKeyDown = (event: KeyboardEvent) => {
		if (event.key === "ArrowDown") {
			event.preventDefault();
			if (!open) {
				setOpen(true);
			} else {
				setActiveIndex(Math.min(clampedIndex + 1, choices.length - 1));
			}
		} else if (event.key === "ArrowUp") {
			event.preventDefault();
			setActiveIndex(Math.max(clampedIndex - 1, 0));
		} else if (event.key === "Enter" && open) {
			// The picker lives inside the share form: while the listbox is
			// open, Enter belongs to the picker, never to the form.
			event.preventDefault();
			if (active) {
				select(active);
			}
		} else if (event.key === "Escape") {
			setOpen(false);
			setActiveIndex(0);
		}
	};

	return (
		<FormControl>
			<FormLabel htmlFor={inputId}>User</FormLabel>
			{error ? <ErrorDisplay error={getApiError(error)} /> : null}
			<Box position="relative">
				<Input
					id={inputId}
					role="combobox"
					aria-autocomplete="list"
					aria-expanded={open}
					aria-controls={open ? listboxId : undefined}
					aria-activedescendant={
						open && active ? `${idPrefix}-option-${active.id}` : undefined
					}
					autoComplete="off"
					placeholder="Search users…"
					value={query ?? value?.username ?? ""}
					onChange={(event) => {
						setQuery(event.target.value);
						setActiveIndex(0);
						setOpen(true);
						if (value) {
							onChange(null);
						}
					}}
					onKeyDown={handleKeyDown}
					// onFocus alone cannot reopen after a selection: focus
					// never left the input, so the next click carries no
					// focus event.
					onClick={() => setOpen(true)}
					onFocus={() => setOpen(true)}
					onBlur={() => setOpen(false)}
					data-testid="user-picker-input"
				/>
				{open ? (
					choices.length > 0 ? (
						<List
							id={listboxId}
							role="listbox"
							position="absolute"
							zIndex="dropdown"
							w="100%"
							mt={1}
							maxH="240px"
							overflowY="auto"
							bg="white"
							_dark={{ bg: "gray.800" }}
							borderWidth="1px"
							borderRadius="md"
							boxShadow="sm"
							spacing={0}
							data-testid="user-picker-listbox"
						>
							{choices.map((user, index) => (
								<ListItem
									key={user.id}
									id={`${idPrefix}-option-${user.id}`}
									role="option"
									aria-selected={index === clampedIndex}
									// Keep focus on the input so the click fires
									// before the blur closes the listbox.
									onMouseDown={(event) => event.preventDefault()}
									onClick={() => select(user)}
									px={3}
									py={2}
									cursor="pointer"
									bg={index === clampedIndex ? "gray.100" : undefined}
									_dark={{
										bg: index === clampedIndex ? "gray.700" : undefined,
									}}
									data-testid={`user-option-${user.id}`}
								>
									{user.username}
								</ListItem>
							))}
						</List>
					) : (
						<Text
							px={3}
							py={2}
							color="gray.500"
							data-testid="user-picker-empty"
						>
							{isPending ? "Loading users…" : "No matching users"}
						</Text>
					)
				) : null}
			</Box>
		</FormControl>
	);
}
