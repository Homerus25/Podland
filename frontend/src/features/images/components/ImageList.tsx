import { useState } from 'react'
import { Button, Table, TableContainer, Tbody, Td, Text, Th, Thead, Tr } from '@chakra-ui/react'
import { useDeleteImage, useImages } from '@features/images/hooks/useImages'
import { PullImageForm } from '@features/images/components/PullImageForm'
import { ErrorDisplay } from '@shared/components/ErrorDisplay'
import { Modal } from '@shared/components/Modal'
import { getApiError } from '@shared/utils/api'
import { formatBytes, formatRelativeDate, shortImageId } from '@shared/utils/format'
import type { ImageSummary } from '@features/images/services/imagesApi'

function imageTags(image: ImageSummary): string[] {
  const tags = (image.repoTags ?? []).filter((tag) => tag !== '<none>:<none>')
  return tags.length > 0 ? tags : ['<none>']
}

function displayTags(image: ImageSummary): string {
  return imageTags(image).join(', ')
}

const USED_BY_CONTAINERS_CODE = 'IMAGE_USED_BY_CONTAINERS'

function isUsedByContainers(error: unknown): boolean {
  return getApiError(error).code === USED_BY_CONTAINERS_CODE
}

function DeleteImageModal({
  image,
  onClose,
  deleteImage,
}: {
  image: ImageSummary
  onClose: () => void
  deleteImage: ReturnType<typeof useDeleteImage>
}) {
  const tags = displayTags(image)
  const inUse = isUsedByContainers(deleteImage.error)

  const requestDelete = (force: boolean) => {
    deleteImage.mutate(
      { imageId: image.id, force },
      { onSuccess: onClose },
    )
  }

  return (
    <Modal
      isOpen
      onClose={onClose}
      title="Delete image"
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          {inUse ? null : (
            <Button
              colorScheme="red"
              onClick={() => requestDelete(false)}
              isLoading={deleteImage.isPending}
              data-testid="confirm-delete-button"
            >
              Delete image
            </Button>
          )}
          {inUse ? (
            <Button
              colorScheme="red"
              variant="outline"
              onClick={() => requestDelete(true)}
              isLoading={deleteImage.isPending}
              data-testid="force-delete-button"
            >
              Force delete
            </Button>
          ) : null}
        </>
      }
    >
      <Text>
        Delete {tags} ({shortImageId(image.id)}) from this machine? This cannot be undone.
      </Text>
      {inUse ? (
        <Text mt={2} color="orange.500" data-testid="image-in-use-warning">
          One or more containers use this image, so it cannot be deleted normally. Force delete
          removes the image even though containers still use it; those containers may stop working.
        </Text>
      ) : null}
      {deleteImage.error && !inUse ? (
        <ErrorDisplay error={getApiError(deleteImage.error)} />
      ) : null}
    </Modal>
  )
}

export function ImageList({ machineId }: { machineId: string }) {
  const { data: images, isPending, error } = useImages(machineId)
  const deleteImage = useDeleteImage(machineId)
  const [pendingDelete, setPendingDelete] = useState<ImageSummary | null>(null)

  if (isPending) {
    return <Text data-testid="images-loading">Loading images…</Text>
  }
  if (error) {
    return <ErrorDisplay error={getApiError(error)} />
  }
  if (!images || images.length === 0) {
    return (
      <Text color="gray.500" data-testid="images-empty">
        No images on this machine yet. Pull one to get started.
      </Text>
    )
  }

  return (
    <>
      <PullImageForm machineId={machineId} />
      <TableContainer>
        <Table data-testid="images-table" variant="simple">
          <Thead>
            <Tr>
              <Th>Tag</Th>
              <Th>Image ID</Th>
              <Th isNumeric>Size</Th>
              <Th>Created</Th>
              <Th />
            </Tr>
          </Thead>
          <Tbody>
            {images.map((image) => (
              <Tr key={image.id} data-testid="image-row">
                <Td fontFamily="mono">{displayTags(image)}</Td>
                <Td fontFamily="mono" color="gray.500">
                  {shortImageId(image.id)}
                </Td>
                <Td isNumeric>{formatBytes(image.size)}</Td>
                <Td>{formatRelativeDate(image.createdAt)}</Td>
                <Td>
                  <Button
                    size="xs"
                    colorScheme="red"
                    variant="outline"
                    aria-label={`Delete ${displayTags(image)}`}
                    onClick={() => {
                      deleteImage.reset()
                      setPendingDelete(image)
                    }}
                  >
                    Delete
                  </Button>
                </Td>
              </Tr>
            ))}
          </Tbody>
        </Table>
      </TableContainer>
      {pendingDelete ? (
        <DeleteImageModal
          image={pendingDelete}
          onClose={() => {
            deleteImage.reset()
            setPendingDelete(null)
          }}
          deleteImage={deleteImage}
        />
      ) : null}
    </>
  )
}
