import { Alert, AlertDescription, AlertIcon, AlertTitle } from '@chakra-ui/react'
import type { ApiError } from '@shared/types/api'

export function ErrorDisplay({ error }: { error: ApiError }) {
  return (
    <Alert status="error" variant="subtle" role="alert">
      <AlertIcon />
      <AlertTitle>{error.code}</AlertTitle>
      <AlertDescription display="block">{error.message}</AlertDescription>
    </Alert>
  )
}
