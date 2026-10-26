import { Box } from "@chakra-ui/react";
import { useEffect } from "react";
import { SidebarNav } from "./SidebarNav";

// The mobile navigation surface: an off-canvas drawer over a blocking
// overlay. Deliberately not Chakra's Drawer — that preset's Escape
// handling rides on its focus trap, and this drawer keeps none.
export function NavDrawer({
	isOpen,
	onClose,
}: {
	isOpen: boolean;
	onClose: () => void;
}) {
	// Escape closes from anywhere: with no focus trap, the key cannot ride
	// on focus inside the panel.
	useEffect(() => {
		if (!isOpen) {
			return;
		}
		const onKeyDown = (event: KeyboardEvent) => {
			if (event.key === "Escape") {
				onClose();
			}
		};
		document.addEventListener("keydown", onKeyDown);
		return () => document.removeEventListener("keydown", onKeyDown);
	}, [isOpen, onClose]);

	if (!isOpen) {
		return null;
	}

	return (
		// Below the modal z-index so mutation dialogs still cover the
		// drawer; the header sits above it (Layout), keeping the
		// hamburger clickable while the drawer is open.
		<Box pos="fixed" inset={0} zIndex="overlay">
			{/* The overlay dims the page and blocks interaction with it. */}
			<Box
				pos="absolute"
				inset={0}
				bg="blackAlpha.600"
				onClick={onClose}
				data-testid="nav-drawer-overlay"
			/>
			<Box
				pos="absolute"
				top={0}
				bottom={0}
				left={0}
				w="240px"
				p={4}
				borderRightWidth="1px"
				bg="gray.50"
				_dark={{ bg: "gray.800" }}
			>
				<SidebarNav onNavigate={onClose} />
			</Box>
		</Box>
	);
}
