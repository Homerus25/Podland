import { Box } from "@chakra-ui/react";
import { SidebarNav } from "./SidebarNav";

// The desktop navigation surface: rendered only at the lg breakpoint and
// above (the Layout owns that switch), always visible, no collapse toggle.
export function Sidebar() {
	return (
		<Box w="240px" flexShrink={0} p={4} borderRightWidth="1px">
			<SidebarNav />
		</Box>
	);
}
