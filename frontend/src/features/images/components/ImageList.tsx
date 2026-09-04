import {
	Box,
	Button,
	Flex,
	Table,
	TableContainer,
	Tbody,
	Td,
	Text,
	Th,
	Thead,
	Tr,
} from "@chakra-ui/react";
import { PullImageForm } from "@features/images/components/PullImageForm";
import {
	useDeleteImage,
	useImages,
	usePruneImages,
} from "@features/images/hooks/useImages";
import type {
	ImagePruneReport,
	ImageSummary,
} from "@features/images/services/imagesApi";
import {
	canManageImages,
	type EffectivePermission,
} from "@features/machines/permissions";
import { ErrorDisplay } from "@shared/components/ErrorDisplay";
import { Modal } from "@shared/components/Modal";
import { getApiError } from "@shared/utils/api";
import {
	formatBytes,
	formatRelativeDate,
	shortImageId,
} from "@shared/utils/format";
import { useState } from "react";

function imageTags(image: ImageSummary): string[] {
	const tags = (image.repoTags ?? []).filter((tag) => tag !== "<none>:<none>");
	return tags.length > 0 ? tags : ["<none>"];
}

function displayTags(image: ImageSummary): string {
	return imageTags(image).join(", ");
}

const USED_BY_CONTAINERS_CODE = "IMAGE_USED_BY_CONTAINERS";

function isUsedByContainers(error: unknown): boolean {
	return getApiError(error).code === USED_BY_CONTAINERS_CODE;
}

function DeleteImageModal({
	image,
	onClose,
	deleteImage,
}: {
	image: ImageSummary;
	onClose: () => void;
	deleteImage: ReturnType<typeof useDeleteImage>;
}) {
	const tags = displayTags(image);
	const inUse = isUsedByContainers(deleteImage.error);

	const requestDelete = (force: boolean) => {
		deleteImage.mutate({ imageId: image.id, force }, { onSuccess: onClose });
	};

	return (
		<Modal
			isOpen
			onClose={onClose}
			title="Delete image"
			footer={
				<>
					<Button variant="ghost" onClick={onClose}>
						Cancel
					</Button>
					{inUse ? null : (
						<Button
							colorScheme="red"
							onClick={() => requestDelete(false)}
							isLoading={deleteImage.isPending}
							data-testid="confirm-delete-button"
						>
							Delete image
						</Button>
					)}
					{inUse ? (
						<Button
							colorScheme="red"
							variant="outline"
							onClick={() => requestDelete(true)}
							isLoading={deleteImage.isPending}
							data-testid="force-delete-button"
						>
							Force delete
						</Button>
					) : null}
				</>
			}
		>
			<Text>
				Delete {tags} ({shortImageId(image.id)}) from this machine? This cannot
				be undone.
			</Text>
			{inUse ? (
				<Text mt={2} color="orange.500" data-testid="image-in-use-warning">
					One or more containers use this image, so it cannot be deleted
					normally. Force delete removes the image even though containers still
					use it; those containers may stop working.
				</Text>
			) : null}
			{deleteImage.error && !inUse ? (
				<ErrorDisplay error={getApiError(deleteImage.error)} />
			) : null}
		</Modal>
	);
}

function PruneReportList({ report }: { report: ImagePruneReport[] }) {
	if (report.length === 0) {
		return (
			<Text data-testid="prune-report-empty">
				Nothing to prune: no unused images on this machine.
			</Text>
		);
	}
	return (
		<TableContainer>
			<Table data-testid="prune-report" size="sm">
				<Thead>
					<Tr>
						<Th>Removed image</Th>
						<Th isNumeric>Reclaimed</Th>
					</Tr>
				</Thead>
				<Tbody>
					{report.map((entry) => (
						<Tr key={entry.id} data-testid="prune-report-row">
							<Td fontFamily="mono">{shortImageId(entry.id)}</Td>
							{entry.error ? (
								<Td color="red.500">{entry.error}</Td>
							) : (
								<Td isNumeric>{formatBytes(entry.size)}</Td>
							)}
						</Tr>
					))}
				</Tbody>
			</Table>
		</TableContainer>
	);
}

