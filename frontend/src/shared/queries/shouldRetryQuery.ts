import { AxiosError } from "axios";

// React Query retries once by default, which used to doom every 4xx to a
// pointless second doomed attempt; the 401 policy owns deterministic
// client errors now. A single retry is only worth it when the failure can
// plausibly heal on its own: a network blip or a 5xx.
export function shouldRetryQuery(
	failureCount: number,
	error: unknown,
): boolean {
	if (failureCount >= 1) return false;
	if (error instanceof AxiosError) {
		if (error.response) return error.response.status >= 500;
		return true;
	}
	return false;
}
