import { useState } from 'react'
import {
  Button,
  FormControl,
  FormLabel,
  HStack,
  Input,
  Select,
  Text,
  VStack,
} from '@chakra-ui/react'
import {
  useGrantMachineAccess,
  useMachineAccess,
  useRevokeMachineAccess,
} from '@features/machines/hooks/useMachines'
import { PermissionBadge } from '@features/machines/components/PermissionBadge'
import { ErrorDisplay } from '@shared/components/ErrorDisplay'
import { getApiError } from '@shared/utils/api'
import type { MachinePermission } from '@shared/types/api'

function formatGrantedAt(iso: string): string {
  return new Date(iso).toLocaleString()
}

// Sharing management for one machine: the current grants plus grant/revoke
// actions. The backend identifies users by id, so grants are shown by user id.
export function ShareMachineSection({ machineId }: { machineId: string }) {
  const { data: grants, isPending, error } = useMachineAccess(machineId)
  const grantAccess = useGrantMachineAccess(machineId)
  const revokeAccess = useRevokeMachineAccess(machineId)
  const [userId, setUserId] = useState('')
  const [permission, setPermission] = useState<MachinePermission>('read')

  const submitGrant = () => {
    grantAccess.reset()
    grantAccess.mutate({ userId: userId.trim(), permission })
  }

  return (
    <VStack spacing={4} align="stretch" data-testid="share-machine-section">
      {error ? <ErrorDisplay error={getApiError(error)} /> : null}
      {isPending ? (
        <Text data-testid="grants-loading">Loading access grants…</Text>
      ) : (
        <VStack spacing={2} align="stretch">
          {grants?.map((grant) => (
            <HStack key={grant.userId} justify="space-between" data-testid={`grant-row-${grant.userId}`}>
              <HStack spacing={4}>
                <Text fontFamily="mono">{grant.userId}</Text>
                <PermissionBadge permission={grant.permission} />
                <Text color="gray.500" fontSize="sm">
                  granted {formatGrantedAt(grant.grantedAt)}
                </Text>
              </HStack>
              <Button
                size="xs"
                colorScheme="red"
                variant="ghost"
                isLoading={
                  revokeAccess.isPending && revokeAccess.variables === grant.userId
                }
                onClick={() => {
                  revokeAccess.reset()
                  revokeAccess.mutate(grant.userId)
                }}
                data-testid={`revoke-access-${grant.userId}`}
              >
                Revoke
              </Button>
            </HStack>
          ))}
        </VStack>
      )}
      {revokeAccess.error ? <ErrorDisplay error={getApiError(revokeAccess.error)} /> : null}
      <HStack spacing={4} as="form" onSubmit={(event) => { event.preventDefault(); submitGrant() }}>
        <FormControl>
          <FormLabel htmlFor="grant-user-id">User id</FormLabel>
          <Input
            id="grant-user-id"
            value={userId}
            onChange={(event) => setUserId(event.target.value)}
            placeholder="u-…"
          />
        </FormControl>
        <FormControl>
          <FormLabel htmlFor="grant-permission">Permission</FormLabel>
          <Select
            id="grant-permission"
            value={permission}
            onChange={(event) => setPermission(event.target.value as MachinePermission)}
          >
            <option value="read">read</option>
            <option value="write">write</option>
            <option value="admin">admin</option>
          </Select>
        </FormControl>
        <Button type="submit" colorScheme="blue" isLoading={grantAccess.isPending} data-testid="grant-access-button">
          Grant access
        </Button>
      </HStack>
      {grantAccess.error ? <ErrorDisplay error={getApiError(grantAccess.error)} /> : null}
    </VStack>
  )
}
