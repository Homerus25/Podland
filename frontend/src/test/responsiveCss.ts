// jsdom has no layout engine, so a responsive surface's small-viewport
// behavior cannot be proven by measuring the DOM — spec 393's testing
// decision is to pin the props that produce the layout instead.
// Chakra compiles responsive style props into the element's emotion
// CSS: the base declarations land in a plain rule, each wider
// breakpoint in its own @media block re-selecting the same class.
// This helper collects the generated rules for an element's own
// emotion class names; a rule inside a media query keeps its @media
// prefix so callers can tell base declarations from breakpoint
// overrides.
export function cssRulesFor(element: HTMLElement): string[] {
	const sheet = Array.from(document.querySelectorAll("style"))
		.map((tag) => tag.textContent ?? "")
		.join("");
	return element.className
		.split(/\s+/)
		.filter((cls) => cls.startsWith("css-"))
		.flatMap(
			(cls) =>
				sheet.match(
					// A plain rule (`.css-x{…}`) or the inner rule of a media
					// block (`@media …{.css-x{…}}`), keeping the prefix.
					new RegExp(
						`(?:@media[^{]*\\{)?[^{}]*\\.${cls}[^{}]*\\{[^{}]*\\}`,
						"g",
					),
				) ?? [],
		);
}

// Chakra's lg breakpoint (992px) — the spec 393 "desktop unchanged" line.
// Chakra compiles it to this min-width media query in the emotion rules.
export const LG_MEDIA_QUERY = "min-width: 62em";

// Splits an element's compiled emotion rules (cssRulesFor) into the base
// declarations and the lg overrides, so a suite can pin both halves of a
// responsive prop: the small-viewport behavior in `base`, the restored
// desktop rendering in `lg`.
export function breakpointRules(element: HTMLElement): {
	base: string;
	lg: string;
} {
	const rules = cssRulesFor(element);
	return {
		base: rules.filter((rule) => !rule.startsWith("@media")).join(""),
		lg: rules.filter((rule) => rule.includes(LG_MEDIA_QUERY)).join(""),
	};
}