function PruneImagesModal({
	onClose,
	pruneImages,
}: {
	onClose: () => void;
	pruneImages: ReturnType<typeof usePruneImages>;
}) {
	const report = pruneImages.data;
	return (
		<Modal
			isOpen
			onClose={onClose}
			title={report ? "Prune report" : "Prune unused images"}
			footer={
				report ? (
					<Button onClick={onClose} data-testid="close-prune-report-button">
						Close
					</Button>
				) : (
					<>
						<Button variant="ghost" onClick={onClose}>
							Cancel
						</Button>
						<Button
							colorScheme="red"
							onClick={() => pruneImages.mutate()}
							isLoading={pruneImages.isPending}
							data-testid="confirm-prune-button"
						>
							Prune images
						</Button>
					</>
				)
			}
		>
			{report ? (
				<PruneReportList report={report} />
			) : (
				<>
					<Text>
						All unused images on this machine will be removed. This cannot be
						undone.
					</Text>
					<Text mt={2} color="gray.500">
						Images that are in use by containers are safe and will not be
						touched.
					</Text>
					{pruneImages.error ? (
						<Box mt={2}>
							<ErrorDisplay error={getApiError(pruneImages.error)} />
						</Box>
					) : null}
				</>
			)}
		</Modal>
	);
}

export function ImageList({
	machineId,
	permission,
}: {
	machineId: string;
	permission: EffectivePermission;
}) {
	const { data: images, isPending, error } = useImages(machineId);
	const deleteImage = useDeleteImage(machineId);
	const pruneImages = usePruneImages(machineId);
	const [pendingDelete, setPendingDelete] = useState<ImageSummary | null>(null);
	const [isPruneOpen, setIsPruneOpen] = useState(false);
	const canManage = canManageImages(permission);

	if (isPending) {
		return <Text data-testid="images-loading">Loading images…</Text>;
	}
	if (error) {
		return <ErrorDisplay error={getApiError(error)} />;
	}
	if (!images || images.length === 0) {
		return (
			<Text color="gray.500" data-testid="images-empty">
				No images on this machine yet. Pull one to get started.
			</Text>
		);
	}

	return (
		<>
			<Flex gap={2} align="flex-start">
				<Box flex={1}>
					<PullImageForm machineId={machineId} />
				</Box>
				{canManage ? (
					<Button
						size="sm"
						colorScheme="red"
						variant="outline"
						onClick={() => {
							pruneImages.reset();
							setIsPruneOpen(true);
						}}
						data-testid="prune-images-button"
					>
						Delete unused images
					</Button>
				) : (
					<Text
						color="gray.500"
						alignSelf="center"
						data-testid="images-permission-hint"
					>
						You have read-only access to this machine. Ask an owner or admin for
						write access to delete its unused images.
					</Text>
				)}
			</Flex>
			<TableContainer>
				<Table data-testid="images-table" variant="simple">
					<Thead>
						<Tr>
							<Th>Tag</Th>
							<Th>Image ID</Th>
							<Th isNumeric>Size</Th>
							<Th>Created</Th>
							<Th />
						</Tr>
					</Thead>
					<Tbody>
						{images.map((image) => (
							<Tr key={image.id} data-testid="image-row">
								<Td fontFamily="mono">{displayTags(image)}</Td>
								<Td fontFamily="mono" color="gray.500">
									{shortImageId(image.id)}
								</Td>
								<Td isNumeric>{formatBytes(image.size)}</Td>
								<Td>{formatRelativeDate(image.createdAt)}</Td>
								<Td>
									<Button
										size="xs"
										colorScheme="red"
										variant="outline"
										aria-label={`Delete ${displayTags(image)}`}
										onClick={() => {
											deleteImage.reset();
											setPendingDelete(image);
										}}
									>
										Delete
									</Button>
								</Td>
							</Tr>
						))}
					</Tbody>
				</Table>
			</TableContainer>
			{isPruneOpen ? (
				<PruneImagesModal
					onClose={() => {
						pruneImages.reset();
						setIsPruneOpen(false);
					}}
					pruneImages={pruneImages}
				/>
			) : null}
			{pendingDelete ? (
				<DeleteImageModal
					image={pendingDelete}
					onClose={() => {
						deleteImage.reset();
						setPendingDelete(null);
					}}
					deleteImage={deleteImage}
				/>
			) : null}
		</>
	);
}
