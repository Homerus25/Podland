import {
	Modal as ChakraModal,
	ModalBody,
	ModalCloseButton,
	ModalContent,
	ModalFooter,
	ModalHeader,
	ModalOverlay,
} from "@chakra-ui/react";
import type { ReactNode } from "react";

interface ModalProps {
	isOpen: boolean;
	onClose: () => void;
	title: string;
	children: ReactNode;
	footer?: ReactNode;
}

// Thin wrapper over the Chakra modal so later slices (delete confirmation,
// share modal) share one look and behavior. It is also the single sizing
// owner (spec 393): every dialog renders through it, so the mobile fit —
// near-full-width at base with a capped height and the body scrolling
// internally — lives here and nowhere else. The md-and-up rendering stays
// byte-identical: the base maxW override restores the default md width
// (28rem) from md on, and the centered box is untouched.
export function Modal({
	isOpen,
	onClose,
	title,
	children,
	footer,
}: ModalProps) {
	return (
		<ChakraModal
			isOpen={isOpen}
			onClose={onClose}
			isCentered
			scrollBehavior="inside"
		>
			<ModalOverlay />
			<ModalContent
				data-testid="modal"
				maxW={{ base: "calc(100vw - 2rem)", md: "md" }}
			>
				<ModalHeader>{title}</ModalHeader>
				<ModalCloseButton aria-label="Close dialog" />
				<ModalBody>{children}</ModalBody>
				{footer && <ModalFooter>{footer}</ModalFooter>}
			</ModalContent>
		</ChakraModal>
	);
}
