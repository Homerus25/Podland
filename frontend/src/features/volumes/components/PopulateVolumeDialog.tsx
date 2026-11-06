import {
	Box,
	Button,
	FormControl,
	FormLabel,
	Text,
	useToast,
} from "@chakra-ui/react";
import type { usePopulateVolume } from "@features/volumes/hooks/useVolumes";
import {
	buildPopulateForm,
	type VolumeSummary,
} from "@features/volumes/services/volumesApi";
import { ErrorDisplay } from "@shared/components/ErrorDisplay";
import { MutationDialog } from "@shared/components/MutationDialog";
import { useState } from "react";

const UPLOAD_TOO_LARGE_CODE = "VOLUME_UPLOAD_TOO_LARGE";

// The populate adapter over the core, shared by the volumes list's row
// action and the volume detail page's header action (spec 477, ticket
// #482): one dialog body, two entry points. The body keeps its local state:
// the files input or the tar archive with the shape-conflict guard, and the
// payload building stays a pure function in the API module. The custom
// footer expresses the confirm-populate-button with its progress text while
// pending through the core's footer mechanism. The too-large failure maps
// to its hint through renderError - the seam replaces the whole region, so
// the adapter renders both branches itself; success toasts via onSuccess.
//
// The mutation arrives as a prop because the caller owns it: the list keeps
// one instance for every row, and the detail page owns its own.
export function PopulateVolumeDialog({
	volume,
	onClose,
	populateVolume,
}: {
	volume: VolumeSummary;
	onClose: () => void;
	populateVolume: ReturnType<typeof usePopulateVolume>;
}) {
	const [files, setFiles] = useState<FileList | null>(null);
	const [archive, setArchive] = useState<File | null>(null);
	const [progress, setProgress] = useState(0);
	const toast = useToast();

	const hasFiles = files !== null && files.length > 0;
	const hasArchive = archive !== null;
	const shapeConflict = hasFiles && hasArchive;
	const canSubmit = (hasFiles || hasArchive) && !shapeConflict;

	const submit = () => {
		if (!canSubmit) return;
		setProgress(0);
		const form = buildPopulateForm(
			hasFiles ? Array.from(files!) : [],
			hasArchive ? archive : undefined,
		);
		populateVolume.mutate({
			volumeName: volume.name,
			form,
			onProgress: setProgress,
		});
	};

	return (
		<MutationDialog
			mutation={populateVolume}
			title={`Populate ${volume.name}`}
			onClose={onClose}
			onSuccess={() =>
				toast({
					title: `Volume populated: ${volume.name}`,
					status: "success",
					isClosable: true,
				})
			}
			renderError={(apiError) =>
				apiError.code === UPLOAD_TOO_LARGE_CODE ? (
					<>
						<ErrorDisplay error={apiError} />
						<Text
							mt={2}
							color="orange.500"
							data-testid="populate-too-large-hint"
						>
							The upload exceeds the configured size limit. Split or shrink it
							and try again.
						</Text>
					</>
				) : (
					<ErrorDisplay error={apiError} />
				)
			}
			footer={
				<>
					<Button variant="ghost" onClick={onClose}>
						Cancel
					</Button>
					<Button
						colorScheme="blue"
						onClick={submit}
						isDisabled={!canSubmit}
						isLoading={populateVolume.isPending}
						loadingText={`Populating ${Math.round(progress * 100)}%`}
						data-testid="confirm-populate-button"
					>
						Upload and populate
					</Button>
				</>
			}
			body={
				<>
					<Text>
						Populate merges into the volume: uploaded paths overwrite same-named
						files, everything already in the volume is kept.
					</Text>
					<FormControl mt={3}>
						<FormLabel htmlFor="populate-files">Files</FormLabel>
						<Box data-testid="populate-files-input">
							<input
								id="populate-files"
								type="file"
								multiple
								onChange={(event) => setFiles(event.target.files)}
							/>
						</Box>
					</FormControl>
					<FormControl mt={3}>
						<FormLabel htmlFor="populate-archive">Or a tar archive</FormLabel>
						<Box data-testid="populate-archive-input">
							<input
								id="populate-archive"
								type="file"
								accept=".tar"
								onChange={(event) =>
									setArchive(event.target.files?.[0] ?? null)
								}
							/>
						</Box>
					</FormControl>
					{shapeConflict ? (
						<Text
							color="orange.500"
							mt={2}
							data-testid="populate-shape-conflict"
						>
							Choose either files or a single tar archive, not both.
						</Text>
					) : null}
				</>
			}
		/>
	);
}
