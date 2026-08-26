import { Table, TableContainer, Tbody, Td, Text, Th, Thead, Tr } from '@chakra-ui/react'
import { useContainers } from '@features/containers/hooks/useContainers'
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
          </Tr>
        </Thead>
        <Tbody>
          {containers.map((container) => (
            <Tr key={container.id} data-testid="container-row">
              <Td fontFamily="mono">{displayName(container)}</Td>
              <Td fontFamily="mono">{container.image}</Td>
              <Td>
                <StateBadge state={container.state} />
              </Td>
            </Tr>
          ))}
        </Tbody>
      </Table>
    </TableContainer>
  )
}
