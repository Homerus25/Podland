import { Table, TableContainer, Tbody, Td, Text, Th, Thead, Tr } from '@chakra-ui/react'
import { useImages } from '@features/images/hooks/useImages'
import { ErrorDisplay } from '@shared/components/ErrorDisplay'
import { getApiError } from '@shared/utils/api'
import { formatBytes, formatRelativeDate, shortImageId } from '@shared/utils/format'
import type { ImageSummary } from '@features/images/services/imagesApi'

function primaryTag(image: ImageSummary): string {
  const tag = image.repoTags.find((t) => t !== '<none>:<none>')
  return tag ?? '<none>'
}

export function ImageList({ machineId }: { machineId: string }) {
  const { data: images, isPending, error } = useImages(machineId)

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
    <TableContainer>
      <Table data-testid="images-table" variant="simple">
        <Thead>
          <Tr>
            <Th>Tag</Th>
            <Th>Image ID</Th>
            <Th isNumeric>Size</Th>
            <Th>Created</Th>
          </Tr>
        </Thead>
        <Tbody>
          {images.map((image) => (
            <Tr key={image.id} data-testid="image-row">
              <Td fontFamily="mono">{primaryTag(image)}</Td>
              <Td fontFamily="mono" color="gray.500">
                {shortImageId(image.id)}
              </Td>
              <Td isNumeric>{formatBytes(image.size)}</Td>
              <Td>{formatRelativeDate(image.createdAt)}</Td>
            </Tr>
          ))}
        </Tbody>
      </Table>
    </TableContainer>
  )
}
