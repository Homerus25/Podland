import { Center, Spinner } from '@chakra-ui/react'

export function FullPageSpinner() {
  return (
    <Center minH="100vh">
      <Spinner role="status" size="xl" thickness="4px" label="Loading" />
    </Center>
  )
}
