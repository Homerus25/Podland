import { useForm } from 'react-hook-form'
import {
  Button,
  FormControl,
  FormErrorMessage,
  FormLabel,
  Input,
  VStack,
} from '@chakra-ui/react'
import { ErrorDisplay } from '@shared/components/ErrorDisplay'
import { getApiError } from '@shared/utils/api'
import type { CreateMachineInput } from '@features/machines/services/machinesApi'

interface MachineFormValues {
  name: string
  description: string
  hostname: string
  port: string
  sshUsername: string
  podmanSocketPath: string
  sshPrivateKeyFile: FileList
}

const portPattern = /^(0|[1-9]\d{0,4})$/

function toPort(value: string): number {
  if (!portPattern.test(value)) return NaN
  const port = Number(value)
  return port >= 1 && port <= 65535 ? port : NaN
}

// File API read of the SSH private key into the request body. FileReader is
// used instead of File.text() for jsdom compatibility.
function readFile(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(reader.result as string)
    reader.onerror = () => reject(reader.error)
    reader.readAsText(file)
  })
}

interface MachineFormProps {
  // Returns a promise that settles when the request finishes so the form's
  // submit state spans the whole mutation, not just the file read.
  onSubmit: (input: CreateMachineInput) => Promise<void>
  submitError: unknown
  isSubmitting: boolean
}

// MachineForm in create mode: name/hostname/port/sshUsername plus the SSH
// private key file are required; description and podman socket path are
// optional. The key file is read client-side via the File API.
export function MachineForm({ onSubmit, submitError, isSubmitting }: MachineFormProps) {
  const {
    register,
    handleSubmit,
    setError,
    formState: { errors, isSubmitting: isValidatingAndReading },
  } = useForm<MachineFormValues>({
    defaultValues: { port: '22', description: '', podmanSocketPath: '' },
  })

  const submit = handleSubmit(async (values) => {
    let sshPrivateKey: string
    try {
      sshPrivateKey = await readFile(values.sshPrivateKeyFile[0])
    } catch {
      setError('sshPrivateKeyFile', { message: 'Could not read the selected key file' })
      return
    }
    await onSubmit({
      name: values.name,
      description: values.description,
      hostname: values.hostname,
      port: toPort(values.port),
      sshUsername: values.sshUsername,
      sshPrivateKey,
      podmanSocketPath: values.podmanSocketPath,
    })
  })

  return (
    <form onSubmit={submit} noValidate>
      <VStack spacing={4} align="stretch">
        {submitError ? <ErrorDisplay error={getApiError(submitError)} /> : null}
        <FormControl isInvalid={!!errors.name} isRequired>
          <FormLabel htmlFor="machine-name">Name</FormLabel>
          <Input id="machine-name" {...register('name', { required: 'Name is required' })} />
          <FormErrorMessage>{errors.name?.message}</FormErrorMessage>
        </FormControl>
        <FormControl isInvalid={!!errors.description}>
          <FormLabel htmlFor="machine-description">Description</FormLabel>
          <Input id="machine-description" {...register('description')} />
          <FormErrorMessage>{errors.description?.message}</FormErrorMessage>
        </FormControl>
        <FormControl isInvalid={!!errors.hostname} isRequired>
          <FormLabel htmlFor="machine-hostname">Hostname or IP</FormLabel>
          <Input id="machine-hostname" {...register('hostname', { required: 'Hostname is required' })} />
          <FormErrorMessage>{errors.hostname?.message}</FormErrorMessage>
        </FormControl>
        <FormControl isInvalid={!!errors.port} isRequired>
          <FormLabel htmlFor="machine-port">SSH port</FormLabel>
          <Input
            id="machine-port"
            inputMode="numeric"
            {...register('port', {
              required: 'Port is required',
              validate: (value) =>
                !Number.isNaN(toPort(value)) || 'Port must be a number between 1 and 65535',
            })}
          />
          <FormErrorMessage>{errors.port?.message}</FormErrorMessage>
        </FormControl>
        <FormControl isInvalid={!!errors.sshUsername} isRequired>
          <FormLabel htmlFor="machine-ssh-username">SSH username</FormLabel>
          <Input
            id="machine-ssh-username"
            {...register('sshUsername', { required: 'SSH username is required' })}
          />
          <FormErrorMessage>{errors.sshUsername?.message}</FormErrorMessage>
        </FormControl>
        <FormControl>
          <FormLabel htmlFor="machine-podman-socket">Podman socket path (optional)</FormLabel>
          <Input id="machine-podman-socket" {...register('podmanSocketPath')} />
          <FormErrorMessage>{errors.podmanSocketPath?.message}</FormErrorMessage>
        </FormControl>
        <FormControl isInvalid={!!errors.sshPrivateKeyFile} isRequired>
          <FormLabel htmlFor="machine-ssh-key">SSH private key</FormLabel>
          <Input
            id="machine-ssh-key"
            type="file"
            {...register('sshPrivateKeyFile', { required: 'SSH private key file is required' })}
          />
          <FormErrorMessage>{errors.sshPrivateKeyFile?.message}</FormErrorMessage>
        </FormControl>
        <Button
          type="submit"
          colorScheme="blue"
          isLoading={isValidatingAndReading || isSubmitting}
          data-testid="machine-form-submit"
        >
          Add machine
        </Button>
      </VStack>
    </form>
  )
}
