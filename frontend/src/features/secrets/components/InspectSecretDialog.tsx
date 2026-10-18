import { useSecretInspect } from "@features/secrets/hooks/useSecrets";
import type { SecretSummary } from "@features/secrets/services/secretsApi";
import { InspectDialog } from "@shared/components/InspectDialog";

// Thin adapter between the secrets feature and the shared inspect dialog:
// it owns the query (fired on mount, dead on unmount, gcTime 0 so every
// open refetches) and hands the dialog its structural slice; the dialog
// owns the whole render. The title is the secret's name, which is also the
// row's key — the backend addresses secrets by name.
export function InspectSecretDialog({
	machineId,
	secret,
	onClose,
}: {
	machineId: string;
	secret: SecretSummary;
	onClose: () => void;
}) {
	const query = useSecretInspect(machineId, secret.name);
	return <InspectDialog title={secret.name} query={query} onClose={onClose} />;
}
