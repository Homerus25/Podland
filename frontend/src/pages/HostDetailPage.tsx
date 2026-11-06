import { Text } from "@chakra-ui/react";
import { ContainerList } from "@features/containers/components/ContainerList";
import { HostSettingsTab } from "@features/hosts/components/HostSettingsTab";
import { useHost } from "@features/hosts/hooks/useHosts";
import { type HostTabKey, resolveHostTabKey } from "@features/hosts/hostNav";
import { ImageList } from "@features/images/components/ImageList";
import { NetworkList } from "@features/networks/components/NetworkList";
import { PodList } from "@features/pods/components/PodList";
import { QuadletList } from "@features/quadlets/components/QuadletList";
import { SecretList } from "@features/secrets/components/SecretList";
import { VolumeList } from "@features/volumes/components/VolumeList";
import { ErrorDisplay } from "@shared/components/ErrorDisplay";
import { getApiError } from "@shared/utils/api";
import type { ReactElement } from "react";
import { useParams, useSearchParams } from "react-router-dom";

// The host detail page renders only the active tab's panel (issue #459):
// the heading and info row live in the top header, the tab strip in the
// sidebar's host section. The ?tab= search parameter selects the panel —
// an unknown or missing value falls back to Containers — and switching
// tabs is a navigation, so a panel mounts on arrival and unmounts on
// leave: each tab's endpoint fires only once its tab is opened, and a
// list's sort state resets on re-entry. The host query stays eager —
// the permission gating needs it.
export function HostDetailPage() {
	const { hostId } = useParams<{ hostId: string }>();
	const [searchParams] = useSearchParams();
	const { data: host, isPending, error } = useHost(hostId!);
	const tab = resolveHostTabKey(searchParams.get("tab"));

	if (isPending) {
		return <Text data-testid="host-loading">Loading host…</Text>;
	}
	if (error) {
		return <ErrorDisplay error={getApiError(error)} />;
	}
	if (!host) {
		return null;
	}

	// Exhaustive over the tab keys: a new host tab is a type error here
	// until it gets an entry.
	const panels: Record<HostTabKey, ReactElement> = {
		containers: <ContainerList host={host} />,
		volumes: <VolumeList host={host} />,
		images: <ImageList host={host} />,
		networks: <NetworkList host={host} />,
		quadlets: <QuadletList host={host} />,
		pods: <PodList host={host} />,
		secrets: <SecretList host={host} />,
		settings: <HostSettingsTab host={host} />,
	};
	return panels[tab];
}
