import {
	anyVolumeExistsOnTarget,
	companionNote,
	hasMigratableVolume,
	isSecretTransferOffered,
	kindLabel,
	MoveQuadletWarnings,
	moveCompanionNames,
	moveCompanions,
	moveDifferingCompanions,
	movePreviewSecrets,
} from "@features/quadlets/components/MoveQuadletWarnings";
import type {
	QuadletMovePreviewSecret,
	QuadletMovePreviewVolume,
} from "@features/quadlets/services/quadletsApi";
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { type ComponentProps, useState } from "react";
import { describe, expect, it, vi } from "vitest";
import {
	hostFixture,
	quadletMoveCompanionVolumeFixture,
	quadletMoveDataVolumeFixture,
	quadletMovePreviewFixture,
	quadletMovePreviewSecretFixture,
} from "@/test/factories";
import { renderWithQuery } from "@/test/render";

// The module takes data props only — no queries, no hooks — so its suite
// pins the projections as pure functions and the rendering straight from
// props, without the list harness or MSW.
type WarningsProps = ComponentProps<typeof MoveQuadletWarnings>;

const hostPathVolume: QuadletMovePreviewVolume = {
	source: "/srv/data",
	kind: "hostPath",
	existsOnTarget: false,
	migratable: false,
	reason: "host bind paths are not migrated",
};

function baseProps(overrides: Partial<WarningsProps> = {}): WarningsProps {
	const volumes = overrides.volumes ?? quadletMovePreviewFixture().volumes;
	return {
		volumes,
		// The adapter hands the projection over; the suite derives it the
		// way the adapter does.
		differsCompanions: moveDifferingCompanions(moveCompanions(volumes)),
		secrets: [],
		sourceName: "prod-web-01",
		targetName: "staging-01",
		transferCompanions: true,
		volumesAcknowledged: false,
		onAcknowledge: () => {},
		...overrides,
	};
}

function renderWarnings(overrides: Partial<WarningsProps> = {}) {
	const props = baseProps(overrides);
	const result = renderWithQuery(<MoveQuadletWarnings {...props} />, {
		chakra: true,
	});
	return { ...result, props };
}

describe("kindLabel", () => {
	it("spells a host path out and labels store kinds with the store name", () => {
		expect(kindLabel("volume", "podman volume")).toBe("podman volume");
		expect(kindLabel("secret", "podman secret")).toBe("podman secret");
		expect(kindLabel("hostPath", "podman volume")).toBe("host path");
	});
});

describe("companionNote", () => {
	const companion = quadletMoveCompanionVolumeFixture("absent");

	it("describes the target file regardless of the transfer option", () => {
		expect(
			companionNote(quadletMoveCompanionVolumeFixture("identical"), true),
		).toBe("already present on target");
		expect(
			companionNote(quadletMoveCompanionVolumeFixture("differs"), false),
		).toBe("differs on target");
	});

	it("answers the default transfer only for an absent companion", () => {
		expect(companionNote(companion, true)).toBe("definition will be copied");
		expect(companionNote(companion, false)).toBe(
			"definition will not be transferred",
		);
	});
});

describe("moveCompanions", () => {
	it("keeps the entries the preview marked with a quadlet file, in order", () => {
		const volumes = [
			quadletMoveDataVolumeFixture(),
			quadletMoveCompanionVolumeFixture("absent"),
			hostPathVolume,
			quadletMoveCompanionVolumeFixture("identical", "db.volume"),
		];
		const companions = moveCompanions(volumes);
		expect(companions.map((volume) => volume.quadletFile)).toEqual([
			"app-data.volume",
			"db.volume",
		]);
	});

	it("returns an empty list when no reference names a companion", () => {
		expect(
			moveCompanions([quadletMoveDataVolumeFixture(), hostPathVolume]),
		).toEqual([]);
	});
});

