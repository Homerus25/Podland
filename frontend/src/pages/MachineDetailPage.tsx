import { Heading, HStack, Tab, TabList, TabPanel, TabPanels, Tabs, Text, VStack } from '@chakra-ui/react'
import { useParams } from 'react-router-dom'
import { useMachine, useUpdateMachine } from '@features/machines/hooks/useMachines'
import { MachineForm } from '@features/machines/components/MachineForm'
import { PermissionBadge } from '@features/machines/components/PermissionBadge'
import { ErrorDisplay } from '@shared/components/ErrorDisplay'
import { getApiError } from '@shared/utils/api'
import type { Machine } from '@shared/types/api'

function TabPlaceholder({ label }: { label: string }) {
  return <Text data-testid={`tab-${label.toLowerCase()}-placeholder`}>Nothing here yet.</Text>
}

function SettingsTab({ machine }: { machine: Machine }) {
  const updateMachine = useUpdateMachine(machine.id)
  if (machine.permission === 'read') {
    return (
      <Text data-testid="settings-read-only">
        You have read-only access to this machine. Ask an owner or admin for write access to edit it.
      </Text>
    )
  }
  return (
    <VStack spacing={4} align="stretch">
      {updateMachine.isSuccess && (
        <Text color="green.500" data-testid="machine-saved">
          Changes saved.
        </Text>
      )}
      <MachineForm
        mode="edit"
        machine={machine}
        onSubmit={
          (input) =>
            new Promise<void>((resolve) =>
              updateMachine.mutate(input, { onSettled: () => resolve() }),
            )
        }
        submitError={updateMachine.error}
        isSubmitting={updateMachine.isPending}
      />
    </VStack>
  )
}

export function MachineDetailPage() {
  const { machineId } = useParams<{ machineId: string }>()
  const { data: machine, isPending, error } = useMachine(machineId!)

  if (isPending) {
    return <Text data-testid="machine-loading">Loading machine…</Text>
  }
  if (error) {
    return <ErrorDisplay error={getApiError(error)} />
  }
  if (!machine) {
    return null
  }

  return (
    <VStack spacing={4} align="stretch">
      <HStack justify="space-between">
        <Heading size="lg">{machine.name}</Heading>
        <PermissionBadge permission={machine.permission} />
      </HStack>
      <HStack spacing={4}>
        <Text fontFamily="mono" color="gray.500" data-testid="machine-endpoint">
          {machine.hostname}:{machine.port}
        </Text>
        <Text fontFamily="mono" color="gray.500" data-testid="machine-ssh-user">
          ssh user: {machine.sshUsername}
        </Text>
      </HStack>
      <Tabs>
        <TabList>
          <Tab>Containers</Tab>
          <Tab>Images</Tab>
          <Tab>Quadlets</Tab>
          <Tab>Settings</Tab>
        </TabList>
        <TabPanels>
          <TabPanel>
            <TabPlaceholder label="containers" />
          </TabPanel>
          <TabPanel>
            <TabPlaceholder label="images" />
          </TabPanel>
          <TabPanel>
            <TabPlaceholder label="quadlets" />
          </TabPanel>
          <TabPanel>
            <SettingsTab machine={machine} />
          </TabPanel>
        </TabPanels>
      </Tabs>
    </VStack>
  )
}
