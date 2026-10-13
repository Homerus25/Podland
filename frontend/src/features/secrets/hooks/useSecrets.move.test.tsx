import { useMoveSecret } from "@features/secrets/hooks/useSecrets";
import { act, waitFor } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import { secretMoveResultFixture } from "@/test/factories";
import { renderHookWithQuery } from "@/test/render";
import { server } from "@/test/server";

// The transfer mutation's domain assertion: the request shape. The
// both-machines invalidation (a partial result is success-shaped) is the
// shared transfer-hooks module's rule, pinned once in its suite; the list
// suite pins the refetch through MSW.
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
});