describe("moveDifferingCompanions", () => {
	it("keeps only the companions that differ on the target", () => {
		const companions = moveCompanions([
			quadletMoveCompanionVolumeFixture("absent"),
			quadletMoveCompanionVolumeFixture("identical", "db.volume"),
			quadletMoveCompanionVolumeFixture("differs", "cache.volume"),
		]);
		expect(
			moveDifferingCompanions(companions).map((volume) => volume.quadletFile),
		).toEqual(["cache.volume"]);
	});
});

describe("moveCompanionNames", () => {
	it("collects the unique companion file names, first-seen order", () => {
		const companions = moveCompanions([
			quadletMoveCompanionVolumeFixture("absent"),
			quadletMoveCompanionVolumeFixture("identical", "db.volume"),
			quadletMoveCompanionVolumeFixture("differs", "app-data.volume"),
		]);
		expect(moveCompanionNames(companions)).toEqual([
			"app-data.volume",
			"db.volume",
		]);
	});
});

describe("hasMigratableVolume", () => {
	it("is true when any reference's data can follow", () => {
		expect(
			hasMigratableVolume([quadletMoveDataVolumeFixture(), hostPathVolume]),
		).toBe(true);
	});

	it("is false when nothing is migratable", () => {
		expect(
			hasMigratableVolume([
				quadletMoveDataVolumeFixture({
					migratable: false,
					reason: "source volume does not exist",
				}),
				hostPathVolume,
			]),
		).toBe(false);
	});
});

describe("anyVolumeExistsOnTarget", () => {
	it("is true when any reference already exists on the target", () => {
		expect(
			anyVolumeExistsOnTarget([
				quadletMoveDataVolumeFixture({ existsOnTarget: true }),
				hostPathVolume,
			]),
		).toBe(true);
	});

	it("is false on a fresh target", () => {
		expect(
			anyVolumeExistsOnTarget([quadletMoveDataVolumeFixture(), hostPathVolume]),
		).toBe(false);
	});
});

describe("movePreviewSecrets", () => {
	it("defaults to an empty list when the preview has not resolved", () => {
		expect(movePreviewSecrets(undefined)).toEqual([]);
	});

	it("returns the preview's secret references", () => {
		const secrets: QuadletMovePreviewSecret[] = [
			quadletMovePreviewSecretFixture(),
		];
		expect(
			movePreviewSecrets({ ...quadletMovePreviewFixture(), secrets }),
		).toBe(secrets);
	});
});

describe("isSecretTransferOffered", () => {
	const transferable = quadletMovePreviewSecretFixture();

	it("is offered to an admin caller when a transferable secret exists", () => {
		expect(isSecretTransferOffered(hostFixture(), [transferable])).toBe(true);
	});

	it("is not offered when no secret is transferable", () => {
		const secrets = [
			quadletMovePreviewSecretFixture({ name: "ghost", existsOnSource: false }),
			quadletMovePreviewSecretFixture({
				name: "/etc/secrets/api.key",
				kind: "hostPath" as const,
				existsOnSource: false,
				existsOnTarget: false,
			}),
		];
		expect(isSecretTransferOffered(hostFixture(), secrets)).toBe(false);
	});

	it("is not offered to a caller without the reveal-secrets capability", () => {
		const grantee = hostFixture({ permission: "write", isOwner: false });
		expect(isSecretTransferOffered(grantee, [transferable])).toBe(false);
	});
});

