import { useNavigate } from 'react-router-dom'
import { Button, Card, CardBody, CardHeader, Heading, HStack, Text } from '@chakra-ui/react'
import type { Machine } from '@shared/types/api'
import { PermissionBadge } from '@features/machines/components/PermissionBadge'

export function MachineCard({ machine }: { machine: Machine }) {
  const navigate = useNavigate()
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
