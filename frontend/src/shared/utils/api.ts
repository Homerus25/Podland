import type {
	ApiError,
	ApiErrorEnvelope,
	BulkRemoveEntry,
	BulkRemoveInput,
	LoginResponse,
} from "@shared/types/api";
import axios, { type InternalAxiosRequestConfig } from "axios";

const apiBaseUrl: string = import.meta.env.VITE_API_BASE_URL || "/api";

export const api = axios.create({
	baseURL: apiBaseUrl,
});

// Auth calls (login/logout) and refreshSession ride this interceptor-free
// instance: a login failure is itself a 401, and it must never be readable
// as session death by any response-side policy.
export const bareApi = axios.create({
	baseURL: apiBaseUrl,
});

// The single owner of host-scoped URL construction: every path segment,
// hostId first, is percent-encoded. Axios calls take the path form (the
// shared instance prepends the base); fetch consumers (stream transport,
// downloadFile) take hostApiUrl, the full form.
export function hostApiPath(hostId: string, ...segments: string[]): string {
	const encoded = [hostId, ...segments].map(encodeURIComponent);
	return `/hosts/${encoded.join("/")}`;
}

export function hostApiUrl(hostId: string, ...segments: string[]): string {
	return `${apiBaseUrl.replace(/\/$/, "")}${hostApiPath(hostId, ...segments)}`;
}

// The one owner of the bulk-remove request body (backend
// internal/bulkremove Request): the ids, plus the force flag only when it is
// set. The first pass is always plain on the wire — force omitted, never
// false — which is what the backend's `Force bool` default documents, and
// the report's retry is the only caller that sets it. Every resource's
// POST .../remove builds its body here so the seven services cannot drift.
export function bulkRemoveBody({
	ids,
	force,
}: BulkRemoveInput): BulkRemoveInput {
	return force ? { ids, force } : { ids };
}

// The one owner of the bulk-remove POST itself: the .../remove path, the
// body, and the report extraction. Each feature's service names it with
// its domain segment and nothing else, so the seven POSTs cannot drift.
export async function bulkRemove(
	hostId: string,
	resource: string,
	input: BulkRemoveInput,
): Promise<BulkRemoveEntry[]> {
	const response = await api.post<BulkRemoveEntry[]>(
		hostApiPath(hostId, resource, "remove"),
		bulkRemoveBody(input),
	);
	return response.data;
}

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

// Declares the session dead: latched exactly once per installed token, and
// the registered handler (if any) fires once. Exported for the stream
// transport, whose marked replay is loop state instead of an axios config
// flag, so its second 401 needs to declare death directly.
export function fireSessionExpired() {
	if (sessionExpiredFired) return;
	sessionExpiredFired = true;
	sessionExpiredHandler?.();
}

// One shared in-flight refresh for all callers — the mount-time resolver
// and the 401 policy alike — so N simultaneous callers trigger exactly one
// POST /auth/refresh; cleared on settle so a later caller starts a fresh
// attempt.
let refreshInFlight: Promise<LoginResponse | null> | null = null;

// The one owner of "refresh the session": exchanges the httpOnly refresh
// cookie for a fresh access JWT, installs the token itself, and resolves
// the payload. Any failure — auth rejection or network — resolves null and
// never throws. Failure here is not session death: the mount caller reads
// it as logged-out, so firing the seam is the 401 policy's decision alone.
export function refreshSession(): Promise<LoginResponse | null> {
	refreshInFlight ??= (async () => {
		try {
			const { data } = await bareApi.post<LoginResponse>("/auth/refresh");
			setBearerToken(data.token);
			return data;
		} catch {
			return null;
		}
	})().finally(() => {
		refreshInFlight = null;
	});
	return refreshInFlight;
}

// The one 401 policy for every transport: resolves true only when a fresh
// token was installed (the caller replays its request once); false means
// the session is dead, the seam has already fired, and the caller surfaces
// the error. Any refresh failure — auth rejection or network — is session
// death here, unlike the mount-time caller, whose failure is just
// logged-out.
export function handleUnauthorized(): Promise<boolean> {
	return refreshSession().then((payload) => {
		if (payload === null) {
			fireSessionExpired();
			return false;
		}
		return true;
	});
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
