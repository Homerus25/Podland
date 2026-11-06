import {
	Button,
	ListItem,
	Table,
	TableContainer,
	Tbody,
	Td,
	Text,
	Tr,
	UnorderedList,
} from "@chakra-ui/react";
import type { CapabilityGate } from "@features/hosts/hooks/useCapabilityGate";
import { PermissionHint } from "@shared/components/PermissionHint";
import { isAxiosError } from "axios";
import { Fragment, type ReactNode } from "react";

// The detail kit owns the primitives every resource detail page's overview
// composes (spec 301): the 404 predicate the pages derive their not-found
// state from, the label/value row, the key/value table, and the header's
// Remove action. One copy each — the pages adapt, they never re-declare.

// True when the overview query's error is a 404: the item no longer exists
// on the host, and the detail page renders its not-found state.
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

// The header's Remove action (spec 461, ticket #467): one owner for the gate
// cascade every resource detail page renders, so the six pages cannot drift
// into two idioms again. The capability gate's tri-state answers it — allowed
// renders the red outline button, denied the shared write-permission hint,
// pending neither (never flash the read-only hint at a write-capable caller).
// A not-found overview hides it outright whatever the gate says: there is
// nothing left to remove, and the gate alone does not cover that case. The
// absent case is `undefined`, not `null`, so the result drops straight into
// `headerActions`.
export function removalAction({
	gate,
	notFound,
	scope,
	resource,
	testId,
	onClick,
}: {
	gate: CapabilityGate;
	notFound: boolean;
	// The hint's testid scope and the resource family it names.
	scope: string;
	resource: string;
	testId: string;
	onClick: () => void;
}): ReactNode {
	if (notFound || gate === "pending") {
		return undefined;
	}
	if (gate === "denied") {
		return (
			<PermissionHint
				scope={scope}
				requirement="write"
				resource={resource}
				fontSize="sm"
			/>
		);
	}
	return (
		<Button
			size="sm"
			colorScheme="red"
			variant="outline"
			onClick={onClick}
			data-testid={testId}
		>
			Remove
		</Button>
	);
}

// One action of a detail page's header row (spec #477): the capability gate
// that answers it, the hint it renders when the gate denies, and the control
// it renders when the action is available.
export type HeaderAction = {
	// The capability gate that answers this action. Omitted for a
	// read-level action, which renders for every caller.
	gate?: CapabilityGate;
	// The hint rendered when the gate denies. Omitted when the action
	// renders no hint, or when the Remove cascade already renders the
	// same hint for the same capability (see the one-hint rule below).
	hint?: ReactNode;
	// The control rendered when the action is available.
	control: ReactNode;
};

// The header action row's one owner (spec #477): the six resource detail
// pages compose their actions here instead of hand-rolling the ritual, so
// the tri-state cascade, the read-level rule, and the empty-group contract
// cannot drift between them again. The gate's tri-state answers each action
// — omitted or allowed renders the control, denied renders the action's own
// hint if it has one, pending renders neither (never flash the read-only
// hint at a write-capable caller). A falsy control is dropped, so a page can
// self-gate a control on its overview DTO having loaded: the header slot
// renders while that query is still in flight. `remove` renders last, and
// the whole group is `undefined` — not an empty fragment — when nothing
// renders, because a bare fragment is always truthy and would leave an
// empty `detail-page-header-actions` row behind.
//
// The one-hint rule: a detail page's header renders at most one hint per
// capability. The Remove cascade already renders the page's write hint on
// denied, so an action gated on that same capability passes no `hint` —
// otherwise a denied caller reads the identical sentence twice. Only a
// genuinely different capability keeps its own hint: the container page's
// `manage-networks` connect (whose copy names networks) and the secret
// page's `reveal-secrets` admin move (Remove is write-level there).
export function headerActionGroup({
	actions,
	remove,
}: {
	actions: readonly HeaderAction[];
	remove: ReactNode;
}): ReactNode {
	// The group is a variable-length list, so each rendered node carries a
	// key: the action's position in the page's static array, and `remove`
	// last. Without them React warns about unkeyed children on every page.
	const rendered: ReactNode[] = [];
	for (const [index, action] of actions.entries()) {
		const key = `header-action-${index}`;
		if (action.gate === "pending") {
			continue;
		}
		if (action.gate === "denied") {
			if (action.hint) {
				rendered.push(<Fragment key={key}>{action.hint}</Fragment>);
			}
			continue;
		}
		if (action.control) {
			rendered.push(<Fragment key={key}>{action.control}</Fragment>);
		}
	}
	if (remove) {
		rendered.push(<Fragment key="remove">{remove}</Fragment>);
	}
	if (rendered.length === 0) {
		return undefined;
	}
	return <>{rendered}</>;
}
