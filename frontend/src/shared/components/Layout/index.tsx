import { Box, Flex } from '@chakra-ui/react'
import { Outlet } from 'react-router-dom'
import { Header } from './Header'
import { Sidebar } from './Sidebar'

export function Layout() {
  return (
    <Flex minH="100vh">
      <Sidebar />
      <Box flex="1" minW={0}>
        <Header />
        <Box as="main" p={6}>
          <Outlet />
        </Box>
      </Box>
    </Flex>
  )
}
