import { useState } from 'react'
import { Button, Heading, HStack, SimpleGrid, Text, VStack } from '@chakra-ui/react'
import { useCreateMachine, useMachines } from '@features/machines/hooks/useMachines'
import type { CreateMachineInput } from '@features/machines/services/machinesApi'
import { MachineForm } from '@features/machines/components/MachineForm'
import { MachineCard } from '@features/machines/components/MachineCard'
import { Modal } from '@shared/components/Modal'
import { ErrorDisplay } from '@shared/components/ErrorDisplay'
import { getApiError } from '@shared/utils/api'
import { mutationToPromise } from '@shared/utils/mutationPromise'

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
          onSubmit={(input) =>
            mutationToPromise(
              createMachine.mutate,
              input as CreateMachineInput,
              { onSuccess: () => setIsAddOpen(false) },
            )
          }
          submitError={createMachine.error}
          isSubmitting={createMachine.isPending}
        />
      </Modal>
    </VStack>
  )
}
