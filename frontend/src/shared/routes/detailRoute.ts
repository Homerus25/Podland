// The detail-route kit owns the resource-detail navigation family (spec
// 301): the resource kind union, the one detail-path builder, the machine
// tab path the detail pages' back button falls back to, and the
// history-awareness predicate that back button rides. It mirrors the logs
// route kit (@shared/logs/logRoute): machine-scoped URL construction is
// single-owned and percent-encodes every segment (the C11 discipline),
// and the router decodes params on the far side, so the round trip is safe.
import { useLocation } from "react-router-dom";
export type DetailResource =
	| "containers"
	| "pods"
	| "volumes"
	| "images"
	| "networks"
	| "secrets";

// The one builder of resource detail paths: machineId and id are
// percent-encoded (an id containing `/` used to break the route match
// silently), the resource rides bare as the route's path segment.
export function detailPath(
	machineId: string,
	resource: DetailResource,
	id: string,
): string {
	return `/machines/${encodeURIComponent(machineId)}/${resource}/${encodeURIComponent(id)}`;
}

// The machine detail page path with the tab the detail page's back button
// falls back to (?tab=<resource>, the spec 297 deep link); the machineId is
// percent-encoded like every machine-scoped segment.
export function machineTabPath(machineId: string, tab: string): string {
	return `/machines/${encodeURIComponent(machineId)}?tab=${tab}`;
}

// The router contract the history-aware back button rides (spec 373): a
// history's initial entry — a deep link, bookmark, or fresh tab — carries
// the key "default" (both browser and memory history), so any other key
// means the session navigated to the current location in-app and history
// back returns to the referring page.
export function useHasInAppHistory(): boolean {
	const { key } = useLocation();
	return key !== "default";
}

// The create-container page's path (spec 341): the one builder of the
// containers area's create route, percent-encoded like every
// machine-scoped segment. The static `new` segment ranks above the
// `:containerId` detail segment in the router, so both routes coexist.
export function containerCreatePath(machineId: string): string {
	return `/machines/${encodeURIComponent(machineId)}/containers/new`;
}
