import { useMoveQuadlet } from "@features/quadlets/hooks/useQuadlets";
import { act, waitFor } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import { quadletMoveResultFixture } from "@/test/factories";
import { renderHookWithQuery } from "@/test/render";
import { server } from "@/test/server";

// The transfer mutation's domain assertion: the request shape. The
// both-hosts invalidation (a partial result is success-shaped) is the
// shared transfer-hooks module's rule, pinned once in its suite; the list
// suite pins the refetch through MSW.
describe("useMoveQuadlet", () => {
	it("POSTs to the source host's move endpoint with the transfer body", async () => {
		let body: unknown;
		server.use(
			http.post("*/api/hosts/m-1/quadlets/web/move", async ({ request }) => {
				body = await request.json();
				return HttpResponse.json(quadletMoveResultFixture());
			}),
		);
		const { result } = renderHookWithQuery(() => useMoveQuadlet("m-1"));

		act(() => {
			result.current.mutate({
				quadletId: "web",
				targetHostId: "m-2",
				mode: "move",
				stop: true,
			});
		});

		await waitFor(() => expect(result.current.isSuccess).toBe(true));
		expect(body).toEqual({
			targetHostId: "m-2",
			mode: "move",
			stop: true,
		});
	});
});
