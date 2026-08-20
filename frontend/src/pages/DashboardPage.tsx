import { Heading, Text, VStack } from '@chakra-ui/react'

export function DashboardPage() {
  return (
    <VStack spacing={4} align="flex-start">
      <Heading size="lg">Dashboard</Heading>
      <Text color="gray.500">Manage Podman on remote machines</Text>
    </VStack>
  )
}
