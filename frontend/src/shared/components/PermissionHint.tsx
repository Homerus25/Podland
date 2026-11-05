import { Text, type TextProps } from "@chakra-ui/react";

// The hint vocabulary, discriminated by the requirement the hidden
// control demanded — also the `data-requirement` attribute value:
// the write template names the resource family it guards, the admin
// template is the fixed secret-reveal sentence, and the owner template
// takes the verb phrase ("edit" on the settings tab).
export type PermissionHintProps = TextProps & {
	// The testid scope: the hint renders as `{scope}-permission-hint`.
	scope: string;
} & (
		| {
				requirement: "write";
				resource: string;
		  }
		| {
				requirement: "admin";
		  }
		| {
				requirement: "owner";
				phrase: string;
		  }
	);

// One owner for every permission hint: the sentence, the styling, the
// derived testid, and the data-requirement attribute. Callers add only
// local layout (margins, font size) through Text props.
export function PermissionHint(props: PermissionHintProps) {
	const sentence =
		props.requirement === "write"
			? `You have read-only access to this host. Ask an owner or admin for write access to manage its ${props.resource}s.`
			: props.requirement === "admin"
				? "Revealing the secret's value requires admin access."
				: `Only the owner can ${props.phrase} this host.`;
	const { scope, requirement, ...textProps } = props;

	return (
		<Text
			color="gray.500"
			data-testid={`${scope}-permission-hint`}
			data-requirement={requirement}
			{...textProps}
		>
			{sentence}
		</Text>
	);
}
