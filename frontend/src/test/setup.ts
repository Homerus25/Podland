import "@testing-library/jest-dom/vitest";
import { transferableAbortController } from "node:util";
import { cleanup } from "@testing-library/react";
import { afterAll, afterEach, beforeAll } from "vitest";
import { server } from "./server";

// Node >= 24 (undici 7) rejects fetch signals that fail its instanceof brand
// check (jsdom's AbortSignal), while DOM listeners (framer-motion) reject
// Node's. Leave the globals to jsdom and relay foreign signals to a Node one.
// Installed after server.listen so it wraps MSW's interceptor instead of
// sitting underneath it.
const NodeAbortSignal = transferableAbortController().signal
	.constructor as unknown as typeof AbortSignal;

function relayForeignAbortSignals() {
	const nodeFetch = globalThis.fetch;
	globalThis.fetch = ((input: RequestInfo | URL, init?: RequestInit) => {
		const { signal } = init ?? {};
		if (!signal || signal instanceof NodeAbortSignal) {
			return nodeFetch(input, init);
		}
		const relay = transferableAbortController();
		const foreign = signal as AbortSignal;
		if (foreign.aborted) {
			relay.abort(foreign.reason);
		} else {
			foreign.addEventListener("abort", () => relay.abort(foreign.reason));
		}
		return nodeFetch(input, { ...init, signal: relay.signal });
	}) as typeof fetch;
}

// jsdom does not implement matchMedia; Chakra's color-mode provider needs it.
Object.defineProperty(window, "matchMedia", {
	writable: true,
	value: (query: string) => ({
		matches: false,
		media: query,
		onchange: null,
		addListener: () => {},
		removeListener: () => {},
		addEventListener: () => {},
		removeEventListener: () => {},
		dispatchEvent: () => false,
	}),
});

// jsdom does not implement scrolling; Chakra's menu scrolls its list into
// view on open.
Element.prototype.scrollTo = () => {};

beforeAll(() => {
	server.listen({ onUnhandledRequest: "error" });
	relayForeignAbortSignals();
});
afterEach(() => {
	cleanup();
	server.resetHandlers();
});
afterAll(() => server.close());
