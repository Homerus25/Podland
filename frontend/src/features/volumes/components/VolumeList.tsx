import { Badge, Table, TableContainer, Tbody, Td, Text, Th, Thead, Tr } from '@chakra-ui/react'
import { useVolumes } from '@features/volumes/hooks/useVolumes'
import type { VolumeSummary } from '@features/volumes/services/volumesApi'
import { ErrorDisplay } from '@shared/components/ErrorDisplay'
import { getApiError } from '@shared/utils/api'
import { formatBytes, formatRelativeDate } from '@shared/utils/format'

function VolumeRow({ volume }: { volume: VolumeSummary }) {
  return (
    <Tr data-testid="volume-row">
      <Td fontFamily="mono">{volume.name}</Td>
      <Td>
        <Badge
          colorScheme={volume.anonymous ? 'gray' : 'purple'}
          data-testid="volume-type-badge"
        >
          {volume.anonymous ? 'Anonymous' : 'Named'}
        </Badge>
      </Td>
      <Td>{volume.driver}</Td>
      <Td>{formatRelativeDate(volume.createdAt)}</Td>
      <Td isNumeric>{volume.size !== undefined ? formatBytes(volume.size) : '—'}</Td>
    </Tr>
  )
}

export function VolumeList({ machineId }: { machineId: string }) {
  const { data: volumes, isPending, error } = useVolumes(machineId)

  if (isPending) {
    return <Text data-testid="volumes-loading">Loading volumes…</Text>
  }

  return (
    <>
      {error ? (
        <ErrorDisplay error={getApiError(error)} data-testid="volumes-error" />
      ) : !volumes || volumes.length === 0 ? (
        <Text color="gray.500" data-testid="volumes-empty">
          No volumes on this machine yet.
        </Text>
      ) : (
        <TableContainer>
          <Table data-testid="volumes-table" variant="simple">
            <Thead>
              <Tr>
                <Th>Name</Th>
                <Th>Type</Th>
                <Th>Driver</Th>
                <Th>Created</Th>
                <Th isNumeric>Size</Th>
              </Tr>
            </Thead>
            <Tbody>
              {volumes.map((volume) => (
                <VolumeRow key={volume.name} volume={volume} />
              ))}
            </Tbody>
          </Table>
        </TableContainer>
      )}
    </>
  )
}
