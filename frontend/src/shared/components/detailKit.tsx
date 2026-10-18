import {
	ListItem,
	Table,
	TableContainer,
	Tbody,
	Td,
	Text,
	Tr,
	UnorderedList,
} from "@chakra-ui/react";
import { isAxiosError } from "axios";
import type { ReactNode } from "react";

// The detail kit owns the primitives every resource detail page's overview
// composes (spec 301): the 404 predicate the pages derive their not-found
// state from, the label/value row, and the key/value table. One copy each —
// the five pages adapt, they never re-declare.

// True when the overview query's error is a 404: the item no longer exists
// on the machine, and the detail page renders its not-found state.
export function isNotFound(error: unknown): boolean {
	return isAxiosError(error) && error.response?.status === 404;
}

// One label/value row of a detail page's overview table. On narrow
// viewports the row stacks: the label takes the full width and the value
// drops below it (spec 301 user story 38, mobile-width).
export function DetailRow({
	label,
	children,
}: {
	label: string;
	children: ReactNode;
}) {
	return (
		<Tr display={{ base: "block", md: "table-row" }}>
			<Td
				fontWeight="bold"
				display={{ base: "block", md: "table-cell" }}
				width={{ base: "100%", md: "40%" }}
				pb={{ base: 0, md: undefined }}
			>
				{label}
			</Td>
			<Td
				display={{ base: "block", md: "table-cell" }}
				pt={{ base: 0, md: undefined }}
			>
				{children}
			</Td>
		</Tr>
	);
}

// A list-valued row's content: the entries rendered as a bulleted list,
// or None when the list is empty or missing — the same placeholder
// convention KeyValueTable owns for maps. The optional mono flag renders
// the entries in a monospace font (the container page's mounts/networks
// rows).
export function DetailList<T>({
	items,
	getKey,
	renderItem,
	testId,
	mono,
}: {
	items: readonly T[] | undefined;
	getKey: (item: T) => string;
	renderItem: (item: T) => ReactNode;
	testId: string;
	mono?: boolean;
}) {
	if (!items || items.length === 0) {
		return <Text color="gray.500">None</Text>;
	}
	return (
		<UnorderedList spacing={1} data-testid={testId}>
			{items.map((item) => (
				<ListItem
					key={getKey(item)}
					fontFamily={mono ? "mono" : undefined}
					wordBreak="break-all"
				>
					{renderItem(item)}
				</ListItem>
			))}
		</UnorderedList>
	);
}

// A string map rendered as a monospace key/value table, or None when the
// map is empty or missing — the app has no error boundary, so a throw here
// would unmount the whole page. The optional label renders as a bold
// heading above the table (the volume page's section style); without it
// the table is cell content inside a DetailRow (the image page's style).
export function KeyValueTable({
	label,
	values,
	testId,
}: {
	label?: string;
	values: Record<string, string> | undefined;
	testId: string;
}) {
	const entries = Object.entries(values ?? {});
	return (
		<>
			{label ? (
				<Text fontWeight="bold" mt={4}>
					{label}
				</Text>
			) : null}
			{entries.length === 0 ? (
				<Text color="gray.500" data-testid={testId}>
					None
				</Text>
			) : (
				<TableContainer data-testid={testId}>
					<Table size="sm">
						<Tbody>
							{entries.map(([key, value]) => (
								<Tr key={key}>
									<Td fontFamily="mono" width="40%">
										{key}
									</Td>
									<Td fontFamily="mono">{value}</Td>
								</Tr>
							))}
						</Tbody>
					</Table>
				</TableContainer>
			)}
		</>
	);
}
