import { useState } from 'react'
import {
  Box,
  Button,
  Checkbox,
  FormControl,
  FormLabel,
  HStack,
  Input,
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
  useRestartQuadlet,
  useStartQuadlet,
  useStopQuadlet,
  useUpdateQuadlet,
} from '@features/quadlets/hooks/useQuadlets'
import { QuadletForm } from '@features/quadlets/components/QuadletForm'
import { ErrorDisplay } from '@shared/components/ErrorDisplay'
import { Modal } from '@shared/components/Modal'
import { StateBadge } from '@shared/components/StateBadge'
import { getApiError } from '@shared/utils/api'
import type { QuadletSummary } from '@features/quadlets/services/quadletsApi'

// Stop and restart share the backend's option set (force, timeout), so one
// modal serves both: the caller names the action and owns the mutation.
// Defaults mirror the backend's own (force off, timeout unset and therefore
// omitted from the request), so confirming untouched sends the defaults.
function LifecycleOptionsModal({
  quadlet,
  actionLabel,
  lifecycle,
  onClose,
}: {
  quadlet: QuadletSummary
  actionLabel: string
  lifecycle: ReturnType<typeof useStopQuadlet> | ReturnType<typeof useRestartQuadlet>
  onClose: () => void
}) {
  const [force, setForce] = useState(false)
  const [timeoutSeconds, setTimeoutSeconds] = useState('')

  const confirm = () => {
    const timeout = Number(timeoutSeconds)
    lifecycle.mutate(
      {
        quadletId: quadlet.id,
        force,
        // An empty input means "systemd default", so no timeout parameter
        // travels; a non-positive or otherwise unparsable value is dropped
        // for the same reason the backend drops it.
        timeout: timeout > 0 ? timeout : undefined,
      },
      { onSuccess: onClose },
    )
  }

  return (
    <Modal
      isOpen
      onClose={onClose}
      title={`${actionLabel} quadlet`}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button
            onClick={confirm}
            isLoading={lifecycle.isPending}
            data-testid="confirm-lifecycle-button"
          >
            {actionLabel} quadlet
          </Button>
        </>
      }
    >
      <Text>
        {actionLabel} “{quadlet.name}”?
      </Text>
      <VStack align="flex-start" mt={3} spacing={2}>
        <Checkbox
          isChecked={force}
          onChange={(event) => setForce(event.target.checked)}
          data-testid="lifecycle-force-option"
        >
          Force
        </Checkbox>
        <FormControl>
          <FormLabel htmlFor="lifecycle-timeout">Timeout (seconds)</FormLabel>
          <Input
            id="lifecycle-timeout"
            type="number"
            min={1}
            value={timeoutSeconds}
            onChange={(event) => setTimeoutSeconds(event.target.value)}
            placeholder="systemd default"
            data-testid="lifecycle-timeout-input"
          />
        </FormControl>
      </VStack>
      {lifecycle.error ? (
        <Box mt={2}>
          <ErrorDisplay error={getApiError(lifecycle.error)} />
        </Box>
      ) : null}
    </Modal>
  )
}

// Each row owns its lifecycle mutation instances, so pending state and
// errors are scoped to the affected quadlet and other rows stay usable.
function QuadletRow({
  machineId,
  quadlet,
  onEdit,
  onDelete,
}: {
  machineId: string
  quadlet: QuadletSummary
  onEdit: (quadlet: QuadletSummary) => void
  onDelete: (quadlet: QuadletSummary) => void
}) {
  const startQuadlet = useStartQuadlet(machineId)
  const stopQuadlet = useStopQuadlet(machineId)
  const restartQuadlet = useRestartQuadlet(machineId)
  // Stop and restart both run through the options modal; this remembers
  // which one is open (null = none).
  const [pendingAction, setPendingAction] = useState<'stop' | 'restart' | null>(null)
  // Any in-flight lifecycle action takes the row's other lifecycle buttons
  // with it, so slow systemd operations don't invite double clicks. Other
  // rows keep their own mutation instances and stay usable.
  const lifecyclePending =
    startQuadlet.isPending || stopQuadlet.isPending || restartQuadlet.isPending

  const actions = {
    stop: { label: 'Stop', mutation: stopQuadlet },
    restart: { label: 'Restart', mutation: restartQuadlet },
  }
  const openAction = pendingAction ? actions[pendingAction] : null

  const openLifecycleOptions = (action: 'stop' | 'restart') => {
    actions[action].mutation.reset()
    setPendingAction(action)
  }

  const closeLifecycleOptions = () => {
    openAction?.mutation.reset()
    setPendingAction(null)
  }

  return (
    <>
      <Tr data-testid="quadlet-row">
        <Td fontFamily="mono">{quadlet.name}</Td>
        <Td fontFamily="mono">{quadlet.resource}</Td>
        <Td>
          <StateBadge state={quadlet.state} />
        </Td>
        <Td>
          <HStack spacing={2}>
            <Button
              size="xs"
              variant="outline"
              colorScheme="green"
              aria-label={`Start ${quadlet.name}`}
              onClick={() => startQuadlet.mutate(quadlet.id)}
              isLoading={startQuadlet.isPending}
              isDisabled={lifecyclePending}
              data-testid="start-quadlet-button"
            >
              Start
            </Button>
            <Button
              size="xs"
              variant="outline"
              aria-label={`Stop ${quadlet.name}`}
              onClick={() => openLifecycleOptions('stop')}
              isDisabled={lifecyclePending}
              data-testid="stop-quadlet-button"
            >
              Stop
            </Button>
            <Button
              size="xs"
              variant="outline"
              aria-label={`Restart ${quadlet.name}`}
              onClick={() => openLifecycleOptions('restart')}
              isDisabled={lifecyclePending}
              data-testid="restart-quadlet-button"
            >
              Restart
            </Button>
          </HStack>
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
      {startQuadlet.error ? (
        <Tr>
          <Td colSpan={6}>
            <ErrorDisplay error={getApiError(startQuadlet.error)} />
          </Td>
        </Tr>
      ) : null}
      {openAction ? (
        <LifecycleOptionsModal
          quadlet={quadlet}
          actionLabel={openAction.label}
          lifecycle={openAction.mutation}
          onClose={closeLifecycleOptions}
        />
      ) : null}
    </>
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
                <Th />
              </Tr>
            </Thead>
            <Tbody>
              {quadlets.map((quadlet) => (
                <QuadletRow
                  key={quadlet.id}
                  machineId={machineId}
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
