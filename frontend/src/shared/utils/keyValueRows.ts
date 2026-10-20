// One key/value row of a field array (environment variables, labels,
// driver options): a mono key input and a mono value input.
export interface KeyValueRow {
	key: string;
	value: string;
}

// Maps filled key/value rows to a record, skipping rows with an empty key
// (an empty key carries no entry, so a half-typed row must not block
// submit). Returns undefined when no row is filled, so the payload omits
// the field entirely. Duplicate keys collapse to the last value.
//
// The mapper is pure, so it lives in shared/utils as the ritual's one
// owner: every consumer — the KeyValueEditor module's row types and the
// payload builders in the feature services modules (which never import
// components) — imports it from here.
export function keyValueRowsToRecord(
	rows: readonly KeyValueRow[],
): Record<string, string> | undefined {
	const filled = rows.filter((row) => row.key.trim() !== "");
	if (filled.length === 0) return undefined;
	return Object.fromEntries(filled.map((row) => [row.key.trim(), row.value]));
}
