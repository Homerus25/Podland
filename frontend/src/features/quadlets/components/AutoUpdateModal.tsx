import {
  Badge,
  Box,
  Button,
  Heading,
  HStack,
  List,
  ListIcon,
  ListItem,
  Spinner,
  Stack,
  Text,
} from '@chakra-ui/react'
import { CheckCircleIcon, WarningIcon, TimeIcon } from '@chakra-ui/icons'
import { Modal } from '@shared/components/Modal'
import { ErrorDisplay } from '@shared/components/ErrorDisplay'
import type { AutoUpdateItem, AutoUpdateReport } from '@features/quadlets/hooks/useQuadletAutoUpdate'
import type { UseQuadletAutoUpdateResult } from '@features/quadlets/hooks/useQuadletAutoUpdate'

// How one report line renders, keyed by the backend's AutoUpdateItem
// outcome vocabulary ("updated", "failed", "pending", "up-to-date").
function ReportItem({ item }: { item: AutoUpdateItem }) {
  const testId = `auto-update-item-${item.updated}-${item.containerName}`
  if (item.updated === 'failed') {
    return (
      <ListItem data-testid={testId} color="red.300">
        <ListIcon as={WarningIcon} color="red.400" />
        <strong>{item.containerName}</strong> ({item.imageName}) failed: {item.error ?? 'unknown error'}
      </ListItem>
    )
  }
  if (item.updated === 'pending') {
    return (
      <ListItem data-testid={testId} color="orange.300">
        <ListIcon as={TimeIcon} color="orange.400" />
        <strong>{item.containerName}</strong> has a pending update for {item.imageName}
      </ListItem>
    )
  }
  if (item.updated === 'up-to-date') {
    return (
      <ListItem data-testid={testId} color="gray.400">
        <ListIcon as={CheckCircleIcon} color="gray.500" />
        <strong>{item.containerName}</strong> ({item.imageName}) is up to date
      </ListItem>
    )
  }
  return (
    <ListItem data-testid={testId} color="green.300">
      <ListIcon as={CheckCircleIcon} color="green.400" />
      <strong>{item.containerName}</strong> updated to {item.imageName}
    </ListItem>
  )
}

function Report({ report }: { report: AutoUpdateReport }) {
  const failed = report.containers.filter((item) => item.updated === 'failed')
  if (report.containers.length === 0) {
    return <Text>No auto-update containers found on this machine.</Text>
  }
  return (
    <Stack spacing={2} mt={3}>
      {failed.length > 0 ? (
        <Text data-testid="auto-update-overall" color="red.300">
          {report.overall}
        </Text>
      ) : null}
      <List spacing={1}>
        {report.containers.map((item) => (
          <ReportItem key={`${item.containerName}-${item.updated}`} item={item} />
        ))}
      </List>
    </Stack>
  )
}

// Live view of one auto-update run. Dry runs say so outright; a terminal
// error event is a connection problem (the run never produced a report)
// and is shown apart from per-container failures.
export function AutoUpdateModal({
  onClose,
  dryRun,
  autoUpdate,
}: {
  onClose: () => void
  dryRun: boolean
  autoUpdate: UseQuadletAutoUpdateResult
}) {
  const title = dryRun ? 'Check for updates' : 'Update all'
  return (
    <Modal
      isOpen
      onClose={onClose}
      title={title}
      footer={
        <Button variant="ghost" onClick={onClose} data-testid="auto-update-close-button">
          {autoUpdate.isRunning ? 'Cancel' : 'Close'}
        </Button>
      }
    >
      <HStack>
        <Heading size="sm">{title}</Heading>
        {dryRun ? (
          <Badge colorScheme="purple" data-testid="auto-update-dry-run-badge">
            Dry run
          </Badge>
        ) : (
          <Badge colorScheme="orange" data-testid="auto-update-real-run-badge">
            Applying updates
          </Badge>
        )}
      </HStack>
      {autoUpdate.isRunning ? (
        <HStack mt={3} data-testid="auto-update-in-progress">
          <Spinner size="sm" />
          <Text>Running…</Text>
        </HStack>
      ) : null}
      {autoUpdate.lines.length > 0 ? (
        <Box
          mt={3}
          p={2}
          bg="blackAlpha.400"
          borderRadius="md"
          fontFamily="mono"
          fontSize="xs"
          maxH="200px"
          overflowY="auto"
          data-testid="auto-update-output"
        >
          {autoUpdate.lines.map((entry: { line: string }, index: number) => (
            <Text key={index} as="span" display="block" whiteSpace="pre-wrap">
              {entry.line}
            </Text>
          ))}
        </Box>
      ) : null}
      {autoUpdate.connectionError ? (
        <Box mt={3} data-testid="auto-update-connection-error">
          <ErrorDisplay error={autoUpdate.connectionError} />
        </Box>
      ) : null}
      {!autoUpdate.isRunning && autoUpdate.report ? <Report report={autoUpdate.report} /> : null}
      {!autoUpdate.isRunning && !autoUpdate.report && !autoUpdate.connectionError ? (
        <Text mt={3}>Done.</Text>
      ) : null}
    </Modal>
  )
}
