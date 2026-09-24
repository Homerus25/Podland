import { HttpResponse, http } from "msw";
import { errorResponse } from "@/test/errors";
import { authStatusFixture } from "@/test/factories";

// Handlers mirror the real API shapes from the backend
// (see backend/internal/app/app.go and backend/internal/domain/auth/handler.go).
// The login body carries the username alongside the token (login echoes
// the credential; refresh would return the session's user).
export const loginSuccessHandler = http.post(
	"*/api/auth/login",
	async ({ request }) => {
		const { username } = (await request.json()) as { username: string };
		return HttpResponse.json({
			token: "test-access-token",
			username,
			role: "user",
		});
	},
);

// Default refresh rejection so full-app renders start logged-out, the
// pre-session browser state; session-restore tests override with
// server.use().
export const refreshRejectedHandler = http.post("*/api/auth/refresh", () =>
	errorResponse(401, "AUTH_REFRESH_INVALID", "Refresh token invalid"),
);

// Default auth status probe: a configured instance, so logged-out boots
// land on the login form as before the first-run feature; first-run
// tests override this with server.use() and needsSetup: true.
export const authStatusConfiguredHandler = http.get("*/api/auth/status", () => {
	return HttpResponse.json(authStatusFixture());
});

export const handlers = [
	http.get("*/api/health", () => {
		return new HttpResponse("OK", { status: 200 });
	}),
	loginSuccessHandler,
	refreshRejectedHandler,
	authStatusConfiguredHandler,
	http.post("*/api/auth/logout", () => {
		return HttpResponse.json({ message: "Logged out successfully" });
	}),
	// Default status probe so card-rendering tests see a healthy machine;
	// individual tests override this with server.use().
	http.get("*/api/machines/:id/status", () => {
		return HttpResponse.json({
			status: "online",
			checkedAt: "2026-08-29T10:00:00Z",
		});
	}),
];
