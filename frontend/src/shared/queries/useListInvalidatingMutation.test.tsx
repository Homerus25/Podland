import type { MachineResource } from "@shared/queries/queryKeys";
import {
	containersQueryKey,
	imagesQueryKey,
	quadletsQueryKey,
} from "@shared/queries/queryKeys";
import { useListInvalidatingMutation } from "@shared/queries/useListInvalidatingMutation";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

function renderMutation(
	resources: MachineResource | MachineResource[],
	run: (input: string) => Promise<string>,
) {
	const queryClient = new QueryClient({
		defaultOptions: { queries: { retry: false } },
	});
	const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");
	const wrapper = ({ children }: { children: React.ReactNode }) => (
		<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
	);
	const hook = renderHook(
		() => useListInvalidatingMutation("m-1", resources, run),
		{ wrapper },
	);
	return { ...hook, invalidateSpy };
}

describe("useListInvalidatingMutation", () => {
	it("invalidates the resource list when the mutation succeeds", async () => {
		const { result, invalidateSpy } = renderMutation("containers", (input) =>
			Promise.resolve(`ran ${input}`),
		);

		result.current.mutate("web");

		await waitFor(() => expect(result.current.isSuccess).toBe(true));
		expect(invalidateSpy).toHaveBeenCalledTimes(1);
		expect(invalidateSpy).toHaveBeenCalledWith({
			queryKey: containersQueryKey("m-1"),
		});
	});

	it("invalidates every resource when given the array form", async () => {
		const { result, invalidateSpy } = renderMutation(
			["quadlets", "containers", "images"],
			(input) => Promise.resolve(input),
		);

		result.current.mutate("web");

		await waitFor(() => expect(result.current.isSuccess).toBe(true));
		expect(invalidateSpy).toHaveBeenCalledTimes(3);
		expect(invalidateSpy).toHaveBeenCalledWith({
			queryKey: quadletsQueryKey("m-1"),
		});
		expect(invalidateSpy).toHaveBeenCalledWith({
			queryKey: containersQueryKey("m-1"),
		});
		expect(invalidateSpy).toHaveBeenCalledWith({
			queryKey: imagesQueryKey("m-1"),
		});
	});

	it("does not invalidate anything when the mutation rejects", async () => {
		const failure = new Error("boom");
		const { result, invalidateSpy } = renderMutation("containers", () =>
			Promise.reject(failure),
		);

		result.current.mutate("web");

		await waitFor(() => expect(result.current.isError).toBe(true));
		expect(invalidateSpy).not.toHaveBeenCalled();
	});

	it("returns the useMutation result untouched", async () => {
		const { result } = renderMutation("containers", (input) =>
			Promise.resolve(`ran ${input}`),
		);

		result.current.mutate("web");

		await waitFor(() => expect(result.current.isSuccess).toBe(true));
		expect(result.current.data).toBe("ran web");
		expect(result.current.error).toBeNull();
	});
});
