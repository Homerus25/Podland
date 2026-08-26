import { useState } from 'react'
import { Box, Button, HStack, Table, TableContainer, Tbody, Td, Text, Th, Thead, Tr } from '@chakra-ui/react'
import {
  useContainers,
  useCreateContainer,
  useDeleteContainer,
  useStartContainer,
  useStopContainer,
} from '@features/containers/hooks/useContainers'
import { CreateContainerForm } from '@features/containers/components/CreateContainerForm'
import { ErrorDisplay } from '@shared/components/ErrorDisplay'
import { Modal } from '@shared/components/Modal'
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

// No delete request leaves the browser until the user explicitly confirms.
function DeleteContainerModal({
  container,
  deleteContainer,
  onClose,
}: {
  container: ContainerSummary
  deleteContainer: ReturnType<typeof useDeleteContainer>
  onClose: () => void
}) {
  const name = displayName(container)
  const confirmDelete = () => {
    deleteContainer.mutate(container.id, { onSuccess: onClose })
  }

  return (
    <Modal
      isOpen
      onClose={onClose}
      title="Delete container"
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button
            colorScheme="red"
            onClick={confirmDelete}
            isLoading={deleteContainer.isPending}
            data-testid="confirm-delete-button"
          >
            Delete container
          </Button>
        </>
      }
    >
      <Text>
        Delete “{name}” ({container.image})? This cannot be undone.
      </Text>
      {deleteContainer.error ? (
        <Box mt={2}>
          <ErrorDisplay error={getApiError(deleteContainer.error)} />
        </Box>
      ) : null}
    </Modal>
  )
}

// Each row owns its lifecycle mutation instances, so pending state and
// errors are scoped to the affected container and other rows stay usable.
function ContainerRow({
  machineId,
  container,
  onDelete,
}: {
  machineId: string
  container: ContainerSummary
  onDelete: (container: ContainerSummary) => void
}) {
  const startContainer = useStartContainer(machineId)
  const stopContainer = useStopContainer(machineId)
  const name = displayName(container)
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
        <Td>
          <Button
            size="xs"
            colorScheme="red"
            variant="outline"
            aria-label={`Delete ${name}`}
            onClick={() => onDelete(container)}
          >
            Delete
          </Button>
        </Td>
      </Tr>
      {action.lifecycle.error ? (
        <Tr>
          <Td colSpan={5}>
            <ErrorDisplay error={getApiError(action.lifecycle.error)} />
          </Td>
        </Tr>
      ) : null}
    </>
  )
}

export function ContainerList({ machineId }: { machineId: string }) {
  const { data: containers, isPending, error } = useContainers(machineId)
  const deleteContainer = useDeleteContainer(machineId)
  const createContainer = useCreateContainer(machineId)
  const [pendingDelete, setPendingDelete] = useState<ContainerSummary | null>(null)
  const [isCreateOpen, setIsCreateOpen] = useState(false)

  if (isPending) {
    return <Text data-testid="containers-loading">Loading containers…</Text>
  }

  // The create action stays reachable even on an empty or errored list —
  // creating the first container on a fresh machine is the primary flow.
  return (
    <>
      <HStack justify="flex-end" mb={2}>
        <Button
          size="sm"
          colorScheme="blue"
          onClick={() => {
            createContainer.reset()
            setIsCreateOpen(true)
          }}
          data-testid="create-container-button"
        >
          Create container
        </Button>
      </HStack>
      {error ? (
        <ErrorDisplay error={getApiError(error)} />
      ) : !containers || containers.length === 0 ? (
        <Text color="gray.500" data-testid="containers-empty">
          No containers on this machine yet.
        </Text>
      ) : (
        <TableContainer>
        <Table data-testid="containers-table" variant="simple">
          <Thead>
            <Tr>
              <Th>Name</Th>
              <Th>Image</Th>
              <Th>State</Th>
              <Th />
              <Th />
            </Tr>
          </Thead>
          <Tbody>
            {containers.map((container) => (
              <ContainerRow
                key={container.id}
                machineId={machineId}
                container={container}
                onDelete={(target) => {
                  deleteContainer.reset()
                  setPendingDelete(target)
                }}
              />
            ))}
          </Tbody>
        </Table>
      </TableContainer>
      )}
      {pendingDelete ? (
        <DeleteContainerModal
          container={pendingDelete}
          deleteContainer={deleteContainer}
          onClose={() => {
            deleteContainer.reset()
            setPendingDelete(null)
          }}
        />
      ) : null}
      {isCreateOpen ? (
        <Modal
          isOpen
          onClose={() => {
            createContainer.reset()
            setIsCreateOpen(false)
          }}
          title="Create container"
        >
          <CreateContainerForm
            onSubmit={(input) =>
              new Promise<void>((resolve) =>
                createContainer.mutate(input, {
                  onSuccess: () => setIsCreateOpen(false),
                  onSettled: () => resolve(),
                }),
              )
            }
            submitError={createContainer.error}
            isSubmitting={createContainer.isPending}
          />
        </Modal>
      ) : null}
    </>
  )
}
