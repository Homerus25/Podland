import { Box, Heading, VStack } from '@chakra-ui/react'
import { LoginForm } from '@features/auth/components/LoginForm'

const appName = import.meta.env.VITE_APP_NAME || 'Podland'

export function LoginPage() {
  return (
    <VStack spacing={8} justify="center" minH="100vh" px={4}>
      <Heading>{appName}</Heading>
      <Box w="full" maxW="sm">
        <LoginForm />
      </Box>
    </VStack>
  )
}
