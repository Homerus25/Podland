import type { ApiErrorEnvelope } from "@shared/types/api";
import { AxiosError, type AxiosResponse } from "axios";
import { HttpResponse, type JsonBodyType } from "msw";

// Mirrors the backend error envelope (backend/internal/errors/handlers.go).
// The one owner of error-envelope test literals: suites build their error
// bodies, MSW responses, and axios errors from these instead of re-deciding
// the shape inline.
export function errorJson(
	code: string,
	message: string,
	requestId = "req-1",
): ApiErrorEnvelope {
	return { requestId, error: { code, message } };
}

export function errorResponse(
	status: number,
	code: string,
	message: string,
	requestId = "req-1",
): HttpResponse<JsonBodyType> {
	return HttpResponse.json(errorJson(code, message, requestId), { status });
}

export function axiosError(
	status: number,
	code: string,
	message: string,
	requestId = "req-1",
): AxiosError {
	return new AxiosError(
		`Request failed with status code ${status}`,
		AxiosError.ERR_BAD_REQUEST,
		undefined,
		undefined,
		{
			status,
			data: errorJson(code, message, requestId),
		} as unknown as AxiosResponse,
	);
}
