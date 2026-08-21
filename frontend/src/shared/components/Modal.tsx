import {
  Modal as ChakraModal,
  ModalBody,
  ModalCloseButton,
  ModalContent,
  ModalFooter,
  ModalHeader,
  ModalOverlay,
} from '@chakra-ui/react'
import type { ReactNode } from 'react'

interface ModalProps {
  isOpen: boolean
  onClose: () => void
  title: string
  children: ReactNode
  footer?: ReactNode
}

// Thin wrapper over the Chakra modal so later slices (delete confirmation,
// share modal) share one look and behavior.
export function Modal({ isOpen, onClose, title, children, footer }: ModalProps) {
  return (
    <ChakraModal isOpen={isOpen} onClose={onClose} isCentered>
      <ModalOverlay />
      <ModalContent data-testid="modal">
        <ModalHeader>{title}</ModalHeader>
        <ModalCloseButton aria-label="Close dialog" />
        <ModalBody>{children}</ModalBody>
        {footer && <ModalFooter>{footer}</ModalFooter>}
      </ModalContent>
    </ChakraModal>
  )
}
