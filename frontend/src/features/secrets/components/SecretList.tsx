import { useState } from 'react'
import {
  Button,
  HStack,
  Table,
  TableContainer,
  Tbody,
  Td,
  Text,
  Th,
  Thead,
  Tr,
} from '@chakra-ui/react'
import { useCreateSecret, useSecrets } from '@features/secrets/hooks/useSecrets'
import { SecretForm } from '@features/secrets/components/SecretForm'
import { canManageSecrets, type EffectivePermission } from '@features/machines/permissions'
import { ErrorDisplay } from '@shared/components/ErrorDisplay'
import { Modal } from '@shared/components/Modal'
import { getApiError } from '@shared/utils/api'
import { formatRelativeDate } from '@shared/utils/format'

// The Secrets tab body. Metadata is visible to any grantee (the backend's
// read level); write affordances are gated on `permission`, which the page
// derives from the machine (effectivePermission) and passes down. Below
// write, an explanatory hint replaces the controls (the SettingsTab
// hide-and-hint pattern — never bait users with disabled buttons).
export function SecretList({
  machineId,
  permission,
}: {
  machineId: string
  permission: EffectivePermission
}) {
  const { data: secrets, isPending, error } = useSecrets(machineId)
  const createSecret = useCreateSecret(machineId)
  const [isCreateOpen, setIsCreateOpen] = useState(false)

  if (isPending) {
    return <Text data-testid="secrets-loading">Loading secrets…</Text>
  }

  return (
    <>
      {canManageSecrets(permission) ? (
        <HStack justify="flex-end" mb={2}>
          <Button
            size="sm"
            colorScheme="blue"
            onClick={() => {
              createSecret.reset()
              setIsCreateOpen(true)
            }}
            data-testid="create-secret-button"
          >
            Create secret
          </Button>
        </HStack>
      ) : (
        <Text
          color="gray.500"
          mb={2}
          data-testid="secrets-permission-hint"
          data-permission="read"
        >
          You have read-only access to this machine. Ask an owner or admin for write access to
          manage its secrets.
        </Text>
      )}
      {error ? (
        <ErrorDisplay error={getApiError(error)} />
      ) : !secrets || secrets.length === 0 ? (
        <Text color="gray.500" data-testid="secrets-empty">
          No secrets on this machine yet.
        </Text>
      ) : (
        <TableContainer>
          <Table data-testid="secrets-table" variant="simple">
            <Thead>
              <Tr>
                <Th>Name</Th>
                <Th>Driver</Th>
                <Th>Created</Th>
              </Tr>
            </Thead>
            <Tbody>
              {secrets.map((secret) => (
                <Tr key={secret.id} data-testid="secret-row">
                  <Td fontFamily="mono">{secret.name}</Td>
                  <Td>{secret.driver}</Td>
                  <Td>{formatRelativeDate(secret.createdAt)}</Td>
                </Tr>
              ))}
            </Tbody>
          </Table>
        </TableContainer>
      )}
      {isCreateOpen ? (
        <Modal
          isOpen
          onClose={() => {
            createSecret.reset()
            setIsCreateOpen(false)
          }}
          title="Create secret"
        >
          <SecretForm
            onSubmit={(input) =>
              new Promise<void>((resolve) =>
                createSecret.mutate(input, {
                  onSuccess: () => setIsCreateOpen(false),
                  onSettled: () => resolve(),
                }),
              )
            }
            submitError={createSecret.error}
            isSubmitting={createSecret.isPending}
          />
        </Modal>
      ) : null}
    </>
  )
}
