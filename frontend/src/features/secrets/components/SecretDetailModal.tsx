import { useState } from 'react'
import { Box, Button, HStack, Stack, Text } from '@chakra-ui/react'
import {
  useRevealSecret,
  useSecret,
} from '@features/secrets/hooks/useSecrets'
import type { SecretSummary } from '@features/secrets/services/secretsApi'
import {
  canRevealSecrets,
  type EffectivePermission,
} from '@features/machines/permissions'
import { ErrorDisplay } from '@shared/components/ErrorDisplay'
import { Modal } from '@shared/components/Modal'
import { getApiError } from '@shared/utils/api'
import { formatRelativeDate } from '@shared/utils/format'

const MASK = '••••••••••••'

function DetailField({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <Stack spacing={0}>
      <Text fontSize="xs" color="gray.500">
        {label}
      </Text>
      {children}
    </Stack>
  )
}

// The per-secret detail view (modal, matching the tab's quadlet edit
// pattern). Metadata comes from the inspect query (gcTime: 0, dropped on
// close); the value — if the user is admin and deliberately reveals — comes
// through a mutation, so React Query never caches it and it dies with the
// modal. On screen the value stays masked until a second click on the eye.
export function SecretDetailModal({
  machineId,
  secret,
  permission,
  onClose,
}: {
  machineId: string
  secret: SecretSummary
  permission: EffectivePermission
  onClose: () => void
}) {
  const detail = useSecret(machineId, secret.name)
  const reveal = useRevealSecret(machineId)
  const [isVisible, setIsVisible] = useState(false)
  const canReveal = canRevealSecrets(permission)

  const close = () => {
    reveal.reset()
    onClose()
  }

  return (
    <Modal isOpen onClose={close} title={`Secret: ${secret.name}`}>
      {detail.isPending ? (
        <Text data-testid="secret-detail-loading">Loading secret…</Text>
      ) : detail.error ? (
        <ErrorDisplay error={getApiError(detail.error)} />
      ) : (
        <Stack spacing={3}>
          <DetailField label="ID">
            <Text fontFamily="mono" data-testid="secret-detail-id">
              {detail.data.id}
            </Text>
          </DetailField>
          <DetailField label="Name">
            <Text fontFamily="mono" data-testid="secret-detail-name">
              {detail.data.name}
            </Text>
          </DetailField>
          <DetailField label="Driver">
            <Text data-testid="secret-detail-driver">{detail.data.driver}</Text>
          </DetailField>
          <DetailField label="Created">
            <Text data-testid="secret-detail-created">
              {formatRelativeDate(detail.data.createdAt)}
            </Text>
          </DetailField>
          <DetailField label="Updated">
            <Text data-testid="secret-detail-updated">
              {formatRelativeDate(detail.data.updatedAt)}
            </Text>
          </DetailField>
          {detail.data.labels && Object.keys(detail.data.labels).length > 0 ? (
            <DetailField label="Labels">
              <HStack data-testid="secret-detail-labels">
                {Object.entries(detail.data.labels).map(([key, value]) => (
                  <Text key={key} fontFamily="mono">
                    {key}={value}
                  </Text>
                ))}
              </HStack>
            </DetailField>
          ) : null}

          <Box borderTopWidth={1} pt={3}>
            {reveal.error ? (
              <Box mb={2}>
                <ErrorDisplay error={getApiError(reveal.error)} />
              </Box>
            ) : null}
            {canReveal ? (
              reveal.data ? (
                <HStack>
                  <Text
                    fontFamily="mono"
                    data-testid="secret-value"
                    data-visible={isVisible}
                  >
                    {isVisible ? reveal.data.secretData : MASK}
                  </Text>
                  <Button
                    size="xs"
                    variant="ghost"
                    aria-label={isVisible ? 'Hide value' : 'Show value'}
                    onClick={() => setIsVisible((v) => !v)}
                    data-testid="toggle-secret-visibility"
                  >
                    {isVisible ? '🙈' : '👁'}
                  </Button>
                </HStack>
              ) : (
                <Button
                  size="sm"
                  onClick={() => {
                    reveal.reset()
                    setIsVisible(false)
                    reveal.mutate(secret.name)
                  }}
                  data-testid="reveal-secret-button"
                >
                  Reveal value
                </Button>
              )
            ) : (
              <Text
                color="gray.500"
                fontSize="sm"
                data-testid="reveal-permission-hint"
              >
                Revealing the secret&apos;s value requires admin access.
              </Text>
            )}
          </Box>
        </Stack>
      )}
    </Modal>
  )
}
