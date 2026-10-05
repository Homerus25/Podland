import { useMoveSecret } from "@features/secrets/hooks/useSecrets";
import { secretsQueryKey } from "@shared/queries/queryKeys";
import { act, waitFor } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import { afterEach, describe, expect, it, vi } from "vitest";
import { secretMoveResultFixture } from "@/test/factories";
import { renderHookWithQuery } from "@/test/render";
import { server } from "@/test/server";

afterEach(() => {
	vi.restoreAllMocks();
});

// The transfer mutation is the one hook that invalidates across machines, so
// the invalidation targets are pinned here (the sanctioned spy, as in
// useQuadlets.move.test.tsx); the dialog suite pins the request shapes and
// the list refetch through MSW.
describe("useMoveSecret", () => {
	it("POSTs to the source machine's move endpoint with the transfer body", async () => {
		let body: unknown;
		server.use(
			http.post(
				"*/api/machines/m-1/secrets/db-password/move",
				async ({ request }) => {
					body = await request.json();
					return HttpResponse.json(secretMoveResultFixture());
				},
			),
		);
		const { result } = renderHookWithQuery(() => useMoveSecret("m-1"));

		act(() => {
			result.current.mutate({
				secretId: "db-password",
				targetMachineId: "m-2",
				mode: "move",
			});
		});

		await waitFor(() => expect(result.current.isSuccess).toBe(true));
		expect(body).toEqual({ targetMachineId: "m-2", mode: "move" });
	});

	it("invalidates the secret queries of both machines from the envelope", async () => {
		server.use(
			http.post("*/api/machines/m-1/secrets/db-password/move", () =>
				HttpResponse.json(
					secretMoveResultFixture({
						sourceMachineId: "m-1",
						targetMachineId: "m-2",
					}),
				),
			),
		);
		const { result, queryClient } = renderHookWithQuery(() =>
			useMoveSecret("m-1"),
		);
		const invalidate = vi.spyOn(queryClient, "invalidateQueries");

		act(() => {
			result.current.mutate({
				secretId: "db-password",
				targetMachineId: "m-2",
				mode: "move",
			});
		});

		await waitFor(() => expect(result.current.isSuccess).toBe(true));
		expect(invalidate).toHaveBeenCalledWith({
			queryKey: secretsQueryKey("m-1"),
		});
		expect(invalidate).toHaveBeenCalledWith({
			queryKey: secretsQueryKey("m-2"),
		});
	});

	it("invalidates both machines on a partial result too — the value crossed", async () => {
		server.use(
			http.post("*/api/machines/m-1/secrets/db-password/move", () =>
				HttpResponse.json(
					secretMoveResultFixture({
						status: "partial",
						sourceMachineId: "m-1",
						targetMachineId: "m-2",
						error: "failed to delete secret: secret is in use",
					}),
				),
			),
		);
		const { result, queryClient } = renderHookWithQuery(() =>
			useMoveSecret("m-1"),
		);
		const invalidate = vi.spyOn(queryClient, "invalidateQueries");

		act(() => {
			result.current.mutate({
				secretId: "db-password",
				targetMachineId: "m-2",
				mode: "move",
			});
		});

		await waitFor(() => expect(result.current.isSuccess).toBe(true));
		expect(invalidate).toHaveBeenCalledWith({
			queryKey: secretsQueryKey("m-1"),
		});
		expect(invalidate).toHaveBeenCalledWith({
			queryKey: secretsQueryKey("m-2"),
		});
	});
});
