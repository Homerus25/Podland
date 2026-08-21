import { Heading, HStack, Tab, TabList, TabPanel, TabPanels, Tabs, Text, VStack } from '@chakra-ui/react'
import { useParams } from 'react-router-dom'
import { useMachine } from '@features/machines/hooks/useMachines'
import { PermissionBadge } from '@features/machines/components/PermissionBadge'
import { ErrorDisplay } from '@shared/components/ErrorDisplay'
import { getApiError } from '@shared/utils/api'

function TabPlaceholder({ label }: { label: string }) {
  return <Text data-testid={`tab-${label.toLowerCase()}-placeholder`}>Nothing here yet.</Text>
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
            <TabPlaceholder label="settings" />
          </TabPanel>
        </TabPanels>
      </Tabs>
    </VStack>
  )
}
