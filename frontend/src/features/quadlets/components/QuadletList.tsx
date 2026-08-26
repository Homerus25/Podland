import { useState } from 'react'
import { Button, HStack, Table, TableContainer, Tbody, Td, Text, Th, Thead, Tr } from '@chakra-ui/react'
import { useCreateQuadlet, useQuadlets } from '@features/quadlets/hooks/useQuadlets'
import { QuadletForm } from '@features/quadlets/components/QuadletForm'
import { ErrorDisplay } from '@shared/components/ErrorDisplay'
import { Modal } from '@shared/components/Modal'
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
  const createQuadlet = useCreateQuadlet(machineId)
  const [isCreateOpen, setIsCreateOpen] = useState(false)

  if (isPending) {
    return <Text data-testid="quadlets-loading">Loading quadlets…</Text>
  }

  // The create action stays reachable even on an empty or errored list —
  // creating the first quadlet on a fresh machine is the primary flow.
  return (
    <>
      <HStack justify="flex-end" mb={2}>
        <Button
          size="sm"
          colorScheme="blue"
          onClick={() => {
            createQuadlet.reset()
            setIsCreateOpen(true)
          }}
          data-testid="create-quadlet-button"
        >
          Create quadlet
        </Button>
      </HStack>
      {error ? (
        <ErrorDisplay error={getApiError(error)} />
      ) : !quadlets || quadlets.length === 0 ? (
        <Text color="gray.500" data-testid="quadlets-empty">
          No quadlets on this machine yet.
        </Text>
      ) : (
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
      )}
      {isCreateOpen ? (
        <Modal
          isOpen
          onClose={() => {
            createQuadlet.reset()
            setIsCreateOpen(false)
          }}
          title="Create quadlet"
        >
          <QuadletForm
            onSubmit={(input) =>
              new Promise<void>((resolve) =>
                createQuadlet.mutate(input, {
                  onSuccess: () => setIsCreateOpen(false),
                  onSettled: () => resolve(),
                }),
              )
            }
            submitError={createQuadlet.error}
            isSubmitting={createQuadlet.isPending}
          />
        </Modal>
      ) : null}
    </>
  )
}
