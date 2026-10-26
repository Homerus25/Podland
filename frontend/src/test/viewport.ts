import { desktopNavQuery } from "@shared/components/Layout";

// jsdom has no layout, so the shell's breakpoint switch — the layout's
// 1024px media query — has no real viewport to read. This mock stands in
// for the viewport; resize() fires the media-query listeners Chakra's
// useMediaQuery subscribes through, standing in for dragging the window
// across the breakpoint.
type MockMediaQueryListener = (event: {
	media: string;
	matches: boolean;
}) => void;

export function mockViewport(desktop: boolean) {
	const listeners = new Set<MockMediaQueryListener>();
	window.matchMedia = ((query: string) => ({
		matches: desktop && query === desktopNavQuery,
		media: query,
		onchange: null,
		addListener: (listener: MockMediaQueryListener) => listeners.add(listener),
		removeListener: (listener: MockMediaQueryListener) =>
			listeners.delete(listener),
		addEventListener: (_type: string, listener: MockMediaQueryListener) =>
			listeners.add(listener),
		removeEventListener: (_type: string, listener: MockMediaQueryListener) =>
			listeners.delete(listener),
		dispatchEvent: () => false,
	})) as unknown as typeof window.matchMedia;
	return {
		resize(toDesktop: boolean) {
			desktop = toDesktop;
			const event = { media: desktopNavQuery, matches: toDesktop };
			for (const listener of listeners) listener(event);
		},
	};
}
