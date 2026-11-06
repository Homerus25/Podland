import { Heading, HStack, Text } from "@chakra-ui/react";
import { useHost, useHostVersion } from "@features/hosts/hooks/useHosts";
import { PermissionBadge } from "./PermissionBadge";

// The top header's host identity (issue #459): the current host's name
// with its permission badge, plus the endpoint, ssh user, and the live
// podman version report on the same row — the row wraps under the name
// on small viewports (the spec 393 base-wrap pattern; the mono endpoint
// may break mid-token). While the host query is pending the slot renders
// nothing; a failed fetch falls back to the area title so the chrome
// never blanks. The version keeps its live-fetch discipline: no text
// while pending, the unavailable fallback when the report is null.
export function HostHeaderIdentity({ hostId }: { hostId: string }) {
	const { data: host, isPending, error } = useHost(hostId);
	const { data: version, isPending: versionPending } = useHostVersion(hostId);

	if (isPending) {
		return null;
	}
	if (error || !host) {
		return <Heading size="md">Hosts</Heading>;
	}

	return (
		<HStack
			spacing={4}
			flexWrap={{ base: "wrap", lg: "nowrap" }}
			align="center"
			minW={0}
		>
			<Heading size="md">{host.name}</Heading>
			<PermissionBadge permission={host.permission} />
			<Text
				fontFamily="mono"
				color="gray.500"
				overflowWrap={{ base: "anywhere", lg: "normal" }}
				data-testid="host-endpoint"
			>
				{host.hostname}:{host.port}
			</Text>
			<Text fontFamily="mono" color="gray.500" data-testid="host-ssh-user">
				ssh user: {host.sshUsername}
			</Text>
			{!versionPending && (
				<Text
					fontFamily="mono"
					color="gray.500"
					data-testid="host-podman-version"
				>
					{version?.podmanVersion && version?.apiVersion
						? `podman ${version.podmanVersion} · api ${version.apiVersion}`
						: "podman version unavailable"}
				</Text>
			)}
		</HStack>
	);
}
