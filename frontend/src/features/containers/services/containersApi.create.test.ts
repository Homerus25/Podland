import {
	buildCreateContainerInput,
	type CreateContainerFormValues,
	type MountMode,
	type PortProtocol,
} from "@features/containers/services/containersApi";
import { describe, expect, it } from "vitest";

// Unit tests for the create-container payload builder (issue 328,
// architecture-review candidate C3): the dialog's whole form-to-payload
// mapping contract, pinned here without a DOM. The DOM create suites keep
// one full-payload smoke (wiring) plus their dialog-behavior pins; the
// per-field mapping cases they used to carry are absorbed by this suite.

// The dialog's default form state: every optional field empty.
const baseValues: CreateContainerFormValues = {
	image: "docker.io/library/redis:7",
	name: "",
	command: "",
	user: "",
	timezone: "",
	restartPolicy: "",
	remove: false,
	environment: [],
	portBindings: [],
	mounts: [],
	volumes: [],
	networks: [],
	secrets: [],
	devices: [],
};

const values = (
	over: Partial<CreateContainerFormValues>,
): CreateContainerFormValues => ({ ...baseValues, ...over });

// The option list the dialog's Secrets section renders (the harness's
// fixtures): the form selects by id, the payload carries the name.
const secrets = [
	{ id: "s-1", name: "db-password" },
	{ id: "s-2", name: "api-token" },
];

