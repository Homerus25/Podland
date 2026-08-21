import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Button, Card, CardBody, CardHeader, Heading, HStack, SimpleGrid, Text, VStack } from '@chakra-ui/react'
import { useCreateMachine, useMachines } from '@features/machines/hooks/useMachines'
import { MachineForm } from '@features/machines/components/MachineForm'
import { PermissionBadge } from '@features/machines/components/PermissionBadge'
import { Modal } from '@shared/components/Modal'
import type { Machine } from '@shared/types/api'
import { ErrorDisplay } from '@shared/components/ErrorDisplay'
import { getApiError } from '@shared/utils/api'

function MachineCard({ machine }: { machine: Machine }) {
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

export function MachinesPage() {
  const { data: machines, isPending, error } = useMachines()
  const createMachine = useCreateMachine()
  const [isAddOpen, setIsAddOpen] = useState(false)

  // Clearing the mutation on open/close means a stale error from a previous
  // attempt never bleeds into a fresh form.
  const openAddModal = () => {
    createMachine.reset()
    setIsAddOpen(true)
  }
  const closeAddModal = () => {
    createMachine.reset()
    setIsAddOpen(false)
  }

  return (
    <VStack spacing={4} align="stretch">
      <HStack justify="space-between">
        <Heading size="lg">Machines</Heading>
        <Button colorScheme="blue" onClick={openAddModal} data-testid="add-machine-button">
          Add Machine
        </Button>
      </HStack>
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
      <Modal isOpen={isAddOpen} onClose={closeAddModal} title="Add Machine">
        <MachineForm
          onSubmit={
            (input) =>
              new Promise<void>((resolve) =>
                createMachine.mutate(input, {
                  onSuccess: () => setIsAddOpen(false),
                  onSettled: () => resolve(),
                }),
              )
          }
          submitError={createMachine.error}
          isSubmitting={createMachine.isPending}
        />
      </Modal>
    </VStack>
  )
}
