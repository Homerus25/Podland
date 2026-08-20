import { Heading, Text, VStack } from '@chakra-ui/react'

export function MachinesPage() {
  return (
    <VStack spacing={4} align="flex-start">
      <Heading size="lg">Machines</Heading>
      <Text color="gray.500">The read-only machines list arrives in the next slice.</Text>
    </VStack>
  )
}
