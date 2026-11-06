import { isLogKind, type LogKind } from "@shared/logs/logRoute";
import { useLocation, useSearchParams } from "react-router-dom";

// The host navigation vocabulary (issue #459): the eight areas of a host
// that the sidebar's host section lists and the header's host identity
// scopes. The order is the deep-link order — the ?tab= param resolves
// against these keys, and the host detail page renders the matching panel.
export const HOST_TABS = [
	{ key: "containers", label: "Containers" },
	{ key: "volumes", label: "Volumes" },
	{ key: "images", label: "Images" },
	{ key: "networks", label: "Networks" },
	{ key: "quadlets", label: "Quadlets" },
	{ key: "pods", label: "Pods" },
	{ key: "secrets", label: "Secrets" },
	{ key: "settings", label: "Settings" },
] as const;

export type HostTabKey = (typeof HOST_TABS)[number]["key"];

// Resolves the ?tab= deep-link param; an unknown or missing value falls
// back to Containers.
export function resolveHostTabKey(tabParam: string | null): HostTabKey {
	return HOST_TABS.find((tab) => tab.key === tabParam)?.key ?? "containers";
}

// A resource sub-route names its own tab; the create-container page
// (containers/new) rides the containers entry like the detail pages.
const SUB_ROUTE_TABS: Record<string, HostTabKey> = {
	containers: "containers",
	volumes: "volumes",
	images: "images",
	networks: "networks",
	quadlets: "quadlets",
	pods: "pods",
	secrets: "secrets",
};

// The logs page highlights the tab matching its target kind.
const LOG_KIND_TABS: Record<LogKind, HostTabKey> = {
	container: "containers",
	quadlet: "quadlets",
	pod: "pods",
};

export interface HostNavScope {
	hostId: string;
	tab: HostTabKey;
}

// The router hands params decoded, but this derivation reads the raw
// pathname; a malformed escape (%zz) must not throw — the shell renders
// through this on every navigation, so the raw segment wins over a crash.
function decodeSegment(segment: string): string {
	try {
		return decodeURIComponent(segment);
	} catch {
		return segment;
	}
}

// Derives the host scope from a location: null off the host-scoped
// routes, otherwise the host id plus the tab the navigation should
// highlight — the ?tab= param on the host detail page, the sub route's
// resource on detail and create pages, the target kind on the logs
// page. The header's host identity and the sidebar's host section both
// read through this one function, so the two surfaces cannot disagree.
export function hostNavScope(
	pathname: string,
	tabParam: string | null,
): HostNavScope | null {
	const segments = pathname
		.split("/")
		.filter(Boolean)
		.map(decodeSegment);
	if (segments[0] !== "hosts" || segments[1] === undefined) {
		return null;
	}
	const hostId = segments[1];
	const [first, second] = segments.slice(2);
	if (first === undefined) {
		return { hostId, tab: resolveHostTabKey(tabParam) };
	}
	if (first === "logs" && isLogKind(second)) {
		return { hostId, tab: LOG_KIND_TABS[second] };
	}
	return { hostId, tab: SUB_ROUTE_TABS[first] ?? "containers" };
}

// The hook form the shell surfaces consume: the current location's
// scope, recomputed on every navigation.
export function useHostNavScope(): HostNavScope | null {
	const { pathname } = useLocation();
	const [searchParams] = useSearchParams();
	return hostNavScope(pathname, searchParams.get("tab"));
}
