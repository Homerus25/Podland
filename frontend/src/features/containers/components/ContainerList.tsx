import { Button, Table, TableContainer, Tbody, Td, Text, Th, Thead, Tr } from '@chakra-ui/react'
import {
  useContainers,
  useStartContainer,
  useStopContainer,
} from '@features/containers/hooks/useContainers'
import { ErrorDisplay } from '@shared/components/ErrorDisplay'
import { StateBadge } from '@shared/components/StateBadge'
import { getApiError } from '@shared/utils/api'
import { shortImageId } from '@shared/utils/format'
import type { ContainerSummary } from '@features/containers/services/containersApi'

// Podman reports container names with a leading slash; display them bare and
// fall back to the short id when a container has no name at all.
function displayName(container: ContainerSummary): string {
  const name = container.names?.[0]?.replace(/^\//, '')
  return name ? name : shortImageId(container.id)
}

// Each row owns its lifecycle mutation instances, so pending state and
// errors are scoped to the affected container and other rows stay usable.
function ContainerRow({
  machineId,
  container,
}: {
  machineId: string
  container: ContainerSummary
}) {
  const startContainer = useStartContainer(machineId)
  const stopContainer = useStopContainer(machineId)
  const action =
    container.state === 'running'
      ? {
          lifecycle: stopContainer,
          testId: 'stop-container-button',
          label: 'Stop',
          colorScheme: undefined,
        }
      : {
          lifecycle: startContainer,
          testId: 'start-container-button',
          label: 'Start',
          colorScheme: 'green' as const,
        }

  return (
    <>
      <Tr data-testid="container-row">
        <Td fontFamily="mono">{displayName(container)}</Td>
        <Td fontFamily="mono">{container.image}</Td>
        <Td>
          <StateBadge state={container.state} />
        </Td>
        <Td>
          <Button
            size="xs"
            variant="outline"
            colorScheme={action.colorScheme}
            onClick={() => action.lifecycle.mutate(container.id)}
            isLoading={action.lifecycle.isPending}
            data-testid={action.testId}
          >
            {action.label}
          </Button>
        </Td>
      </Tr>
      {action.lifecycle.error ? (
        <Tr>
          <Td colSpan={4}>
            <ErrorDisplay error={getApiError(action.lifecycle.error)} />
          </Td>
        </Tr>
      ) : null}
    </>
  )
}

export function ContainerList({ machineId }: { machineId: string }) {
  const { data: containers, isPending, error } = useContainers(machineId)

  if (isPending) {
    return <Text data-testid="containers-loading">Loading containers…</Text>
  }
  if (error) {
    return <ErrorDisplay error={getApiError(error)} />
  }
  if (!containers || containers.length === 0) {
    return (
      <Text color="gray.500" data-testid="containers-empty">
        No containers on this machine yet.
      </Text>
    )
  }

  return (
    <TableContainer>
      <Table data-testid="containers-table" variant="simple">
        <Thead>
          <Tr>
            <Th>Name</Th>
            <Th>Image</Th>
            <Th>State</Th>
            <Th />
          </Tr>
        </Thead>
        <Tbody>
          {containers.map((container) => (
            <ContainerRow key={container.id} machineId={machineId} container={container} />
          ))}
        </Tbody>
      </Table>
    </TableContainer>
  )
}