describe("MoveQuadletWarnings rendering", () => {
	it("renders nothing when the preview reports no volumes and no secrets", () => {
		renderWarnings({ volumes: [], secrets: [] });
		expect(
			screen.queryByTestId("move-companion-conflict-warning"),
		).not.toBeInTheDocument();
		expect(screen.queryByTestId("move-volume-warning")).not.toBeInTheDocument();
		expect(screen.queryByTestId("move-secret-warning")).not.toBeInTheDocument();
	});

	it("warns about a differing companion, naming the files and the target", () => {
		renderWarnings({
			volumes: [
				quadletMoveCompanionVolumeFixture("differs", "app-data.volume"),
				quadletMoveCompanionVolumeFixture("differs", "cache.volume"),
				hostPathVolume,
			],
		});

		const warning = screen.getByTestId("move-companion-conflict-warning");
		expect(warning).toHaveTextContent("app-data.volume");
		expect(warning).toHaveTextContent("cache.volume");
		expect(warning).toHaveTextContent("staging-01");
		expect(warning).toHaveTextContent("different content");
	});

	it("stays silent about companions that do not differ on the target", () => {
		renderWarnings({
			volumes: [
				quadletMoveCompanionVolumeFixture("absent"),
				quadletMoveCompanionVolumeFixture("identical", "db.volume"),
				hostPathVolume,
			],
		});

		expect(
			screen.queryByTestId("move-companion-conflict-warning"),
		).not.toBeInTheDocument();
	});

	it("lists volumes with their kind labels, naming the source host", () => {
		renderWarnings();

		const warning = screen.getByTestId("move-volume-warning");
		expect(warning).toHaveTextContent(
			"This quadlet references volumes. Volume data does not move with the file — it stays behind on prod-web-01:",
		);
		expect(warning).toHaveTextContent("pgdata");
		expect(warning).toHaveTextContent("podman volume");
		expect(warning).toHaveTextContent("/srv/data");
		expect(warning).toHaveTextContent("host path");
	});

	it("annotates companion entries in the volume list", () => {
		renderWarnings({
			volumes: [
				quadletMoveCompanionVolumeFixture("absent"),
				quadletMoveCompanionVolumeFixture("identical", "db.volume"),
				quadletMoveCompanionVolumeFixture("differs", "cache.volume"),
				hostPathVolume,
			],
		});

		const rows = screen.getAllByTestId("move-volume-row");
		expect(rows[0]).toHaveTextContent("definition will be copied");
		expect(rows[1]).toHaveTextContent("already present on target");
		expect(rows[2]).toHaveTextContent("differs on target");
		expect(
			within(rows[3]).queryByTestId("move-companion-note"),
		).not.toBeInTheDocument();
	});

	it("answers the companion note to the transfer option", () => {
		renderWarnings({
			volumes: [quadletMoveCompanionVolumeFixture("absent")],
			transferCompanions: false,
		});

		expect(screen.getByTestId("move-companion-note")).toHaveTextContent(
			"definition will not be transferred",
		);
	});

	it("renders the volume data facts per entry (issue 214)", () => {
		renderWarnings({
			volumes: [
				quadletMoveDataVolumeFixture({
					sizeBytes: 1572864,
					existsOnTarget: true,
					targetSizeBytes: 2048,
				}),
				quadletMoveDataVolumeFixture({
					source: "ghost",
					podmanVolume: "ghost",
					migratable: false,
					reason: "source volume does not exist",
				}),
				hostPathVolume,
			],
		});

		const rows = screen.getAllByTestId("move-volume-row");
		expect(
			within(rows[0]).getByTestId("move-volume-resolved"),
		).toHaveTextContent("resolves to “pgdata” (1.6 MB)");
		expect(within(rows[0]).getByTestId("move-volume-target")).toHaveTextContent(
			"already on target (2 KB)",
		);
		expect(
			within(rows[1]).getByTestId("move-volume-resolved"),
		).toHaveTextContent("resolves to “ghost”");
		expect(within(rows[1]).getByTestId("move-volume-target")).toHaveTextContent(
			"new on target",
		);
		expect(within(rows[1]).getByTestId("move-volume-reason")).toHaveTextContent(
			"source volume does not exist",
		);
		// A host path claims nothing: its reason only.
		expect(
			within(rows[2]).queryByTestId("move-volume-resolved"),
		).not.toBeInTheDocument();
		expect(
			within(rows[2]).queryByTestId("move-volume-target"),
		).not.toBeInTheDocument();
		expect(within(rows[2]).getByTestId("move-volume-reason")).toHaveTextContent(
			"host bind paths are not migrated",
		);
	});

	it("renders the stranded-data acknowledgement from props and reports changes", async () => {
		const user = userEvent.setup();
		const onAcknowledge = vi.fn();
		// The adapter owns the acknowledgement state; the harness wires it
		// the way MoveQuadletDialog does, so the prop-in/change-out
		// contract is pinned through the real controlled shape.
		function Harness() {
			const [acknowledged, setAcknowledged] = useState(false);
			return (
				<MoveQuadletWarnings
					{...baseProps({
						onAcknowledge: (next) => {
							onAcknowledge(next);
							setAcknowledged(next);
						},
						volumesAcknowledged: acknowledged,
					})}
				/>
			);
		}
		renderWithQuery(<Harness />, { chakra: true });

		const checkbox = screen.getByRole("checkbox", {
			name: "I understand this data stays behind",
		});
		expect(checkbox).not.toBeChecked();
		await user.click(checkbox);
		expect(onAcknowledge).toHaveBeenCalledWith(true);

		const acknowledged = screen.getByRole("checkbox", {
			name: "I understand this data stays behind",
		});
		expect(acknowledged).toBeChecked();
		await user.click(acknowledged);
		expect(onAcknowledge).toHaveBeenCalledWith(false);
		expect(
			screen.getByRole("checkbox", {
				name: "I understand this data stays behind",
			}),
		).not.toBeChecked();
	});

	it("lists secrets with their kind and existence facts (issue 222)", () => {
		renderWarnings({
			secrets: [
				quadletMovePreviewSecretFixture(),
				quadletMovePreviewSecretFixture({
					name: "tls-cert",
					existsOnTarget: true,
				}),
				quadletMovePreviewSecretFixture({
					name: "ghost",
					existsOnSource: false,
				}),
				quadletMovePreviewSecretFixture({
					name: "/etc/secrets/api.key",
					kind: "hostPath" as const,
					existsOnSource: false,
					existsOnTarget: false,
				}),
			],
		});

		const warning = screen.getByTestId("move-secret-warning");
		expect(warning).toHaveTextContent(
			"This quadlet references secrets. Secret values do not move with the file — they stay behind on prod-web-01:",
		);
		const rows = screen.getAllByTestId("move-secret-row");
		// Transferable: a podman secret, fresh on the target.
		expect(rows[0]).toHaveTextContent("db-password");
		expect(rows[0]).toHaveTextContent("podman secret");
		expect(
			within(rows[0]).queryByTestId("move-secret-source"),
		).not.toBeInTheDocument();
		expect(within(rows[0]).getByTestId("move-secret-target")).toHaveTextContent(
			"new on target",
		);
		// Same name already on the target: the stranded-dependency warning.
		expect(within(rows[1]).getByTestId("move-secret-target")).toHaveTextContent(
			"already on target",
		);
		// Dangling: renders honestly as missing here.
		expect(within(rows[2]).getByTestId("move-secret-source")).toHaveTextContent(
			"missing on this host",
		);
		// A host path claims nothing: podman reads it off the filesystem.
		expect(
			within(rows[3]).queryByTestId("move-secret-source"),
		).not.toBeInTheDocument();
		expect(
			within(rows[3]).queryByTestId("move-secret-target"),
		).not.toBeInTheDocument();
	});

	it("renders only the volume warning when there are no secrets", () => {
		renderWarnings({ secrets: [] });

		expect(screen.getByTestId("move-volume-warning")).toBeInTheDocument();
		expect(screen.queryByTestId("move-secret-warning")).not.toBeInTheDocument();
	});

	it("renders only the secret warning when there are no volumes", () => {
		renderWarnings({
			volumes: [],
			secrets: [quadletMovePreviewSecretFixture()],
		});

		expect(screen.queryByTestId("move-volume-warning")).not.toBeInTheDocument();
		expect(screen.getByTestId("move-secret-warning")).toBeInTheDocument();
	});
});
