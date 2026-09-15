import { shouldRetryQuery } from "@shared/queries/shouldRetryQuery";
import { AxiosError } from "axios";
import { describe, expect, it } from "vitest";

function axiosErrorWithStatus(status: number): AxiosError {
	return new AxiosError("Request failed", "ERR_BAD_REQUEST", undefined, {}, {
		status,
		data: {},
		statusText: "",
		headers: {},
		config: {},
	} as AxiosError["response"]);
}

describe("shouldRetryQuery", () => {
	it.each([400, 401, 403, 404, 409, 422])(
		"never retries a deterministic %i",
		(status) => {
			expect(shouldRetryQuery(0, axiosErrorWithStatus(status))).toBe(false);
		},
	);

	it.each([500, 502, 503])(
		"retries a transient %i once, and only once",
		(status) => {
			expect(shouldRetryQuery(0, axiosErrorWithStatus(status))).toBe(true);
			expect(shouldRetryQuery(1, axiosErrorWithStatus(status))).toBe(false);
		},
	);

	it("retries a network error (no response) once, and only once", () => {
		const networkError = new AxiosError(
			"Network Error",
			AxiosError.ERR_NETWORK,
		);
		expect(shouldRetryQuery(0, networkError)).toBe(true);
		expect(shouldRetryQuery(1, networkError)).toBe(false);
	});

	it("never retries errors it cannot classify as transient", () => {
		expect(shouldRetryQuery(0, new Error("boom"))).toBe(false);
	});
});