describe("buildCreateContainerInput", () => {
	it("trims the image and omits every optional field on a bare form", () => {
		const input = buildCreateContainerInput(
			values({ image: "  docker.io/library/redis:7  " }),
		);

		expect(input).toEqual({ image: "docker.io/library/redis:7" });
	});

	it("splits the command on whitespace and collapses runs of it", () => {
		const input = buildCreateContainerInput(
			values({ command: "  redis-server   --appendonly   yes " }),
		);

		expect(input.command).toEqual(["redis-server", "--appendonly", "yes"]);
	});

	it("omits the command when it is empty or whitespace-only", () => {
		expect(buildCreateContainerInput(values({ command: "" }))).toEqual({
			image: "docker.io/library/redis:7",
		});
		expect(buildCreateContainerInput(values({ command: "   " }))).toEqual({
			image: "docker.io/library/redis:7",
		});
	});

	it("trims the optional name and omits it when blank", () => {
		expect(buildCreateContainerInput(values({ name: "  cache " })).name).toBe(
			"cache",
		);
		expect(buildCreateContainerInput(values({ name: "   " }))).toEqual({
			image: "docker.io/library/redis:7",
		});
	});

	it("trims user and timezone and omits them when blank", () => {
		const input = buildCreateContainerInput(
			values({ user: " 1000:1000 ", timezone: " Europe/Berlin " }),
		);

		expect(input.user).toBe("1000:1000");
		expect(input.timezone).toBe("Europe/Berlin");
		expect(
			buildCreateContainerInput(values({ user: " ", timezone: "\t" })),
		).toEqual({ image: "docker.io/library/redis:7" });
	});

	it("sends the chosen restart policy and omits it for (none)", () => {
		expect(
			buildCreateContainerInput(values({ restartPolicy: "always" }))
				.restartPolicy,
		).toBe("always");
		expect(buildCreateContainerInput(values({ restartPolicy: "" }))).toEqual({
			image: "docker.io/library/redis:7",
		});
	});

	it("sends remove only when the checkbox is checked", () => {
		expect(buildCreateContainerInput(values({ remove: true }))).toEqual({
			image: "docker.io/library/redis:7",
			remove: true,
		});
		expect(buildCreateContainerInput(values({ remove: false }))).toEqual({
			image: "docker.io/library/redis:7",
		});
	});

	it("maps filled environment rows to a record, skipping empty keys", () => {
		const input = buildCreateContainerInput(
			values({
				environment: [
					{ key: "FOO", value: "bar" },
					{ key: "  MODE  ", value: "prod" },
					// An empty key carries no variable, so the row is skipped.
					{ key: "", value: "ignored" },
					{ key: "   ", value: "also ignored" },
				],
			}),
		);

		expect(input.environment).toEqual({ FOO: "bar", MODE: "prod" });
	});

	it("omits the environment when no row has a key", () => {
		expect(
			buildCreateContainerInput(
				values({ environment: [{ key: "", value: "ignored" }] }),
			),
		).toEqual({ image: "docker.io/library/redis:7" });
	});

	it("keeps port rows whose ports both parse to integers, defaulting the protocol to tcp", () => {
		const input = buildCreateContainerInput(
			values({
				portBindings: [
					{ hostPort: "8080", containerPort: "80", protocol: "udp" },
					{ hostPort: " 9090 ", containerPort: " 90 ", protocol: "tcp" },
					// The protocol select always offers a value; an empty one
					// falls back to tcp.
					{
						hostPort: "1",
						containerPort: "2",
						protocol: "" as PortProtocol,
					},
				],
			}),
		);

		expect(input.portBindings).toEqual([
			{ hostPort: 8080, containerPort: 80, protocol: "udp" },
			{ hostPort: 9090, containerPort: 90, protocol: "tcp" },
			{ hostPort: 1, containerPort: 2, protocol: "tcp" },
		]);
	});

	it("filters port rows with an empty or non-integer port", () => {
		const input = buildCreateContainerInput(
			values({
				portBindings: [
					{ hostPort: "", containerPort: "80", protocol: "tcp" },
					{ hostPort: "8080", containerPort: "", protocol: "tcp" },
					{ hostPort: "80.5", containerPort: "80", protocol: "tcp" },
					{ hostPort: "abc", containerPort: "80", protocol: "tcp" },
					{ hostPort: "8080", containerPort: "80", protocol: "tcp" },
				],
			}),
		);

		expect(input.portBindings).toEqual([
			{ hostPort: 8080, containerPort: 80, protocol: "tcp" },
		]);
	});

	it("omits portBindings when no row is complete", () => {
		expect(
			buildCreateContainerInput(
				values({
					portBindings: [{ hostPort: "", containerPort: "", protocol: "tcp" }],
				}),
			),
		).toEqual({ image: "docker.io/library/redis:7" });
	});

	it("keeps mount rows with a source and a destination, defaulting the mode to rw", () => {
		const input = buildCreateContainerInput(
			values({
				mounts: [
					{ source: "/srv/app", destination: "/app", mode: "ro" },
					{
						source: " /srv/other ",
						destination: " /opt ",
						mode: "" as MountMode,
					},
					// An empty destination makes the row incomplete.
					{ source: "/srv/incomplete", destination: "  ", mode: "rw" },
				],
			}),
		);

		expect(input.mounts).toEqual([
			{ source: "/srv/app", destination: "/app", mode: "ro" },
			{ source: "/srv/other", destination: "/opt", mode: "rw" },
		]);
	});

	it("keeps volume rows with a name and a destination, defaulting the mode to rw", () => {
		const input = buildCreateContainerInput(
			values({
				volumes: [
					{ name: "pgdata", destination: "/data", mode: "rw" },
					// No selected name makes the row incomplete.
					{ name: "", destination: "/var/lib", mode: "ro" },
				],
			}),
		);

		expect(input.volumes).toEqual([
			{ name: "pgdata", destination: "/data", mode: "rw" },
		]);
	});

	it("omits mounts and volumes when no row is complete", () => {
		expect(
			buildCreateContainerInput(
				values({
					mounts: [{ source: "", destination: "", mode: "rw" }],
					volumes: [{ name: "", destination: "", mode: "rw" }],
				}),
			),
		).toEqual({ image: "docker.io/library/redis:7" });
	});

	it("passes the selected networks through as a string array", () => {
		const input = buildCreateContainerInput(
			values({ networks: ["app-net", "other-net"] }),
		);

		expect(input.networks).toEqual(["app-net", "other-net"]);
		expect(buildCreateContainerInput(values({ networks: [] }))).toEqual({
			image: "docker.io/library/redis:7",
		});
	});

	it("resolves selected secret ids to names with the default target path", () => {
		const input = buildCreateContainerInput(
			values({ secrets: ["s-1", "s-2"] }),
			secrets,
		);

		expect(input.secrets).toEqual([
			{ source: "db-password", target: "/run/secrets/db-password" },
			{ source: "api-token", target: "/run/secrets/api-token" },
		]);
	});

	it("skips secret ids that no longer exist in the fetched list", () => {
		const input = buildCreateContainerInput(
			values({ secrets: ["s-1", "s-gone"] }),
			secrets,
		);

		expect(input.secrets).toEqual([
			{ source: "db-password", target: "/run/secrets/db-password" },
		]);
	});

	it("omits secrets when every selected id is stale or the option list is still pending", () => {
		expect(
			buildCreateContainerInput(values({ secrets: ["s-gone"] }), secrets),
		).toEqual({ image: "docker.io/library/redis:7" });
		// The dialog tolerates a pending option fetch: no list, no secrets.
		expect(
			buildCreateContainerInput(values({ secrets: ["s-1"] }), undefined),
		).toEqual({ image: "docker.io/library/redis:7" });
	});

	it("keeps device rows with both paths and filters incomplete ones", () => {
		const input = buildCreateContainerInput(
			values({
				devices: [
					{
						hostPath: "/dev/dri/renderD128",
						containerPath: "/dev/dri/renderD128",
					},
					{ hostPath: "/dev/kvm", containerPath: "" },
					{ hostPath: "", containerPath: "/dev/kvm" },
				],
			}),
		);

		expect(input.devices).toEqual([
			{
				hostPath: "/dev/dri/renderD128",
				containerPath: "/dev/dri/renderD128",
			},
		]);
		expect(
			buildCreateContainerInput(
				values({ devices: [{ hostPath: "", containerPath: "" }] }),
			),
		).toEqual({ image: "docker.io/library/redis:7" });
	});

	it("builds the complete payload when every field is populated", () => {
		const input = buildCreateContainerInput(
			values({
				name: "cache",
				command: "redis-server --appendonly yes",
				user: "1000:1000",
				timezone: "Europe/Berlin",
				restartPolicy: "always",
				remove: true,
				environment: [
					{ key: "FOO", value: "bar" },
					{ key: "MODE", value: "prod" },
				],
				portBindings: [
					{ hostPort: "8080", containerPort: "80", protocol: "udp" },
				],
				mounts: [{ source: "/srv/app", destination: "/app", mode: "ro" }],
				volumes: [{ name: "pgdata", destination: "/data", mode: "rw" }],
				networks: ["app-net"],
				secrets: ["s-1"],
				devices: [
					{
						hostPath: "/dev/dri/renderD128",
						containerPath: "/dev/dri/renderD128",
					},
				],
			}),
			secrets,
		);

		expect(input).toEqual({
			image: "docker.io/library/redis:7",
			name: "cache",
			command: ["redis-server", "--appendonly", "yes"],
			user: "1000:1000",
			timezone: "Europe/Berlin",
			restartPolicy: "always",
			remove: true,
			environment: { FOO: "bar", MODE: "prod" },
			portBindings: [{ hostPort: 8080, containerPort: 80, protocol: "udp" }],
			mounts: [{ source: "/srv/app", destination: "/app", mode: "ro" }],
			volumes: [{ name: "pgdata", destination: "/data", mode: "rw" }],
			networks: ["app-net"],
			secrets: [{ source: "db-password", target: "/run/secrets/db-password" }],
			devices: [
				{
					hostPath: "/dev/dri/renderD128",
					containerPath: "/dev/dri/renderD128",
				},
			],
		});
	});
});
