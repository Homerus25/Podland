// Single owner of the host-resource query key vocabulary. Every key in the
// app is built here and every list invalidation targets these keys, so a
// rename cannot strand a string literal in another feature. All keys are
// plain `as const` tuples: invalidation is prefix-based, so the union carries
// all the rename-safety that matters.

// Exactly the invalidatable set; mirrors the host detail page's resource
// tabs.
export type HostResource =
	| "containers"
	| "pods"
	| "volumes"
	| "images"
	| "networks"
	| "quadlets"
	| "secrets";

export const hostsQueryKey = ["hosts"] as const;

// App-wide resources are bare constants too (no host id to embed).
export const usersQueryKey = ["users"] as const;

export function containersQueryKey(hostId: string) {
	return ["hosts", hostId, "containers"] as const;
}

export function podsQueryKey(hostId: string) {
	return ["hosts", hostId, "pods"] as const;
}

export function volumesQueryKey(hostId: string) {
	return ["hosts", hostId, "volumes"] as const;
}

export function imagesQueryKey(hostId: string) {
	return ["hosts", hostId, "images"] as const;
}

export function networksQueryKey(hostId: string) {
	return ["hosts", hostId, "networks"] as const;
}

export function quadletsQueryKey(hostId: string) {
	return ["hosts", hostId, "quadlets"] as const;
}

export function secretsQueryKey(hostId: string) {
	return ["hosts", hostId, "secrets"] as const;
}

export function quadletQueryKey(hostId: string, quadletId: string) {
	return [...quadletsQueryKey(hostId), quadletId] as const;
}

// The move/copy preview is a quadlet sub-resource keyed by the chosen target,
// so changing the target refetches. Never an invalidation target of its own:
// quadlets-prefix invalidation (a transfer's success) covers it.
export function quadletMovePreviewQueryKey(
	hostId: string,
	quadletId: string,
	targetHostId: string,
) {
	return [
		...quadletQueryKey(hostId, quadletId),
		"move-preview",
		targetHostId,
	] as const;
}

// The volume copy/move preview is a volume sub-resource keyed by the chosen
// target, so changing the target refetches. Never an invalidation target of
// its own: volumes-prefix invalidation (a transfer's success) covers it.
export function volumeMovePreviewQueryKey(
	hostId: string,
	volumeName: string,
	targetHostId: string,
) {
	return [
		...volumeQueryKey(hostId, volumeName),
		"move-preview",
		targetHostId,
	] as const;
}

export function secretQueryKey(hostId: string, secretId: string) {
	return [...secretsQueryKey(hostId), secretId] as const;
}

// The secret copy/move preview is a secret sub-resource keyed by the chosen
// target, so changing the target refetches. Never an invalidation target of
// its own: secrets-prefix invalidation (a transfer's success) covers it.
export function secretMovePreviewQueryKey(
	hostId: string,
	secretId: string,
	targetHostId: string,
) {
	return [
		...secretQueryKey(hostId, secretId),
		"move-preview",
		targetHostId,
	] as const;
}

export function volumeQueryKey(hostId: string, volumeName: string) {
	return [...volumesQueryKey(hostId), volumeName] as const;
}

export function hostDetailKey(id: string) {
	return ["hosts", id] as const;
}

// Sub-resources reuse the key shape but are never invalidation targets, so
// they are deliberately not HostResource members.
export function hostStatusQueryKey(id: string) {
	return ["hosts", id, "status"] as const;
}

export function hostVersionQueryKey(id: string) {
	return ["hosts", id, "version"] as const;
}

export function hostAccessQueryKey(id: string) {
	return ["hosts", id, "access"] as const;
}

// The detail view is a container sub-resource keyed by the container, so
// every page visit resolves to its own entry. Never an invalidation target
// of its own: containers-prefix invalidation covers it.
export function containerQueryKey(hostId: string, containerId: string) {
	return [...containersQueryKey(hostId), containerId] as const;
}

// The detail view is a pod sub-resource keyed by the pod, so every page
// visit resolves to its own entry. Never an invalidation target of its
// own: pods-prefix invalidation covers it.
export function podQueryKey(hostId: string, podId: string) {
	return [...podsQueryKey(hostId), podId] as const;
}

// The raw inspect report is a pod sub-resource keyed by the pod, so every
// reveal resolves to its own entry. Never an invalidation target of its
// own: pods-prefix invalidation covers it.
export function podInspectQueryKey(hostId: string, podId: string) {
	return [...podsQueryKey(hostId), podId, "inspect"] as const;
}

// The raw inspect report is a container sub-resource keyed by the container,
// so every dialog open resolves to its own entry. Never an invalidation
// target of its own: containers-prefix invalidation covers it.
export function containerInspectQueryKey(hostId: string, containerId: string) {
	return [...containersQueryKey(hostId), containerId, "inspect"] as const;
}

// The all-stats snapshot is a host's containers sub-resource, so every
// dialog open resolves to its own entry. Never an invalidation target of
// its own: containers-prefix invalidation covers it.
export function containersStatsQueryKey(hostId: string) {
	return [...containersQueryKey(hostId), "stats"] as const;
}

// The one-container stats snapshot is a container sub-resource keyed by
// the container, a sibling of the inspect key, so every dialog open
// resolves to its own entry. Never an invalidation target of its own:
// containers-prefix invalidation covers it.
export function containerStatsQueryKey(hostId: string, containerId: string) {
	return [...containersQueryKey(hostId), containerId, "stats"] as const;
}

// The detail view is a network sub-resource keyed by the network, so every
// page visit resolves to its own entry. Never an invalidation target of its
// own: networks-prefix invalidation covers it.
export function networkQueryKey(hostId: string, networkName: string) {
	return [...networksQueryKey(hostId), networkName] as const;
}

// The raw inspect report is a network sub-resource keyed by the network,
// so every dialog open resolves to its own entry. Never an invalidation
// target of its own: networks-prefix invalidation covers it.
export function networkInspectQueryKey(hostId: string, networkName: string) {
	return [...networksQueryKey(hostId), networkName, "inspect"] as const;
}

// The detail view is an image sub-resource keyed by the image, so every
// page visit resolves to its own entry. Never an invalidation target of its
// own: images-prefix invalidation covers it.
export function imageQueryKey(hostId: string, imageId: string) {
	return [...imagesQueryKey(hostId), imageId] as const;
}

// The raw inspect report is an image sub-resource keyed by the image, so
// every dialog open resolves to its own entry. Never an invalidation
// target of its own: images-prefix invalidation covers it.
export function imageInspectQueryKey(hostId: string, imageId: string) {
	return [...imagesQueryKey(hostId), imageId, "inspect"] as const;
}

// The image copy preview is an image sub-resource keyed by the chosen
// target, so changing the target refetches. Never an invalidation target
// of its own: images-prefix invalidation (a copy's success) covers it.
export function imageCopyPreviewQueryKey(
	hostId: string,
	imageId: string,
	targetHostId: string,
) {
	return [
		...imageQueryKey(hostId, imageId),
		"copy-preview",
		targetHostId,
	] as const;
}

// The raw inspect report is a secret sub-resource keyed by the secret, so
// every dialog open resolves to its own entry. Never an invalidation target
// of its own: secrets-prefix invalidation covers it.
export function secretInspectQueryKey(hostId: string, secretId: string) {
	return [...secretsQueryKey(hostId), secretId, "inspect"] as const;
}
