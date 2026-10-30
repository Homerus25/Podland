import { readdirSync, readFileSync } from "node:fs";
import { dirname, join, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import {
	containerQueryKey,
	containersQueryKey,
} from "@shared/queries/queryKeys";
import {
	useAlwaysRefetchListQuery,
	useOneShotQuery,
} from "@shared/queries/useFreshQuery";
import { QueryClient } from "@tanstack/react-query";
import { waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { renderHookWithQuery } from "@/test/render";

// The app-wide stale window (main.tsx): inside it a cached entry would be
// served without a refetch, so every pin below proves the flavor's own rule,
// not a staleTime of zero.
function appWideClient() {
	return new QueryClient({
		defaultOptions: { queries: { retry: false, staleTime: 5 * 60 * 1000 } },
	});
}

// gcTime 0 drops the cache entry on a macrotask after the last observer
// leaves; flush it so a remount exercises the flavor, not the timer.
async function flushGarbageCollection() {
	await new Promise((resolve) => setTimeout(resolve, 0));
}

describe("useOneShotQuery", () => {
	it("refetches on remount instead of serving the cache", async () => {
		const fetcher = vi.fn(() => Promise.resolve("current-state"));
		const queryClient = appWideClient();
		const key = containerQueryKey("m-1", "web");

		const first = renderHookWithQuery(() => useOneShotQuery(key, fetcher), {
			queryClient,
		});
		await waitFor(() =>
			expect(first.result.current.data).toBe("current-state"),
		);
		expect(fetcher).toHaveBeenCalledTimes(1);
		first.unmount();
		await flushGarbageCollection();

		const second = renderHookWithQuery(() => useOneShotQuery(key, fetcher), {
			queryClient,
		});
		expect(second.result.current.isPending).toBe(true);
		expect(second.result.current.data).toBeUndefined();

		await waitFor(() =>
			expect(second.result.current.data).toBe("current-state"),
		);
		expect(fetcher).toHaveBeenCalledTimes(2);
	});

	it("sends no request while disabled and exactly one when the gate flips", async () => {
		const fetcher = vi.fn(() => Promise.resolve("current-state"));
		const queryClient = appWideClient();
		const key = containerQueryKey("m-1", "web");
		let enabled = false;

		const hook = renderHookWithQuery(
			() => useOneShotQuery(key, fetcher, { enabled }),
			{ queryClient },
		);
		await flushGarbageCollection();
		expect(fetcher).not.toHaveBeenCalled();
		expect(hook.result.current.data).toBeUndefined();

		enabled = true;
		hook.rerender();
		await waitFor(() => expect(hook.result.current.data).toBe("current-state"));
		expect(fetcher).toHaveBeenCalledTimes(1);
	});
});

describe("useAlwaysRefetchListQuery", () => {
	it("refetches on remount inside the stale window while the cached list stays visible", async () => {
		const fetcher = vi.fn(() => Promise.resolve(["web", "db"]));
		const queryClient = appWideClient();
		const key = containersQueryKey("m-1");

		const first = renderHookWithQuery(
			() => useAlwaysRefetchListQuery(key, fetcher),
			{ queryClient },
		);
		await waitFor(() =>
			expect(first.result.current.data).toEqual(["web", "db"]),
		);
		expect(fetcher).toHaveBeenCalledTimes(1);
		first.unmount();

		const second = renderHookWithQuery(
			() => useAlwaysRefetchListQuery(key, fetcher),
			{ queryClient },
		);
		// The cached list stays visible while the fresh one lands: no
		// loading flash on re-entry.
		expect(second.result.current.isPending).toBe(false);
		expect(second.result.current.data).toEqual(["web", "db"]);

		await waitFor(() => expect(fetcher).toHaveBeenCalledTimes(2));
	});
});

// The single-ownership contract (ticket #429): the freshness vocabulary —
// the zero-gcTime option, the refetch-on-mount "always" option, and the
// enabled-defaulting gate — lives in exactly one module, this factory and
// its suite. The scan reads every source file under src/ (tests included)
// as raw text, so a hand-assembled policy fails here whether it lands in
// code or in a comment that re-teaches the mechanism, instead of hiding in
// a feature. The transfer-hooks module's preview ritual is the settled
// exception and needs no carve-out: its enabled gate is a null-target
// guard, not a freshness default, and it carries none of these options.
describe("single ownership of the freshness vocabulary", () => {
	const srcRoot = resolve(dirname(fileURLToPath(import.meta.url)), "../..");
	const ownerFiles = new Set([
		"shared/queries/useFreshQuery.ts",
		"shared/queries/useFreshQuery.test.tsx",
	]);
	const vocabulary: [label: string, pattern: RegExp][] = [
		["a zero-gcTime option", /gcTime/],
		["a refetch-on-mount option", /refetchOnMount/],
		["an enabled-defaulting gate", /enabled[^\n]*\?\? true/],
	];

	function sourceFiles(dir: string): string[] {
		return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
			const path = join(dir, entry.name);
			if (entry.isDirectory()) return sourceFiles(path);
			return /\.(ts|tsx)$/.test(entry.name) ? [path] : [];
		});
	}

	it("finds the freshness options only inside the factory", () => {
		const violations: string[] = [];
		for (const path of sourceFiles(srcRoot)) {
			const relativePath = relative(srcRoot, path);
			if (ownerFiles.has(relativePath)) continue;
			const content = readFileSync(path, "utf8");
			for (const [label, pattern] of vocabulary) {
				const match = pattern.exec(content);
				if (match) {
					const line = content.slice(0, match.index).split("\n").length;
					violations.push(`${relativePath}:${line} carries ${label}`);
				}
			}
		}

		expect(violations).toEqual([]);
	});
});
