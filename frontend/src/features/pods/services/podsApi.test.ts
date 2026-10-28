import {
	buildCreatePodInput,
	type PodFormValues,
} from "@features/pods/services/podsApi";
import { describe, expect, it } from "vitest";

// The create-pod payload builder's unit tests (spec 352 ticket #355): the
// form-to-payload mapping is a pure builder in the services module, tested
// here without a DOM (the buildCreateContainerInput precedent, C3).

const baseValues: PodFormValues = {
	name: "  app-pod  ",
	networks: ["app-net", "db-net"],
	volumes: [{ name: "app-data", destination: "/data", mode: "rw" }],
	mounts: [
		{
			source: "/srv/data",
			destination: "/srv",
			mode: "ro",
			selinuxShared: true,
		},
	],
	labels: [
		{ key: "app", value: "web" },
		{ key: "tier", value: "frontend" },
	],
	portMappings: [{ hostPort: "8080", containerPort: "80", protocol: "udp" }],
	cpus: "1.5",
	memory: " 512m ",
};

describe("buildCreatePodInput", () => {
	it("maps the full form: trimmed name, networks, storage, labels, limits", () => {
		expect(buildCreatePodInput(baseValues)).toEqual({
			name: "app-pod",
			networks: ["app-net", "db-net"],
			volumes: [{ name: "app-data", destination: "/data", mode: "rw" }],
			mounts: [
				{
					source: "/srv/data",
					destination: "/srv",
					mode: "ro",
					selinuxShared: true,
				},
			],
			labels: { app: "web", tier: "frontend" },
			portMappings: [{ hostPort: 8080, containerPort: 80, protocol: "udp" }],
			cpus: 1.5,
			memory: "512m",
		});
	});

	it("trims the name and memory, and parses cpus to a float", () => {
		const input = buildCreatePodInput({
			...baseValues,
			name: "  padded  ",
			cpus: " 2 ",
			memory: "  1g ",
		});
		expect(input.name).toBe("padded");
		expect(input.cpus).toBe(2);
		expect(input.memory).toBe("1g");
	});

	it("filters incomplete volume and mount rows and defaults their mode to rw", () => {
		const input = buildCreatePodInput({
			...baseValues,
			volumes: [
				{ name: "", destination: "/data", mode: "" },
				{ name: "app-data", destination: "", mode: "" },
				{ name: "app-data", destination: "/data", mode: "" },
			],
			mounts: [
				{ source: "", destination: "/srv", mode: "" },
				{ source: "/srv/data", destination: "", mode: "" },
				{ source: "/srv/data", destination: "/srv", mode: "" },
			],
		});
		expect(input.volumes).toEqual([
			{ name: "app-data", destination: "/data", mode: "rw" },
		]);
		expect(input.mounts).toEqual([
			{ source: "/srv/data", destination: "/srv", mode: "rw" },
		]);
	});

	it("filters incomplete port-mapping rows and defaults the protocol to tcp", () => {
		const input = buildCreatePodInput({
			...baseValues,
			portMappings: [
				{ hostPort: "", containerPort: "80", protocol: "tcp" },
				{ hostPort: "8080", containerPort: "", protocol: "tcp" },
				{ hostPort: "not-a-port", containerPort: "80", protocol: "tcp" },
				// The protocol select always offers a value; an empty one
				// falls back to tcp.
				{
					hostPort: "8080",
					containerPort: "80",
					protocol: "" as "tcp" | "udp",
				},
			],
		});
		expect(input.portMappings).toEqual([
			{ hostPort: 8080, containerPort: 80, protocol: "tcp" },
		]);
	});

	it("keeps the mount flags only when checked", () => {
		const input = buildCreatePodInput({
			...baseValues,
			mounts: [
				{
					source: "/srv/data",
					destination: "/srv",
					mode: "rw",
					selinuxPrivate: true,
					chownUser: true,
				},
			],
		});
		expect(input.mounts).toEqual([
			{
				source: "/srv/data",
				destination: "/srv",
				mode: "rw",
				selinuxPrivate: true,
				chownUser: true,
			},
		]);
	});

	it("omits empty collections and limits", () => {
		const input = buildCreatePodInput({
			name: "app-pod",
			networks: [],
			volumes: [],
			mounts: [],
			labels: [],
			portMappings: [],
			cpus: "",
			memory: "",
		});
		expect(input).toEqual({ name: "app-pod" });
		expect("networks" in input).toBe(false);
		expect("volumes" in input).toBe(false);
		expect("mounts" in input).toBe(false);
		expect("labels" in input).toBe(false);
		expect("portMappings" in input).toBe(false);
		expect("cpus" in input).toBe(false);
		expect("memory" in input).toBe(false);
	});

	it("drops an unparseable cpus value; memory travels verbatim (trimmed)", () => {
		const input = buildCreatePodInput({
			...baseValues,
			cpus: "not-a-number",
			memory: " 1g ",
		});
		expect("cpus" in input).toBe(false);
		expect(input.memory).toBe("1g");
	});

	it("collapses duplicate label keys to the last value", () => {
		const input = buildCreatePodInput({
			...baseValues,
			labels: [
				{ key: "app", value: "web" },
				{ key: "app", value: "api" },
			],
		});
		expect(input.labels).toEqual({ app: "api" });
	});
});
