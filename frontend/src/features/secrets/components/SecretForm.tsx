import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { ViewIcon, ViewOffIcon } from '@chakra-ui/icons'
import {
  Button,
  FormControl,
  FormErrorMessage,
  FormLabel,
  Input,
  InputGroup,
  InputRightElement,
  VStack,
} from '@chakra-ui/react'
import { ErrorDisplay } from '@shared/components/ErrorDisplay'
import { getApiError } from '@shared/utils/api'
import type { CreateSecretInput } from '@features/secrets/services/secretsApi'

interface SecretFormValues {
  name: string
  value: string
}

// Podman's secret name rules (backend SecretCreateRequest validation):
// letter-or-digit first, then letters, digits, underscores, dots, dashes.
const SECRET_NAME_PATTERN = /^[a-zA-Z0-9][a-zA-Z0-9_.-]*$/

const SECRET_NAME_MESSAGE =
  'Name must start with a letter or digit and contain only letters, digits, underscores, dots, or dashes'

interface SecretFormProps {
  onSubmit: (input: CreateSecretInput) => Promise<void>
  submitError: unknown
  isSubmitting: boolean
}

// Create-secret form: a name field and a masked value input with an eye
// toggle, so shoulder-surfers cannot read the value. The name is trimmed
// before submit (the backend trims it too); the value is sent byte-identical
// and never trimmed — only validation looks at its trimmed form, because a
// masked input makes an accidental whitespace-only value undetectable.
export function SecretForm({ onSubmit, submitError, isSubmitting }: SecretFormProps) {
  const [showValue, setShowValue] = useState(false)
  const {
    register,
    handleSubmit,
    formState: { errors, isSubmitting: isSubmittingForm, isValid },
  } = useForm<SecretFormValues>({
    defaultValues: { name: '', value: '' },
    // onChange so the submit button's disabled state tracks validity as
    // the user types, not only after a failed submit attempt.
    mode: 'onChange',
  })

  const submit = handleSubmit(async (values) => {
    await onSubmit({ name: values.name.trim(), value: values.value })
  })

  return (
    <form onSubmit={submit} noValidate>
      <VStack spacing={4} align="stretch">
        {submitError ? <ErrorDisplay error={getApiError(submitError)} /> : null}
        <FormControl isInvalid={!!errors.name} isRequired>
          <FormLabel htmlFor="secret-name">Name</FormLabel>
          <Input
            id="secret-name"
            fontFamily="mono"
            placeholder="db-password"
            {...register('name', {
              required: 'Name is required',
              // All name rules apply to the TRIMMED name — submit trims it,
              // so a padded-but-valid name must not be rejected for its
              // padding.
              validate: (value) => {
                const name = value.trim()
                if (name.length === 0) return 'Name is required'
                if (name.length > 255) return 'Name must be at most 255 characters'
                if (!SECRET_NAME_PATTERN.test(name)) return SECRET_NAME_MESSAGE
                return true
              },
            })}
          />
          <FormErrorMessage>{errors.name?.message}</FormErrorMessage>
        </FormControl>
        <FormControl isInvalid={!!errors.value} isRequired>
          <FormLabel htmlFor="secret-value">Value</FormLabel>
          <InputGroup>
            <Input
              id="secret-value"
              fontFamily="mono"
              type={showValue ? 'text' : 'password'}
              autoComplete="off"
              {...register('value', {
                required: 'Value is required',
                validate: (value) => value.trim().length > 0 || 'Value is required',
              })}
            />
            <InputRightElement width="2.75rem">
              <Button
                type="button"
                h="1.5rem"
                size="xs"
                p={0}
                aria-label={showValue ? 'Hide secret value' : 'Show secret value'}
                onClick={() => setShowValue((visible) => !visible)}
                data-testid="toggle-value-visibility"
              >
                {showValue ? <ViewOffIcon /> : <ViewIcon />}
              </Button>
            </InputRightElement>
          </InputGroup>
          <FormErrorMessage>{errors.value?.message}</FormErrorMessage>
        </FormControl>
        <Button
          type="submit"
          colorScheme="blue"
          isLoading={isSubmittingForm || isSubmitting}
          isDisabled={!isValid || isSubmittingForm || isSubmitting}
          data-testid="secret-form-submit"
        >
          Create secret
        </Button>
      </VStack>
    </form>
  )
}
