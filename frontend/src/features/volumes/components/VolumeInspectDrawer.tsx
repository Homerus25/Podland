import {
	Drawer,
	DrawerBody,
	DrawerCloseButton,
	DrawerContent,
	DrawerHeader,
	DrawerOverlay,
	Table,
	TableContainer,
	Tbody,
	Td,
	Text,
	Tr,
} from "@chakra-ui/react";
import { useVolume } from "@features/volumes/hooks/useVolume";
import type { VolumeDetail } from "@features/volumes/services/volumesApi";
import { ErrorDisplay } from "@shared/components/ErrorDisplay";
import { invalidateMachineResources } from "@shared/queries/useListInvalidatingMutation";
import { getApiError } from "@shared/utils/api";
import { formatBytes, formatRelativeDate } from "@shared/utils/format";
import { useQueryClient } from "@tanstack/react-query";
import { isAxiosError } from "axios";
import { useEffect, useRef } from "react";

function isNotFound(error: unknown): boolean {
	return isAxiosError(error) && error.response?.status === 404;
}

function KeyValueTable({
	label,
	values,
	testId,
}: {
	label: string;
	values: Record<string, string>;
	testId: string;
}) {
	// A missing map renders as None: the app has no error boundary, so a
	// throw here would unmount the whole page.
	const entries = Object.entries(values ?? {});
	return (
		<>
			<Text fontWeight="bold" mt={4}>
				{label}
			</Text>
			{entries.length === 0 ? (
				<Text color="gray.500" data-testid={testId}>
					None
				</Text>
			) : (
				<TableContainer data-testid={testId}>
					<Table size="sm">
						<Tbody>
							{entries.map(([key, value]) => (
								<Tr key={key}>
									<Td fontFamily="mono" width="40%">
										{key}
									</Td>
									<Td fontFamily="mono">{value}</Td>
								</Tr>
							))}
						</Tbody>
					</Table>
				</TableContainer>
			)}
		</>
	);
}

function DetailRow({
	label,
	children,
}: {
	label: string;
	children: React.ReactNode;
}) {
	return (
		<Tr>
			<Td fontWeight="bold" width="40%">
				{label}
			</Td>
			<Td>{children}</Td>
		</Tr>
	);
}

function VolumeDetailBody({ volume }: { volume: VolumeDetail }) {
	return (
		<TableContainer data-testid="volume-drawer-content">
			<Table size="sm">
				<Tbody>
					<DetailRow label="Name">
						<Text fontFamily="mono">{volume.name}</Text>
					</DetailRow>
					<DetailRow label="Driver">{volume.driver}</DetailRow>
					<DetailRow label="Type">
						<Text>{volume.anonymous ? "Anonymous" : "Named"}</Text>
					</DetailRow>
					<DetailRow label="Created">
						<Text data-testid="volume-drawer-created">
							{formatRelativeDate(volume.createdAt)}
						</Text>
					</DetailRow>
					<DetailRow label="Size">
						{volume.size !== undefined ? formatBytes(volume.size) : "—"}
					</DetailRow>
					<DetailRow label="Mountpoint">
						<Text fontFamily="mono" wordBreak="break-all">
							{volume.mountpoint}
						</Text>
					</DetailRow>
				</Tbody>
			</Table>
			<KeyValueTable
				label="Labels"
				testId="volume-drawer-labels"
				values={volume.labels}
			/>
			<KeyValueTable
				label="Options"
				testId="volume-drawer-options"
				values={volume.options}
			/>
		</TableContainer>
	);
}

export function VolumeInspectDrawer({
	machineId,
	volumeName,
	isOpen,
	onClose,
}: {
	machineId: string;
	volumeName: string | null;
	isOpen: boolean;
	onClose: () => void;
}) {
	const queryClient = useQueryClient();
	const {
		data: volume,
		isPending,
		error,
	} = useVolume(machineId, isOpen ? volumeName : null);

	// The volumes list key is a prefix of the inspect key, so invalidating the
	// list also refetches this query; each refetched 404 is a fresh error
	// object, which would re-trigger this effect forever. One invalidation per
	// stale volume is what the flow needs.
	const invalidatedRef = useRef<string | null>(null);

	useEffect(() => {
		if (!isOpen) invalidatedRef.current = null;
		// A 404 means the row was stale (the volume was deleted since the
		// list loaded); refetch the list so the dead row disappears.
		if (isOpen && isNotFound(error) && invalidatedRef.current !== volumeName) {
			invalidatedRef.current = volumeName;
			invalidateMachineResources(queryClient, machineId, "volumes");
		}
	}, [isOpen, volumeName, error, queryClient, machineId]);

	return (
		<Drawer isOpen={isOpen} onClose={onClose} size="md">
			<DrawerOverlay />
			<DrawerContent>
				<DrawerCloseButton />
				<DrawerHeader>Volume details</DrawerHeader>
				<DrawerBody>
					{isPending ? (
						<Text data-testid="volume-drawer-loading">Loading volume…</Text>
					) : isNotFound(error) ? (
						<Text data-testid="volume-drawer-not-found">
							This volume no longer exists on the machine.
						</Text>
					) : error ? (
						<ErrorDisplay
							error={getApiError(error)}
							data-testid="volume-drawer-error"
						/>
					) : volume ? (
						<VolumeDetailBody volume={volume} />
					) : null}
				</DrawerBody>
			</DrawerContent>
		</Drawer>
	);
}
