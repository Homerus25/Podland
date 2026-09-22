import { useUsers } from "@features/users/hooks/useUsers";
import { usersQueryKey } from "@shared/queries/queryKeys";
import { waitFor } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import { userFixture } from "@/test/factories";
import { renderHookWithQuery } from "@/test/render";
import { server } from "@/test/server";

describe("useUsers", () => {
	it("fetches and caches the user directory", async () => {
		const users = [userFixture(), userFixture({ id: "u-2", username: "bob" })];
		server.use(http.get("*/api/users", () => HttpResponse.json(users)));

		const { result, queryClient } = renderHookWithQuery(() => useUsers());
		await waitFor(() => expect(result.current.isSuccess).toBe(true));
		expect(result.current.data).toEqual(users);

		expect(queryClient.getQueryData(usersQueryKey)).toEqual(users);
	});
});
