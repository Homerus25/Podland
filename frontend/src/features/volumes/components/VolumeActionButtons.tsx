import { Button, HStack } from "@chakra-ui/react";

// The volume action set, owned once (spec #477): the list row and the volume
// detail page render the same buttons, so the labels, the aria names, the
// testids, and the gating can never drift between the two surfaces. Export is
// a read and renders for every caller; Move/Copy and Populate are write-gated,
// so `canManage` false leaves Export alone — the caller decides whether that
// means a hidden group (the list's read-only row) or the shared write hint
// beside it (the detail page's denied gate).
//
// The set renders the actions its caller offers: Move/Copy is opt-in through
// `onMove` (spec #486, ticket #490), so a surface that moves some other way —
// the volumes list, whose bulk "Move/copy selected" replaces the per-row
// button — simply does not hand the handler over and the button never
// renders. The volume detail page still passes it, so its Move/Copy action is
// unchanged (spec #477 parity).
//
// Every button stops propagation: the list row is clickable (it opens the
// volume's detail page) and a button click must never also fire the row
// action.
export function VolumeActionButtons({
	volume,
	canManage,
	isExporting,
	size = "xs",
	onExport,
	onMove,
	onPopulate,
}: {
	volume: { name: string };
	canManage: boolean;
	isExporting: boolean;
	size?: "xs" | "sm";
	onExport: () => void;
	// Opt-in: a caller that offers no per-volume move passes nothing and the
	// button never renders.
	onMove?: () => void;
	onPopulate: () => void;
}) {
	return (
		<HStack spacing={2}>
			<Button
				size={size}
				variant="outline"
				aria-label={`Export ${volume.name}`}
				isLoading={isExporting}
				onClick={(event) => {
					event.stopPropagation();
					onExport();
				}}
				data-testid="export-volume-button"
			>
				Export
			</Button>
			{canManage ? (
				<>
					{onMove ? (
						<Button
							size={size}
							variant="outline"
							aria-label={`Move or copy ${volume.name}`}
							onClick={(event) => {
								event.stopPropagation();
								onMove();
							}}
							data-testid="move-volume-button"
						>
							Move/Copy
						</Button>
					) : null}
					<Button
						size={size}
						variant="outline"
						aria-label={`Populate ${volume.name}`}
						onClick={(event) => {
							event.stopPropagation();
							onPopulate();
						}}
						data-testid="populate-volume-button"
					>
						Populate
					</Button>
				</>
			) : null}
		</HStack>
	);
}
