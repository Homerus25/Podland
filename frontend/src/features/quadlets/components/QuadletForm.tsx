import { useForm } from 'react-hook-form'
import {
  Button,
  FormControl,
  FormErrorMessage,
  FormLabel,
  Input,
  Select,
  Textarea,
  VStack,
} from '@chakra-ui/react'
import { ErrorDisplay } from '@shared/components/ErrorDisplay'
import { getApiError } from '@shared/utils/api'
import {
  QUADLET_RESOURCES,
  type CreateQuadletInput,
  type UpdateQuadletInput,
} from '@features/quadlets/services/quadletsApi'

interface QuadletFormValues {
  name: string
  resource: string
  content: string
}

// Values the edit flow seeds the form with from the quadlet get endpoint.
export interface QuadletFormSeed {
  name: string
  resource: string
  content: string
}

interface QuadletFormSharedProps {
  submitError: unknown
  isSubmitting: boolean
}

// Create mode: name, resource type, and content are all editable and all
// three are submitted.
interface QuadletCreateProps extends QuadletFormSharedProps {
  mode?: 'create'
  onSubmit: (input: CreateQuadletInput) => Promise<void>
}

// Edit mode: the backend derives the name from the URL and the resource type
// from the existing file path on update, so both render read-only, only the
// content textarea is editable, and only the content is submitted.
interface QuadletEditProps extends QuadletFormSharedProps {
  mode: 'edit'
  seed: QuadletFormSeed
  onSubmit: (input: UpdateQuadletInput) => Promise<void>
}

export type QuadletFormProps = QuadletCreateProps | QuadletEditProps

// Create/edit quadlet form: name, resource type, and raw INI content in a
// plain monospace textarea. The edit flow reuses this component rather than
// duplicating the layout.
export function QuadletForm(props: QuadletFormProps) {
  const { submitError, isSubmitting } = props
  const isEdit = props.mode === 'edit'
  const {
    register,
    handleSubmit,
    formState: { errors, isSubmitting: isSubmittingForm },
  } = useForm<QuadletFormValues>({
    defaultValues: isEdit ? props.seed : { name: '', resource: '', content: '' },
  })

  const submit = handleSubmit(async (values) => {
    if (props.mode === 'edit') {
      await props.onSubmit({ content: values.content })
      return
    }
    // The select only offers QUADLET_RESOURCES; narrow the form string back
    // to the typed payload field.
    const resource = QUADLET_RESOURCES.find((candidate) => candidate === values.resource)
    if (!resource) return
    await props.onSubmit({
      name: values.name.trim(),
      resource,
      content: values.content,
    })
  })

  return (
    <form onSubmit={submit} noValidate>
      <VStack spacing={4} align="stretch">
        {submitError ? <ErrorDisplay error={getApiError(submitError)} /> : null}
        {isEdit ? (
          <FormControl>
            <FormLabel htmlFor="quadlet-name">Name</FormLabel>
            <Input id="quadlet-name" isReadOnly fontFamily="mono" {...register('name')} />
          </FormControl>
        ) : (
          <FormControl isInvalid={!!errors.name} isRequired>
            <FormLabel htmlFor="quadlet-name">Name</FormLabel>
            <Input
              id="quadlet-name"
              placeholder="web (without .container extension)"
              {...register('name', {
                required: 'Name is required',
                validate: (value) => value.trim().length > 0 || 'Name is required',
              })}
            />
            <FormErrorMessage>{errors.name?.message}</FormErrorMessage>
          </FormControl>
        )}
        {isEdit ? (
          <FormControl>
            <FormLabel htmlFor="quadlet-resource">Resource type</FormLabel>
            <Input id="quadlet-resource" isReadOnly fontFamily="mono" {...register('resource')} />
          </FormControl>
        ) : (
          <FormControl isInvalid={!!errors.resource} isRequired>
            <FormLabel htmlFor="quadlet-resource">Resource type</FormLabel>
            <Select
              id="quadlet-resource"
              placeholder="Select resource type"
              {...register('resource', { required: 'Resource type is required' })}
            >
              {QUADLET_RESOURCES.map((resource) => (
                <option key={resource} value={resource}>
                  {resource}
                </option>
              ))}
            </Select>
            <FormErrorMessage>{errors.resource?.message}</FormErrorMessage>
          </FormControl>
        )}
        <FormControl isInvalid={!!errors.content} isRequired>
          <FormLabel htmlFor="quadlet-content">Content</FormLabel>
          <Textarea
            id="quadlet-content"
            fontFamily="mono"
            rows={12}
            placeholder={'[Container]\nImage=docker.io/library/nginx:1.27'}
            {...register('content', {
              required: 'Content is required',
              validate: (value) => value.trim().length > 0 || 'Content is required',
            })}
          />
          <FormErrorMessage>{errors.content?.message}</FormErrorMessage>
        </FormControl>
        <Button
          type="submit"
          colorScheme="blue"
          isLoading={isSubmittingForm || isSubmitting}
          data-testid="quadlet-form-submit"
        >
          {isEdit ? 'Save changes' : 'Create quadlet'}
        </Button>
      </VStack>
    </form>
  )
}
