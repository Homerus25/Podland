import { Divider, Text, VStack } from "@chakra-ui/react";
import { navItemStyle } from "@shared/components/Layout/navItemStyle";
import { Link } from "react-router-dom";
import { HOST_TABS, useHostNavScope } from "../hostNav";

// The sidebar's host section (issue #459, prototype variant A): on a
// host-scoped route the current host's eight areas render as a flat
// second list behind a divider, styled as peers of the main nav links
// and deep-linking through ?tab=. Off a host route the section is gone
// entirely. The active tab comes from the shared scope derivation, so
// the sidebar and the header can never disagree.
export function HostNavSection({ onNavigate }: { onNavigate?: () => void }) {
	const scope = useHostNavScope();
	if (!scope) {
		return null;
	}

	return (
		<VStack spacing={1} align="stretch">
			<Divider my={4} />
			{HOST_TABS.map(({ key, label }) => {
				const active = scope.tab === key;
				return (
					<Link
						key={key}
						to={`/hosts/${encodeURIComponent(scope.hostId)}?tab=${key}`}
						onClick={onNavigate}
						aria-current={active ? "page" : undefined}
						style={{ textDecoration: "none" }}
					>
						<Text as="span" display="block" {...navItemStyle(active)}>
							{label}
						</Text>
					</Link>
				);
			})}
		</VStack>
	);
}
