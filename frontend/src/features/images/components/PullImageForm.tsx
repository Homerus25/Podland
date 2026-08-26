import { useState } from 'react'
import { Button, Flex, FormControl, FormLabel, Input, Progress, Text, VStack } from '@chakra-ui/react'
import { usePullImage } from '@features/images/hooks/usePullImage'
import { ErrorDisplay } from '@shared/components/ErrorDisplay'
import { formatBytes } from '@shared/utils/format'

// Pulls an image by reference, rendering the backend's NDJSON progress
// stream live: status text, a byte progress bar, and a cancel button.
export function PullImageForm({ machineId }: { machineId: string }) {
  const { isPulling, progress, error, pull, cancel } = usePullImage(machineId)
  const [imageRef, setImageRef] = useState('')

  const current = progress?.state?.current
  const total = progress?.state?.total
  const percent =
    current !== undefined && total !== undefined && total > 0 ? Math.round((current / total) * 100) : undefined

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
            <Button colorScheme="red" variant="outline" onClick={cancel} data-testid="cancel-pull-button">
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
        <VStack spacing={1} align="stretch" data-testid="pull-progress">
          <Text fontSize="sm">{progress?.state?.status ?? 'Starting pull…'}</Text>
          {progress?.state?.message ? (
            <Text fontSize="sm" color="gray.500">{progress.state.message}</Text>
          ) : null}
          {percent !== undefined ? (
            <>
              <Progress value={percent} size="sm" />
              <Text fontSize="sm" color="gray.500">
                {`${formatBytes(current ?? 0)} / ${formatBytes(total ?? 0)}`}
              </Text>
            </>
          ) : null}
        </VStack>
      ) : null}

      {error ? <ErrorDisplay error={error} /> : null}
    </VStack>
  )
}
