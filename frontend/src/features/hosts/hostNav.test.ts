import { describe, expect, it } from "vitest";
import { hostNavScope, resolveHostTabKey } from "./hostNav";

describe("resolveHostTabKey", () => {
	it("passes a known tab key through", () => {
		expect(resolveHostTabKey("volumes")).toBe("volumes");
		expect(resolveHostTabKey("settings")).toBe("settings");
	});

	it("falls back to containers for a missing or unknown value", () => {
		expect(resolveHostTabKey(null)).toBe("containers");
		expect(resolveHostTabKey("bogus")).toBe("containers");
	});
});

describe("hostNavScope", () => {
	it("returns null off the host-scoped routes", () => {
		expect(hostNavScope("/", null)).toBeNull();
		expect(hostNavScope("/hosts", null)).toBeNull();
		expect(hostNavScope("/users", null)).toBeNull();
	});

	it("scopes the host detail page to the ?tab= param, defaulting to containers", () => {
		expect(hostNavScope("/hosts/m-1", null)).toEqual({
			hostId: "m-1",
			tab: "containers",
		});
		expect(hostNavScope("/hosts/m-1", "volumes")).toEqual({
			hostId: "m-1",
			tab: "volumes",
		});
		expect(hostNavScope("/hosts/m-1", "bogus")).toEqual({
			hostId: "m-1",
			tab: "containers",
		});
	});

	it.each([
		["/hosts/m-1/containers/c-1", "containers"],
		["/hosts/m-1/containers/new", "containers"],
		["/hosts/m-1/volumes/vol-1", "volumes"],
		["/hosts/m-1/images/sha256:abc123", "images"],
		["/hosts/m-1/networks/net-1", "networks"],
		["/hosts/m-1/quadlets/web", "quadlets"],
		["/hosts/m-1/pods/p-1", "pods"],
		["/hosts/m-1/secrets/s-1", "secrets"],
	] as const)(
		"highlights the matching host tab on the sub-page %s",
		(pathname, tab) => {
			expect(hostNavScope(pathname, null)).toEqual({ hostId: "m-1", tab });
		},
	);

	it.each([
		["container", "containers"],
		["quadlet", "quadlets"],
		["pod", "pods"],
	] as const)(
		"highlights the target kind's tab on the %s logs page",
		(kind, tab) => {
			expect(hostNavScope(`/hosts/m-1/logs/${kind}/t-1`, null)).toEqual({
				hostId: "m-1",
				tab,
			});
		},
	);

	it("decodes the percent-encoded host id", () => {
		expect(hostNavScope("/hosts/host%20one", null)).toEqual({
			hostId: "host one",
			tab: "containers",
		});
	});

	it("keeps a malformed escape raw instead of throwing", () => {
		expect(hostNavScope("/hosts/%zz", null)).toEqual({
			hostId: "%zz",
			tab: "containers",
		});
	});

	it("falls back to containers on an unrecognized sub-route", () => {
		expect(hostNavScope("/hosts/m-1/unknown/x", null)).toEqual({
			hostId: "m-1",
			tab: "containers",
		});
	});
});
