import { Badge } from '@chakra-ui/react'
import type { Machine } from '@shared/types/api'

export function PermissionBadge({ permission }: { permission: Machine['permission'] }) {
  const colorScheme = permission === 'admin' ? 'purple' : permission === 'write' ? 'blue' : 'gray'
  return (
    <Badge colorScheme={colorScheme} data-testid="machine-permission">
      {permission}
    </Badge>
  )
}
