import { useNavigate } from "react-router-dom";

// The logs route kit owns the whole logs navigation ritual (2026-09-22
// architecture-review candidate 9): the kind union, its guard, the route
// pattern, the path builder, the typed navigation state, and the navigate
// hook. Fetching stays in the logSource kit (@shared/streaming/logSource) —
// that kit owns the log-endpoint ritual, this one the route contract. A new
// log-bearing kind is one LogKind member plus one LOG_SOURCES entry in the
// logs page; the route and path edits the old literal copies needed are gone.
export type LogKind = "container" | "quadlet" | "pod";

export function isLogKind(value: string | undefined): value is LogKind {
	return value === "container" || value === "quadlet" || value === "pod";
}

// The route shape, owned once: the app router mounts it and every feature
// logs test renderer mounts the same constant, so a shape change is a
// one-line edit and no literal copy can drift.
export const LOG_ROUTE_PATTERN = "/machines/:machineId/logs/:kind/:targetId";

// The one builder of logs paths: machineId and targetId are percent-encoded
// (the C11 machine-scoped URL discipline, extended to the one machine-scoped
// URL family it did not cover — an id containing `/` used to break the route
// match silently), and the router decodes params on the far side, so the
// round trip is safe. The kind is a typed LogKind member and rides bare.
export function logsPath(
	machineId: string,
	kind: LogKind,
	targetId: string,
): string {
	return `/machines/${encodeURIComponent(machineId)}/logs/${kind}/${encodeURIComponent(targetId)}`;
}

// What the logs page reads back from navigation state; the typed cast there
// replaces its old untyped one. The display name is optional and falls back
// to the raw target id on the page.
export interface LogsPageState {
	name?: string;
}

// Opens the logs page for one target of a fixed kind. The kind is bound once
// per feature at the hook call (a typo is a compile error, and each list
// feature mentions its kind exactly once); open() builds the path through
// logsPath and carries the typed navigation state.
export function useOpenLogs(machineId: string, kind: LogKind) {
	const navigate = useNavigate();
	return ({ targetId, name }: { targetId: string; name?: string }) => {
		navigate(logsPath(machineId, kind, targetId), {
			state: { name } satisfies LogsPageState,
		});
	};
}
