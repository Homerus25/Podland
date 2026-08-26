import { Table, TableContainer, Tbody, Td, Text, Th, Thead, Tr } from '@chakra-ui/react'
import { useQuadlets } from '@features/quadlets/hooks/useQuadlets'
import { ErrorDisplay } from '@shared/components/ErrorDisplay'
import { StateBadge } from '@shared/components/StateBadge'
import { getApiError } from '@shared/utils/api'
import type { QuadletSummary } from '@features/quadlets/services/quadletsApi'

function QuadletRow({ quadlet }: { quadlet: QuadletSummary }) {
  return (
    <Tr data-testid="quadlet-row">
      <Td fontFamily="mono">{quadlet.name}</Td>
      <Td fontFamily="mono">{quadlet.resource}</Td>
      <Td>
        <StateBadge state={quadlet.state} />
      </Td>
    </Tr>
  )
}

export function QuadletList({ machineId }: { machineId: string }) {
  const { data: quadlets, isPending, error } = useQuadlets(machineId)

  if (isPending) {
    return <Text data-testid="quadlets-loading">Loading quadlets…</Text>
  }

  if (error) {
    return <ErrorDisplay error={getApiError(error)} />
  }

  if (!quadlets || quadlets.length === 0) {
    return (
      <Text color="gray.500" data-testid="quadlets-empty">
        No quadlets on this machine yet.
      </Text>
    )
  }

  return (
    <TableContainer>
      <Table data-testid="quadlets-table" variant="simple">
        <Thead>
          <Tr>
            <Th>Name</Th>
            <Th>Resource</Th>
            <Th>State</Th>
          </Tr>
        </Thead>
        <Tbody>
          {quadlets.map((quadlet) => (
            <QuadletRow key={quadlet.id} quadlet={quadlet} />
          ))}
        </Tbody>
      </Table>
    </TableContainer>
  )
}
