import { Badge, Card, CardBody, CardHeader, Heading, HStack, SimpleGrid, Text, VStack } from '@chakra-ui/react'
import { useMachines } from '@features/machines/hooks/useMachines'
import type { Machine } from '@shared/types/api'
import { ErrorDisplay } from '@shared/components/ErrorDisplay'
import { getApiError } from '@shared/utils/api'

function PermissionBadge({ permission }: { permission: Machine['permission'] }) {
  const colorScheme = permission === 'admin' ? 'purple' : permission === 'write' ? 'blue' : 'gray'
  return <Badge colorScheme={colorScheme} data-testid="machine-permission">{permission}</Badge>
}

function MachineCard({ machine }: { machine: Machine }) {
  return (
    <Card data-testid="machine-card">
      <CardHeader>
        <HStack justify="space-between">
          <Heading size="md">{machine.name}</Heading>
          <PermissionBadge permission={machine.permission} />
        </HStack>
      </CardHeader>
      <CardBody>
        <Text color="gray.500">{machine.description}</Text>
        <Text fontFamily="mono">
          {machine.hostname}:{machine.port}
        </Text>
      </CardBody>
    </Card>
  )
}

export function MachinesPage() {
  const { data: machines, isPending, error } = useMachines()

  return (
    <VStack spacing={4} align="stretch">
      <Heading size="lg">Machines</Heading>
      {isPending && <Text data-testid="machines-loading">Loading machines…</Text>}
      {error && <ErrorDisplay error={getApiError(error)} />}
      {machines &&
        (machines.length === 0 ? (
          <Text data-testid="machines-empty">
            No machines yet. Register a machine to start managing its Podman
            containers — or ask an owner to share one with you. Once you have
            access, it will appear here.
          </Text>
        ) : (
          <SimpleGrid columns={[1, null, 2, 3]} spacing={4}>
            {machines.map((machine) => (
              <MachineCard key={machine.id} machine={machine} />
            ))}
          </SimpleGrid>
        ))}
    </VStack>
  )
}
