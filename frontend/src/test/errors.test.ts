import { AxiosError } from "axios";
import { describe, expect, it } from "vitest";
import { axiosError, errorJson, errorResponse } from "@/test/errors";

describe("errorJson", () => {
	it("builds exactly the backend envelope, defaulting requestId to req-1", () => {
		expect(errorJson("MACHINE_NOT_FOUND", "no such machine")).toEqual({
			requestId: "req-1",
			error: { code: "MACHINE_NOT_FOUND", message: "no such machine" },
		});
	});

	it("honors a requestId override", () => {
		const envelope = errorJson("TOKEN_EXPIRED", "Token has expired", "req-7");

		expect(envelope.requestId).toBe("req-7");
	});
});

describe("errorResponse", () => {
	it("passes the status through and its body equals errorJson", async () => {
		const response = errorResponse(409, "GADGET_IN_USE", "Gadget is in use");

		expect(response.status).toBe(409);
		await expect(response.json()).resolves.toEqual(
			errorJson("GADGET_IN_USE", "Gadget is in use"),
		);
	});

	it("keeps a requestId override through to the body", async () => {
		const response = errorResponse(
			401,
			"TOKEN_EXPIRED",
			"Token has expired",
			"req-2",
		);

		expect(response.status).toBe(401);
		await expect(response.json()).resolves.toEqual(
			errorJson("TOKEN_EXPIRED", "Token has expired", "req-2"),
		);
	});
});

describe("axiosError", () => {
	it("carries the axios fields and the envelope as response.data", () => {
		const error = axiosError(404, "MACHINE_NOT_FOUND", "no such machine");

		expect(error).toBeInstanceOf(AxiosError);
		expect(error.isAxiosError).toBe(true);
		expect(error.message).toBe("Request failed with status code 404");
		expect(error.code).toBe("ERR_BAD_REQUEST");
		expect(error.response?.status).toBe(404);
		expect(error.response?.data).toEqual(
			errorJson("MACHINE_NOT_FOUND", "no such machine"),
		);
	});

	it("honors a requestId override in the response body", () => {
		const error = axiosError(409, "GADGET_IN_USE", "Gadget is in use", "req-3");

		expect(error.response?.data).toEqual(
			errorJson("GADGET_IN_USE", "Gadget is in use", "req-3"),
		);
	});
});
