import {
	Alert,
	AlertDescription,
	AlertIcon,
	AlertTitle,
} from "@chakra-ui/react";
import type { ApiError } from "@shared/types/api";

export function ErrorDisplay({
	error,
	...rest
}: { error: ApiError } & Record<string, unknown>) {
	return (
		<Alert status="error" variant="subtle" role="alert" {...rest}>
			<AlertIcon />
			<AlertTitle>{error.code}</AlertTitle>
			<AlertDescription display="block">{error.message}</AlertDescription>
		</Alert>
	);
}
