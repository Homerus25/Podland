import type {
	ApiError,
	ApiErrorEnvelope,
	LoginResponse,
} from "@shared/types/api";
import axios, { type InternalAxiosRequestConfig } from "axios";

export const apiBaseUrl: string = import.meta.env.VITE_API_BASE_URL || "/api";

export const api = axios.create({
	baseURL: apiBaseUrl,
});

// Auth calls (login/refresh/logout) and the 401 policy's refresh ride this
// interceptor-free instance: a login failure is itself a 401, and it must
// never be readable as session death by any response-side policy.
export const bareApi = axios.create({
	baseURL: apiBaseUrl,
});

// The access token is held only in this module variable for the lifetime of
// the page — never localStorage/sessionStorage — so XSS cannot exfiltrate a
// persisted bearer credential. The request interceptor attaches it below.
let bearerToken: string | null = null;

// Session death is latched exactly once per installed token; a fresh token
// re-arms it (see setBearerToken).
let sessionExpiredFired = false;

export function setBearerToken(token: string | null) {
	bearerToken = token;
	// A fresh token means a live session (new login or successful refresh),
	// so session death from the previous token is forgiven.
	if (token !== null) {
		sessionExpiredFired = false;
	}
}

// Streaming endpoints use plain fetch (axios buffers responses), so they
// need read access to the same bearer credential.
export function getBearerToken(): string | null {
	return bearerToken;
}

api.interceptors.request.use((config) => {
	if (bearerToken) {
		config.headers.Authorization = `Bearer ${bearerToken}`;
	}
	return config;
});

// --- The 401 policy ---

// Marks a config the interceptor has already replayed once, so a second
// 401 goes straight to session death instead of another refresh. Comparing
// bearer tokens instead would misclassify a late 401 from a request sent
// with the pre-refresh token as death.
interface ReplayedConfig extends InternalAxiosRequestConfig {
	replayedAfter401?: boolean;
}

let sessionExpiredHandler: (() => void) | null = null;

// Registers the one handler notified when the session is declared dead
// (pass null to unregister). Firing is a latched no-op when nothing is
// registered, so policy unit tests need no AuthContext.
export function setSessionExpiredHandler(handler: (() => void) | null) {
	sessionExpiredHandler = handler;
}

function fireSessionExpired() {
	if (sessionExpiredFired) return;
	sessionExpiredFired = true;
	sessionExpiredHandler?.();
}

// One shared in-flight refresh for all concurrent 401s, so N simultaneous
// rejections trigger exactly one POST /auth/refresh; cleared on settle so
// a later 401 starts a fresh attempt.
let refreshInFlight: Promise<boolean> | null = null;

// The one 401 policy for every transport: resolves true only when a fresh
// token was installed (the caller replays its request once); false means
// the session is dead, the seam has already fired, and the caller surfaces
// the error. Any refresh failure — auth rejection or network — is session
// death, matching the mount-time refresh resolver.
export function handleUnauthorized(): Promise<boolean> {
	refreshInFlight ??= (async () => {
		try {
			const { data } = await bareApi.post<LoginResponse>("/auth/refresh");
			setBearerToken(data.token);
			return true;
		} catch {
			fireSessionExpired();
			return false;
		}
	})().finally(() => {
		refreshInFlight = null;
	});
	return refreshInFlight;
}

api.interceptors.response.use(undefined, async (error: unknown) => {
	if (
		axios.isAxiosError(error) &&
		error.response?.status === 401 &&
		error.config
	) {
		const config = error.config as ReplayedConfig;
		if (config.replayedAfter401) {
			fireSessionExpired();
		} else if (await handleUnauthorized()) {
			config.replayedAfter401 = true;
			return api.request(config);
		}
	}
	throw error;
});

export const genericError: ApiError = {
	code: "INTERNAL_ERROR",
	message: "An unexpected error occurred",
};

// Error codes that indicate server-internal failures. The backend already
// rewrites 5xx responses to these, but the code is what the spec keys on:
// internal codes never reach the user, whatever status they arrived with.
const internalCodePrefixes = ["INTERNAL_", "SERVICE_"];

function isInternalCode(code: string) {
	return internalCodePrefixes.some((prefix) => code.startsWith(prefix));
}

// Extracts the backend error envelope for display. Client errors (4xx) with
// a non-internal code pass their code and message through; server errors
// (5xx), internal error codes, non-envelope bodies, and network failures are
// collapsed to the generic message so internals are never leaked to users.
export function getApiError(error: unknown): ApiError {
	if (axios.isAxiosError(error)) {
		const status = error.response?.status;
		const envelope = error.response?.data as ApiErrorEnvelope | undefined;
		if (
			status !== undefined &&
			status < 500 &&
			envelope?.error?.code &&
			envelope?.error?.message &&
			!isInternalCode(envelope.error.code)
		) {
			return { code: envelope.error.code, message: envelope.error.message };
		}
	}
	return { ...genericError };
}

// Fetch-based counterpart of getApiError for streaming endpoints, which use
// plain fetch (axios buffers responses). Same rules: client errors (4xx) with
// a non-internal envelope code pass through; server errors (5xx), internal
// error codes, non-envelope bodies, and network failures collapse to the
// generic message.
export async function getApiErrorFromResponse(
	response: Response,
): Promise<ApiError> {
	const envelope = (await response
		.json()
		.catch(() => null)) as ApiErrorEnvelope | null;
	if (
		response.status < 500 &&
		envelope?.error?.code &&
		envelope.error.message &&
		!isInternalCode(envelope.error.code)
	) {
		return { code: envelope.error.code, message: envelope.error.message };
	}
	return { ...genericError };
}
