import { Box, Text, VStack } from '@chakra-ui/react'
import { NavLink } from 'react-router-dom'

const navItems = [
  { label: 'Dashboard', to: '/', end: true },
  { label: 'Machines', to: '/machines' },
]

export function Sidebar() {
  return (
    <Box as="nav" w="240px" flexShrink={0} p={4} borderRightWidth="1px">
      <VStack spacing={1} align="stretch">
        {navItems.map((item) => (
          <NavLink key={item.to} to={item.to} end={item.end} style={{ textDecoration: 'none' }}>
            {({ isActive }) => (
              <Text
                as="span"
                display="block"
                px={3}
                py={2}
                borderRadius="md"
                fontWeight={isActive ? 'semibold' : 'normal'}
                bg={isActive ? 'brand.600' : 'transparent'}
                color={isActive ? 'white' : 'inherit'}
                _hover={{ bg: isActive ? 'brand.700' : 'gray.100', _dark: { bg: isActive ? 'brand.700' : 'gray.700' } }}
              >
                {item.label}
              </Text>
            )}
          </NavLink>
        ))}
      </VStack>
    </Box>
  )
}
