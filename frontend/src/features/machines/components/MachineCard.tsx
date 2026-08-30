import { useNavigate } from 'react-router-dom'
import { Button, Card, CardBody, CardHeader, Heading, HStack, Text } from '@chakra-ui/react'
import type { Machine } from '@shared/types/api'
import { PermissionBadge } from '@features/machines/components/PermissionBadge'
import { MachineStatusBadge } from '@shared/components/MachineStatusBadge'
import { useMachineStatus } from '@features/machines/hooks/useMachines'

export function MachineCard({ machine }: { machine: Machine }) {
  const navigate = useNavigate()
  const statusQuery = useMachineStatus(machine.id)

  // checking/unknown are client-side states; online/offline come from the
  // endpoint. Each card resolves on its own so slow or dead hosts never
  // block the list.
  const status =
    statusQuery.isPending
      ? 'checking'
      : statusQuery.isError
        ? 'unknown'
        : (statusQuery.data?.status ?? 'checking')

  return (
    <Card data-testid="machine-card">
      <CardHeader>
        <HStack justify="space-between">
          <Heading size="md">{machine.name}</Heading>
          <HStack spacing={2}>
            <MachineStatusBadge
              status={status}
              checkedAt={statusQuery.data?.checkedAt}
            />
            <PermissionBadge permission={machine.permission} />
          </HStack>
        </HStack>
      </CardHeader>
      <CardBody>
        <Text color="gray.500">{machine.description}</Text>
        <Text fontFamily="mono">
          {machine.hostname}:{machine.port}
        </Text>
        <Button
          size="sm"
          colorScheme="blue"
          variant="outline"
          onClick={() => navigate(`/machines/${machine.id}`)}
          data-testid="view-machine-button"
        >
          View
        </Button>
      </CardBody>
    </Card>
  )
}
