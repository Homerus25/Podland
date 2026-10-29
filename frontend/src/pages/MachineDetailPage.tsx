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
import {
	useMachine,
	useMachineVersion,
} from "@features/machines/hooks/useMachines";
import { NetworkList } from "@features/networks/components/NetworkList";
import { PodList } from "@features/pods/components/PodList";
import { QuadletList } from "@features/quadlets/components/QuadletList";
import { SecretList } from "@features/secrets/components/SecretList";
import { VolumeList } from "@features/volumes/components/VolumeList";
import { ErrorDisplay } from "@shared/components/ErrorDisplay";
import { getApiError } from "@shared/utils/api";
import { useParams, useSearchParams } from "react-router-dom";

// Tab keys in panel order; the ?tab= deep link resolves against these.
// An unknown or missing value falls back to the Containers tab.
// This array must stay in sync with the <TabList>/<TabPanels> order
// below: index i here selects tab i there, so adding, removing, or
// reordering a tab means editing both places together.
const MACHINE_TAB_KEYS = [
	"containers",
	"volumes",
	"images",
	"networks",
	"quadlets",
	"pods",
	"secrets",
	"settings",
] as const;

function initialTabIndex(tabParam: string | null): number {
	const index = MACHINE_TAB_KEYS.indexOf(
		tabParam as (typeof MACHINE_TAB_KEYS)[number],
	);
	return index === -1 ? 0 : index;
}

export function MachineDetailPage() {
	const { machineId } = useParams<{ machineId: string }>();
	const [searchParams] = useSearchParams();
	const { data: machine, isPending, error } = useMachine(machineId!);
	// The remote Podman's version report is a live fact, fetched fresh on
	// every page visit (staleTime 0, the status probe's pattern). While the
	// fetch is pending the header shows no version text; null fields (the
	// machine is offline or the version call failed) render the
	// unavailable fallback — never an error, a down machine must not break
	// the page header.
	const { data: version, isPending: versionPending } = useMachineVersion(
		machineId!,
	);

	if (isPending) {
		return <Text data-testid="machine-loading">Loading machine…</Text>;
	}
	if (error) {
		return <ErrorDisplay error={getApiError(error)} />;
	}
	if (!machine) {
		return null;
	}

	return (
		<VStack spacing={4} align="stretch">
			{/* Small viewports (spec 393): the header rows wrap at base
				instead of clipping, and the mono endpoint — a single
				unbreakable token — may break mid-token. lg and up render
				as before. */}
			<HStack justify="space-between" flexWrap={{ base: "wrap", lg: "nowrap" }}>
				<Heading size="lg">{machine.name}</Heading>
				<PermissionBadge permission={machine.permission} />
			</HStack>
			<HStack spacing={4} flexWrap={{ base: "wrap", lg: "nowrap" }}>
				<Text
					fontFamily="mono"
					color="gray.500"
					overflowWrap={{ base: "anywhere", lg: "normal" }}
					data-testid="machine-endpoint"
				>
					{machine.hostname}:{machine.port}
				</Text>
				<Text fontFamily="mono" color="gray.500" data-testid="machine-ssh-user">
					ssh user: {machine.sshUsername}
				</Text>
				{!versionPending && (
					<Text
						fontFamily="mono"
						color="gray.500"
						data-testid="machine-podman-version"
					>
						{version?.podmanVersion && version?.apiVersion
							? `podman ${version.podmanVersion} · api ${version.apiVersion}`
							: "podman version unavailable"}
					</Text>
				)}
			</HStack>
			{/* Lazy mounting: a tab's panel mounts on first activation and
			unmounts on leave (Chakra's default lazyBehavior), so each tab's
			endpoint fires only once its tab is opened and a list's
			sort state resets on re-entry. The machine query above stays
			eager — the header and the permission gating need it.
			defaultIndex reads ?tab= once on load (uncontrolled afterwards:
			syncing the URL on manual tab switches is out of scope), so a
			deep-linked tab's panel is the one that mounts on arrival. */}
			<Tabs isLazy defaultIndex={initialTabIndex(searchParams.get("tab"))}>
				{/* Small viewports (spec 393): below lg the strip scrolls
					sideways instead of overflowing the page; at lg and up
					the overflow resets to the CSS default, so the desktop
					strip renders as before. */}
				<TabList
					overflowX={{ base: "auto", lg: "visible" }}
					overflowY={{ base: "hidden", lg: "visible" }}
				>
					<Tab>Containers</Tab>
					<Tab>Volumes</Tab>
					<Tab>Images</Tab>
					<Tab>Networks</Tab>
					<Tab>Quadlets</Tab>
					<Tab>Pods</Tab>
					<Tab>Secrets</Tab>
					<Tab>Settings</Tab>
				</TabList>
				<TabPanels>
					<TabPanel>
						<ContainerList machine={machine} />
					</TabPanel>
					<TabPanel>
						<VolumeList machine={machine} />
					</TabPanel>
					<TabPanel>
						<ImageList machine={machine} />
					</TabPanel>
					<TabPanel>
						<NetworkList machine={machine} />
					</TabPanel>
					<TabPanel>
						<QuadletList machine={machine} />
					</TabPanel>
					<TabPanel>
						<PodList machine={machine} />
					</TabPanel>
					<TabPanel>
						<SecretList machine={machine} />
					</TabPanel>
					<TabPanel>
						<MachineSettingsTab machine={machine} />
					</TabPanel>
				</TabPanels>
			</Tabs>
		</VStack>
	);
}
