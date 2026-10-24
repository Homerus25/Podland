import {
	Button,
	Flex,
	FormControl,
	FormLabel,
	Input,
	VStack,
} from "@chakra-ui/react";
import { PullProgressDisplay } from "@features/images/components/PullProgressDisplay";
import { usePullImage } from "@features/images/hooks/usePullImage";
import { ErrorDisplay } from "@shared/components/ErrorDisplay";
import { useState } from "react";

// Pulls an image by reference, rendering the backend's NDJSON progress
// stream live: status text, a byte progress bar, and a cancel button.
export function PullImageForm({ machineId }: { machineId: string }) {
	const { isPulling, progress, error, pull, cancel } = usePullImage(machineId);
	const [imageRef, setImageRef] = useState("");

	return (
		<VStack spacing={3} align="stretch" data-testid="pull-image-form">
			<FormControl>
				<FormLabel htmlFor="image-reference">Image reference</FormLabel>
				<Flex gap={2}>
					<Input
						id="image-reference"
						placeholder="e.g. docker.io/library/nginx:latest"
						fontFamily="mono"
						value={imageRef}
						onChange={(event) => setImageRef(event.target.value)}
						isDisabled={isPulling}
					/>
					{isPulling ? (
						<Button
							colorScheme="red"
							variant="outline"
							onClick={cancel}
							data-testid="cancel-pull-button"
						>
							Cancel
						</Button>
					) : (
						<Button
							colorScheme="blue"
							onClick={() => pull(imageRef.trim())}
							isDisabled={imageRef.trim().length === 0}
							data-testid="start-pull-button"
						>
							Pull
						</Button>
					)}
				</Flex>
			</FormControl>

			{isPulling ? (
				<PullProgressDisplay progress={progress} data-testid="pull-progress" />
			) : null}

			{error ? <ErrorDisplay error={error} /> : null}
		</VStack>
	);
}
