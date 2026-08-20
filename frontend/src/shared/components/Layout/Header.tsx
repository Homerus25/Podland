import { Button, Flex, Heading, HStack, Spacer, Text } from '@chakra-ui/react'
import { useNavigate } from 'react-router-dom'
import { useAuth } from '@features/auth/hooks/useAuth'
import { ColorModeToggle } from '@shared/components/ColorModeToggle'

const appName = import.meta.env.VITE_APP_NAME || 'Podland'

export function Header() {
  const { user, logout } = useAuth()
  const navigate = useNavigate()

  const handleLogout = async () => {
    await logout()
    navigate('/login', { replace: true })
  }

  return (
    <Flex as="header" p={4} pl={6} borderBottomWidth="1px" align="center">
      <Heading size="md">{appName}</Heading>
      <Spacer />
      <HStack spacing={4}>
        <ColorModeToggle />
        {user && (
          <>
            <Text>{user.username}</Text>
            <Button onClick={handleLogout}>Log out</Button>
          </>
        )}
      </HStack>
    </Flex>
  )
}
