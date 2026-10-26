// Single owner of the machine-resource query key vocabulary. Every key in the
// app is built here and every list invalidation targets these keys, so a
// rename cannot strand a string literal in another feature. All keys are
// plain `as const` tuples: invalidation is prefix-based, so the union carries
// all the rename-safety that matters.

// Exactly the invalidatable set; mirrors the machine detail page's resource
// tabs.
export type MachineResource =
	| "containers"
	| "pods"
	| "volumes"
	| "images"
	| "networks"
	| "quadlets"
	| "secrets";

export const machinesQueryKey = ["machines"] as const;

// App-wide resources are bare constants too (no machine id to embed).
export const usersQueryKey = ["users"] as const;

export function containersQueryKey(machineId: string) {
	return ["machines", machineId, "containers"] as const;
}

export function podsQueryKey(machineId: string) {
	return ["machines", machineId, "pods"] as const;
}

export function volumesQueryKey(machineId: string) {
	return ["machines", machineId, "volumes"] as const;
}

export function imagesQueryKey(machineId: string) {
	return ["machines", machineId, "images"] as const;
}

export function networksQueryKey(machineId: string) {
	return ["machines", machineId, "networks"] as const;
}

export function quadletsQueryKey(machineId: string) {
	return ["machines", machineId, "quadlets"] as const;
}

export function secretsQueryKey(machineId: string) {
	return ["machines", machineId, "secrets"] as const;
}

export function quadletQueryKey(machineId: string, quadletId: string) {
	return [...quadletsQueryKey(machineId), quadletId] as const;
}

// The move/copy preview is a quadlet sub-resource keyed by the chosen target,
// so changing the target refetches. Never an invalidation target of its own:
// quadlets-prefix invalidation (a transfer's success) covers it.
export function quadletMovePreviewQueryKey(
	machineId: string,
	quadletId: string,
	targetMachineId: string,
) {
	return [
		...quadletQueryKey(machineId, quadletId),
		"move-preview",
		targetMachineId,
	] as const;
}

// The volume copy/move preview is a volume sub-resource keyed by the chosen
// target, so changing the target refetches. Never an invalidation target of
// its own: volumes-prefix invalidation (a transfer's success) covers it.
export function volumeMovePreviewQueryKey(
	machineId: string,
	volumeName: string,
	targetMachineId: string,
) {
	return [
		...volumeQueryKey(machineId, volumeName),
		"move-preview",
		targetMachineId,
	] as const;
}

export function secretQueryKey(machineId: string, secretId: string) {
	return [...secretsQueryKey(machineId), secretId] as const;
}

// The secret copy/move preview is a secret sub-resource keyed by the chosen
// target, so changing the target refetches. Never an invalidation target of
// its own: secrets-prefix invalidation (a transfer's success) covers it.
export function secretMovePreviewQueryKey(
	machineId: string,
	secretId: string,
	targetMachineId: string,
) {
	return [
		...secretQueryKey(machineId, secretId),
		"move-preview",
		targetMachineId,
	] as const;
}

export function volumeQueryKey(machineId: string, volumeName: string) {
	return [...volumesQueryKey(machineId), volumeName] as const;
}

export function machineDetailKey(id: string) {
	return ["machines", id] as const;
}

// Sub-resources reuse the key shape but are never invalidation targets, so
// they are deliberately not MachineResource members.
export function machineStatusQueryKey(id: string) {
	return ["machines", id, "status"] as const;
}

export function machineAccessQueryKey(id: string) {
	return ["machines", id, "access"] as const;
}

// The detail view is a container sub-resource keyed by the container, so
// every page visit resolves to its own entry. Never an invalidation target
// of its own: containers-prefix invalidation covers it.
export function containerQueryKey(machineId: string, containerId: string) {
	return [...containersQueryKey(machineId), containerId] as const;
}

// The detail view is a pod sub-resource keyed by the pod, so every page
// visit resolves to its own entry. Never an invalidation target of its
// own: pods-prefix invalidation covers it.
export function podQueryKey(machineId: string, podId: string) {
	return [...podsQueryKey(machineId), podId] as const;
}

// The raw inspect report is a pod sub-resource keyed by the pod, so every
// reveal resolves to its own entry. Never an invalidation target of its
// own: pods-prefix invalidation covers it.
export function podInspectQueryKey(machineId: string, podId: string) {
	return [...podsQueryKey(machineId), podId, "inspect"] as const;
}

// The raw inspect report is a container sub-resource keyed by the container,
// so every dialog open resolves to its own entry. Never an invalidation
// target of its own: containers-prefix invalidation covers it.
export function containerInspectQueryKey(
	machineId: string,
	containerId: string,
) {
	return [...containersQueryKey(machineId), containerId, "inspect"] as const;
}

// The detail view is a network sub-resource keyed by the network, so every
// page visit resolves to its own entry. Never an invalidation target of its
// own: networks-prefix invalidation covers it.
export function networkQueryKey(machineId: string, networkName: string) {
	return [...networksQueryKey(machineId), networkName] as const;
}

// The raw inspect report is a network sub-resource keyed by the network,
// so every dialog open resolves to its own entry. Never an invalidation
// target of its own: networks-prefix invalidation covers it.
export function networkInspectQueryKey(machineId: string, networkName: string) {
	return [...networksQueryKey(machineId), networkName, "inspect"] as const;
}

// The detail view is an image sub-resource keyed by the image, so every
// page visit resolves to its own entry. Never an invalidation target of its
// own: images-prefix invalidation covers it.
export function imageQueryKey(machineId: string, imageId: string) {
	return [...imagesQueryKey(machineId), imageId] as const;
}

// The raw inspect report is an image sub-resource keyed by the image, so
// every dialog open resolves to its own entry. Never an invalidation
// target of its own: images-prefix invalidation covers it.
export function imageInspectQueryKey(machineId: string, imageId: string) {
	return [...imagesQueryKey(machineId), imageId, "inspect"] as const;
}

// The image copy preview is an image sub-resource keyed by the chosen
// target, so changing the target refetches. Never an invalidation target
// of its own: images-prefix invalidation (a copy's success) covers it.
export function imageCopyPreviewQueryKey(
	machineId: string,
	imageId: string,
	targetMachineId: string,
) {
	return [
		...imageQueryKey(machineId, imageId),
		"copy-preview",
		targetMachineId,
	] as const;
}

// The raw inspect report is a secret sub-resource keyed by the secret, so
// every dialog open resolves to its own entry. Never an invalidation target
// of its own: secrets-prefix invalidation covers it.
export function secretInspectQueryKey(machineId: string, secretId: string) {
	return [...secretsQueryKey(machineId), secretId, "inspect"] as const;
}
