import {
	Heading,
	HStack,
	Tab,
	TabList,
	TabPanel,
	TabPanels,
	Tabs,
	Text,
	VStack,
} from "@chakra-ui/react";
import { ContainerList } from "@features/containers/components/ContainerList";
import { ImageList } from "@features/images/components/ImageList";
import { MachineSettingsTab } from "@features/machines/components/MachineSettingsTab";
import { PermissionBadge } from "@features/machines/components/PermissionBadge";
import { useMachine } from "@features/machines/hooks/useMachines";
import { effectivePermission } from "@features/machines/permissions";
import { QuadletList } from "@features/quadlets/components/QuadletList";
import { SecretList } from "@features/secrets/components/SecretList";
import { VolumeList } from "@features/volumes/components/VolumeList";
import { ErrorDisplay } from "@shared/components/ErrorDisplay";
import { getApiError } from "@shared/utils/api";
import { useParams } from "react-router-dom";

export function MachineDetailPage() {
	const { machineId } = useParams<{ machineId: string }>();
	const { data: machine, isPending, error } = useMachine(machineId!);

	if (isPending) {
		return <Text data-testid="machine-loading">Loading machine…</Text>;
	}
	if (error) {
		return <ErrorDisplay error={getApiError(error)} />;
	}
	if (!machine) {
		return null;
	}
	const permission = effectivePermission(machine);

	return (
		<VStack spacing={4} align="stretch">
			<HStack justify="space-between">
				<Heading size="lg">{machine.name}</Heading>
				<PermissionBadge permission={machine.permission} />
			</HStack>
			<HStack spacing={4}>
				<Text fontFamily="mono" color="gray.500" data-testid="machine-endpoint">
					{machine.hostname}:{machine.port}
				</Text>
				<Text fontFamily="mono" color="gray.500" data-testid="machine-ssh-user">
					ssh user: {machine.sshUsername}
				</Text>
			</HStack>
			<Tabs>
				<TabList>
					<Tab>Containers</Tab>
					<Tab>Volumes</Tab>
					<Tab>Images</Tab>
					<Tab>Quadlets</Tab>
					<Tab>Secrets</Tab>
					<Tab>Settings</Tab>
				</TabList>
				<TabPanels>
					<TabPanel>
						<ContainerList machineId={machine.id} />
					</TabPanel>
					<TabPanel>
						<VolumeList machineId={machine.id} permission={permission} />
					</TabPanel>
					<TabPanel>
						<ImageList machineId={machine.id} permission={permission} />
					</TabPanel>
					<TabPanel>
						<QuadletList machineId={machine.id} permission={permission} />
					</TabPanel>
					<TabPanel>
						<SecretList machineId={machine.id} permission={permission} />
					</TabPanel>
					<TabPanel>
						<MachineSettingsTab machine={machine} />
					</TabPanel>
				</TabPanels>
			</Tabs>
		</VStack>
	);
}
