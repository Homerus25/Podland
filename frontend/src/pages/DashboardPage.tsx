import { Heading, Text, VStack } from '@chakra-ui/react'

const appName = import.meta.env.VITE_APP_NAME || 'Podland'

export function DashboardPage() {
  return (
    <VStack spacing={4} justify="center" minH="100vh">
      <Heading>{appName}</Heading>
      <Text color="gray.500">Manage Podman on remote machines</Text>
    </VStack>
  )
}
