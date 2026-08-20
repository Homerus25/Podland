import { useForm } from 'react-hook-form'
import { Button, FormControl, FormErrorMessage, FormLabel, Input, VStack } from '@chakra-ui/react'
import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuth } from '@features/auth/hooks/useAuth'
import { ErrorDisplay } from '@shared/components/ErrorDisplay'
import { getApiError } from '@shared/utils/api'
import type { ApiError, LoginCredentials } from '@shared/types/api'

const required = (label: string) => (value: string) =>
  value.trim().length > 0 || `${label} is required`

export function LoginForm() {
  const { login } = useAuth()
  const navigate = useNavigate()
  const [envelopeError, setEnvelopeError] = useState<ApiError | null>(null)
  const {
    register,
    handleSubmit,
    formState: { errors, isSubmitting },
  } = useForm<LoginCredentials>()

  const onSubmit = async (credentials: LoginCredentials) => {
    setEnvelopeError(null)
    try {
      await login(credentials)
      navigate('/')
    } catch (error) {
      setEnvelopeError(getApiError(error))
    }
  }

  return (
    <VStack as="form" onSubmit={handleSubmit(onSubmit)} spacing={4} align="stretch">
      {envelopeError && <ErrorDisplay error={envelopeError} />}
      <FormControl isInvalid={!!errors.username}>
        <FormLabel htmlFor="username">Username</FormLabel>
        <Input id="username" autoComplete="username" {...register('username', { validate: required('Username') })} />
        {errors.username && <FormErrorMessage>{errors.username.message}</FormErrorMessage>}
      </FormControl>
      <FormControl isInvalid={!!errors.password}>
        <FormLabel htmlFor="password">Password</FormLabel>
        <Input
          id="password"
          type="password"
          autoComplete="current-password"
          {...register('password', { validate: required('Password') })}
        />
        {errors.password && <FormErrorMessage>{errors.password.message}</FormErrorMessage>}
      </FormControl>
      <Button type="submit" isLoading={isSubmitting}>
        Log in
      </Button>
    </VStack>
  )
}
