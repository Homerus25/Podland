// The one owner of the nav link look: the main nav links and the host
// tab section render through this style so the two lists read as peers
// (issue #459, prototype variant A).
export function navItemStyle(active: boolean) {
	return {
		px: 3,
		py: 2,
		borderRadius: "md",
		fontWeight: active ? "semibold" : "normal",
		bg: active ? "brand.600" : "transparent",
		color: active ? "white" : "inherit",
		_hover: {
			bg: active ? "brand.700" : "gray.100",
			_dark: { bg: active ? "brand.700" : "gray.700" },
		},
	} as const;
}
