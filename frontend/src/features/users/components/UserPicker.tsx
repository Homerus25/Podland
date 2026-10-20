import { FormControl, FormLabel } from "@chakra-ui/react";
import { useUsers } from "@features/users/hooks/useUsers";
import type { UserSummary } from "@features/users/services/usersApi";
import { ErrorDisplay } from "@shared/components/ErrorDisplay";
import { SearchableSelect } from "@shared/components/SearchableSelect";
import { getApiError } from "@shared/utils/api";
import { useId, useMemo } from "react";

interface UserPickerProps {
	// The currently picked user, or null while nothing is picked. The
	// chip is the selection; the search input is only the query, so the
	// two can never disagree.
	value: UserSummary | null;
	onChange: (user: UserSummary | null) => void;
	// Users the picker must not offer (e.g. the machine owner and users
	// who already hold a grant on the share form).
	excludeUserIds?: string[];
}

// Username picker over the users directory, a thin adapter over the shared
// combobox core: the directory fetch, the exclusion pre-filter, and the
// field chrome (form label, error display) are the users feature's, while
// selection display, keyboard behavior, and ARIA all come from the core.
// The options carry the full UserSummary as their value, so the core's
// selection is handed to the caller unchanged — consumers submit the
// user's id without ever parsing the text input.
export function UserPicker({
	value,
	onChange,
	excludeUserIds,
}: UserPickerProps) {
	const { data: users, isPending, error } = useUsers();
	const inputId = useId().replace(/:/g, "");

	// Exclusion is adapter-side pre-filtering: the core already treats
	// "not in options" correctly.
	const options = useMemo(() => {
		const excluded = new Set(excludeUserIds);
		return (users ?? [])
			.filter((user) => !excluded.has(user.id))
			.map((user) => ({ value: user, label: user.username }));
	}, [users, excludeUserIds]);

	return (
		<FormControl>
			<FormLabel htmlFor={inputId}>User</FormLabel>
			{error ? <ErrorDisplay error={getApiError(error)} /> : null}
			<SearchableSelect
				id={inputId}
				mode="single"
				options={options}
				value={value}
				onChange={onChange}
				// The directory refetch rebuilds the option objects, so the
				// selection compares by the user's id — a held summary
				// stays selected across refetches instead of going stale.
				keyOf={(user) => user.id}
				searchPlaceholder="Search users…"
				emptyMessage={isPending ? "Loading users…" : "No matching users"}
			/>
		</FormControl>
	);
}
