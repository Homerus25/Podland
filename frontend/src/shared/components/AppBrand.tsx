import type { HeadingProps, StackProps } from "@chakra-ui/react";
import { Heading, HStack, Image } from "@chakra-ui/react";

const appName = import.meta.env.VITE_APP_NAME || "Podland";

// The brand row — logo beside the app title — rendered at every site
// that shows the app name: the sidebar/drawer nav, the login page, and
// the setup page. The logo is decorative (alt=""): the adjacent text
// carries the name, so the heading's accessible name stays "Podland".
// The JPEG lives in public/ so the same file also serves as the tab
// icon via the link tag in index.html.
export function AppBrand({
	headingSize,
	...rest
}: { headingSize?: HeadingProps["size"] } & StackProps) {
	return (
		<HStack spacing={3} {...rest}>
			<Image src="/podland.jpg" alt="" boxSize={8} borderRadius="md" />
			<Heading size={headingSize}>{appName}</Heading>
		</HStack>
	);
}
