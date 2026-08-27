import { useState } from 'react'
import {
  Box,
  Button,
  Checkbox,
  HStack,
  Table,
  TableContainer,
  Tbody,
  Td,
  Text,
  Th,
  Thead,
  Tr,
  VStack,
} from '@chakra-ui/react'
import {
  useCreateQuadlet,
  useDeleteQuadlet,
  useQuadlet,
  useQuadlets,
  useUpdateQuadlet,
} from '@features/quadlets/hooks/useQuadlets'
import { QuadletForm } from '@features/quadlets/components/QuadletForm'
import { ErrorDisplay } from '@shared/components/ErrorDisplay'
import { Modal } from '@shared/components/Modal'
import { StateBadge } from '@shared/components/StateBadge'
import { getApiError } from '@shared/utils/api'
import type { QuadletSummary } from '@features/quadlets/services/quadletsApi'

function QuadletRow({
  quadlet,
  onEdit,
  onDelete,
}: {
  quadlet: QuadletSummary
  onEdit: (quadlet: QuadletSummary) => void
  onDelete: (quadlet: QuadletSummary) => void
}) {
  return (
    <Tr data-testid="quadlet-row">
      <Td fontFamily="mono">{quadlet.name}</Td>
      <Td fontFamily="mono">{quadlet.resource}</Td>
      <Td>
        <StateBadge state={quadlet.state} />
      </Td>
      <Td>
        <Button
          size="xs"
          variant="outline"
          aria-label={`Edit ${quadlet.name}`}
          onClick={() => onEdit(quadlet)}
          data-testid="edit-quadlet-button"
        >
          Edit
        </Button>
      </Td>
      <Td>
        <Button
          size="xs"
          colorScheme="red"
          variant="outline"
          aria-label={`Delete ${quadlet.name}`}
          onClick={() => onDelete(quadlet)}
          data-testid="delete-quadlet-button"
        >
          Delete
        </Button>
      </Td>
    </Tr>
  )
}

// The editor seeds the shared quadlet form from the get endpoint. Loading
// and error states cover the fetch: the quadlet may have disappeared or the
// ssh connection may fail between listing and opening.
function EditQuadletModal({
  machineId,
  quadlet,
  updateQuadlet,
  onClose,
}: {
  machineId: string
  quadlet: QuadletSummary
  updateQuadlet: ReturnType<typeof useUpdateQuadlet>
  onClose: () => void
}) {
  const { data, isPending, error } = useQuadlet(machineId, quadlet.id)

  return (
    <Modal isOpen onClose={onClose} title={`Edit quadlet — ${quadlet.name}`}>
      {isPending ? (
        <Text data-testid="quadlet-edit-loading">Loading content…</Text>
      ) : error ? (
        <ErrorDisplay error={getApiError(error)} />
      ) : data ? (
        <QuadletForm
          mode="edit"
          seed={{ name: data.name, resource: data.resource, content: data.content }}
          onSubmit={({ content }) =>
            new Promise<void>((resolve) =>
              updateQuadlet.mutate(
                { quadletId: quadlet.id, content },
                {
                  onSuccess: onClose,
                  onSettled: () => resolve(),
                },
              ),
            )
          }
          submitError={updateQuadlet.error}
          isSubmitting={updateQuadlet.isPending}
        />
      ) : null}
    </Modal>
  )
}

// No delete request leaves the browser until the user explicitly confirms.
// Stop-first defaults to on and force to off, matching the backend
// contract's defaults, so confirming without touching anything behaves like
// the API's own defaults.
function DeleteQuadletModal({
  quadlet,
  deleteQuadlet,
  onClose,
}: {
  quadlet: QuadletSummary
  deleteQuadlet: ReturnType<typeof useDeleteQuadlet>
  onClose: () => void
}) {
  const [stopFirst, setStopFirst] = useState(true)
  const [force, setForce] = useState(false)

  const confirmDelete = () => {
    deleteQuadlet.mutate(
      { quadletId: quadlet.id, stop: stopFirst, force },
      { onSuccess: onClose },
    )
  }

  return (
    <Modal
      isOpen
      onClose={onClose}
      title="Delete quadlet"
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button
            colorScheme="red"
            onClick={confirmDelete}
            isLoading={deleteQuadlet.isPending}
            data-testid="confirm-delete-button"
          >
            Delete quadlet
          </Button>
        </>
      }
    >
      <Text>Delete “{quadlet.name}”? This cannot be undone.</Text>
      <VStack align="flex-start" mt={3} spacing={2}>
        <Checkbox
          isChecked={stopFirst}
          onChange={(event) => setStopFirst(event.target.checked)}
          data-testid="stop-first-option"
        >
          Stop the unit before deleting
        </Checkbox>
        <Checkbox
          isChecked={force}
          onChange={(event) => setForce(event.target.checked)}
          data-testid="force-option"
        >
          Force
        </Checkbox>
      </VStack>
      {deleteQuadlet.error ? (
        <Box mt={2}>
          <ErrorDisplay error={getApiError(deleteQuadlet.error)} />
        </Box>
      ) : null}
    </Modal>
  )
}

export function QuadletList({ machineId }: { machineId: string }) {
  const { data: quadlets, isPending, error } = useQuadlets(machineId)
  const createQuadlet = useCreateQuadlet(machineId)
  const updateQuadlet = useUpdateQuadlet(machineId)
  const deleteQuadlet = useDeleteQuadlet(machineId)
  const [isCreateOpen, setIsCreateOpen] = useState(false)
  const [editTarget, setEditTarget] = useState<QuadletSummary | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<QuadletSummary | null>(null)

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
                <Th />
                <Th />
              </Tr>
            </Thead>
            <Tbody>
              {quadlets.map((quadlet) => (
                <QuadletRow
                  key={quadlet.id}
                  quadlet={quadlet}
                  onEdit={setEditTarget}
                  onDelete={(target) => {
                    deleteQuadlet.reset()
                    setDeleteTarget(target)
                  }}
                />
              ))}
            </Tbody>
          </Table>
        </TableContainer>
      )}
      {editTarget ? (
        <EditQuadletModal
          machineId={machineId}
          quadlet={editTarget}
          updateQuadlet={updateQuadlet}
          onClose={() => {
            updateQuadlet.reset()
            setEditTarget(null)
          }}
        />
      ) : null}
      {deleteTarget ? (
        <DeleteQuadletModal
          quadlet={deleteTarget}
          deleteQuadlet={deleteQuadlet}
          onClose={() => {
            deleteQuadlet.reset()
            setDeleteTarget(null)
          }}
        />
      ) : null}
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
