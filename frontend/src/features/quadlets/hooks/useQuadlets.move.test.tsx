import { useMoveQuadlet } from "@features/quadlets/hooks/useQuadlets";
import { quadletsQueryKey } from "@shared/queries/queryKeys";
import { act, waitFor } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import { afterEach, describe, expect, it, vi } from "vitest";
import { quadletMoveResultFixture } from "@/test/factories";
import { renderHookWithQuery } from "@/test/render";
import { server } from "@/test/server";

afterEach(() => {
	vi.restoreAllMocks();
});

// The transfer mutation is the one hook that invalidates across machines, so
// the invalidation targets are pinned here (the sanctioned spy, as in
// useListInvalidatingMutation.test.tsx); the dialog suite pins the request
// shapes and the list refetch through MSW.
describe("useMoveQuadlet", () => {
	it("POSTs to the source machine's move endpoint with the transfer body", async () => {
		let body: unknown;
		server.use(
			http.post("*/api/machines/m-1/quadlets/web/move", async ({ request }) => {
				body = await request.json();
				return HttpResponse.json(quadletMoveResultFixture());
			}),
		);
		const { result } = renderHookWithQuery(() => useMoveQuadlet("m-1"));

		act(() => {
			result.current.mutate({
				quadletId: "web",
				targetMachineId: "m-2",
				mode: "move",
				stop: true,
			});
		});

		await waitFor(() => expect(result.current.isSuccess).toBe(true));
		expect(body).toEqual({
			targetMachineId: "m-2",
			mode: "move",
			stop: true,
		});
	});

	it("invalidates the quadlet queries of both machines from the envelope", async () => {
		server.use(
			http.post("*/api/machines/m-1/quadlets/web/move", () =>
				HttpResponse.json(
					quadletMoveResultFixture({
						sourceMachineId: "m-1",
						targetMachineId: "m-2",
					}),
				),
			),
		);
		const { result, queryClient } = renderHookWithQuery(() =>
			useMoveQuadlet("m-1"),
		);
		const invalidate = vi.spyOn(queryClient, "invalidateQueries");

		act(() => {
			result.current.mutate({
				quadletId: "web",
				targetMachineId: "m-2",
				mode: "move",
			});
		});

		await waitFor(() => expect(result.current.isSuccess).toBe(true));
		expect(invalidate).toHaveBeenCalledWith({
			queryKey: quadletsQueryKey("m-1"),
		});
		expect(invalidate).toHaveBeenCalledWith({
			queryKey: quadletsQueryKey("m-2"),
		});
	});

	it("invalidates both machines on a partial result too — the target install stands", async () => {
		server.use(
			http.post("*/api/machines/m-1/quadlets/web/move", () =>
				HttpResponse.json(
					quadletMoveResultFixture({
						status: "partial",
						sourceMachineId: "m-1",
						targetMachineId: "m-2",
						error: "failed to remove quadlet: resource running",
					}),
				),
			),
		);
		const { result, queryClient } = renderHookWithQuery(() =>
			useMoveQuadlet("m-1"),
		);
		const invalidate = vi.spyOn(queryClient, "invalidateQueries");

		act(() => {
			result.current.mutate({
				quadletId: "web",
				targetMachineId: "m-2",
				mode: "move",
				stop: true,
			});
		});

		await waitFor(() => expect(result.current.isSuccess).toBe(true));
		expect(invalidate).toHaveBeenCalledWith({
			queryKey: quadletsQueryKey("m-1"),
		});
		expect(invalidate).toHaveBeenCalledWith({
			queryKey: quadletsQueryKey("m-2"),
		});
	});
});
