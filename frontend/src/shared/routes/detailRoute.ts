// The detail-route kit owns the resource-detail navigation family (spec
// 301): the resource kind union, the one detail-path builder, and the
// machine tab path the detail pages' back button returns to. It mirrors the
// logs route kit (@shared/logs/logRoute): machine-scoped URL construction
// is single-owned and percent-encodes every segment (the C11 discipline),
// and the router decodes params on the far side, so the round trip is safe.
export type DetailResource =
	| "containers"
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
// returns to (?tab=<resource>, the spec 297 deep link); the machineId is
// percent-encoded like every machine-scoped segment.
export function machineTabPath(machineId: string, tab: string): string {
	return `/machines/${encodeURIComponent(machineId)}?tab=${tab}`;
}
