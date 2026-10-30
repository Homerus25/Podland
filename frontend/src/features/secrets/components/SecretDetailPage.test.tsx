import { SecretDetailPage } from "@features/secrets/components/SecretDetailPage";
import type { SecretInspect } from "@features/secrets/services/secretsApi";
import { secretQueryKey } from "@shared/queries/queryKeys";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { Route, Routes } from "react-router-dom";
import { describe, expect, it } from "vitest";
import { errorResponse } from "@/test/errors";
import {
	machineFixture,
	rawInspectFixture,
	secretFixture,
} from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

// The detail endpoint answers the metadata; the same URL with ?reveal=true
// (the admin reveal mutation) additionally carries the value.
function mockSecret(secret: SecretInspect) {
	server.use(
		http.get("*/api/machines/m-1/secrets/db-password", ({ request }) =>
			HttpResponse.json(
				request.url.includes("reveal=true")
					? { ...secret, secretData: "s3cr3t-value" }
					: secret,
			),
		),
	);
}

// The page reads the machine's per-caller facts for the reveal gate.
function mockMachine(permission: "read" | "write" | "admin" = "admin") {
	server.use(
		http.get("*/api/machines/m-1", () =>
			HttpResponse.json(
				machineFixture({ permission, isOwner: permission === "admin" }),
			),
		),
	);
}

function renderDetailPage() {
	return renderWithQuery(
		<Routes>
			<Route
				path="/machines/:machineId/secrets/:secretId"
				element={<SecretDetailPage />}
			/>
		</Routes>,
		{ router: { initialEntries: ["/machines/m-1/secrets/db-password"] } },
	);
}

describe("SecretDetailPage overview", () => {
	it("renders all overview attributes", async () => {
		mockMachine("admin");
		mockSecret(
			secretFixture({
				updatedAt: "2026-08-21T10:00:00Z",
				labels: { env: "prod" },
			}),
		);
		renderDetailPage();

		expect(await screen.findByTestId("secret-detail-id")).toHaveTextContent(
			"3f2e1d0c",
		);
		expect(screen.getByTestId("detail-page-title")).toHaveTextContent(
			"db-password",
		);
		expect(screen.getByTestId("secret-detail-name")).toHaveTextContent(
			"db-password",
		);
		expect(screen.getByTestId("secret-detail-driver")).toHaveTextContent(
			"file",
		);
		expect(
			screen.getByTestId("secret-detail-created"),
		).not.toBeEmptyDOMElement();
		expect(
			screen.getByTestId("secret-detail-updated"),
		).not.toBeEmptyDOMElement();
		expect(screen.getByTestId("secret-detail-labels")).toHaveTextContent(
			"env=prod",
		);
	});

	it("hides the labels row when the secret has no labels", async () => {
		mockMachine("admin");
		mockSecret(secretFixture());
		renderDetailPage();

		await screen.findByTestId("secret-detail-id");
		expect(
			screen.queryByTestId("secret-detail-labels"),
		).not.toBeInTheDocument();
	});

	it("links the back button to the machine's secrets tab", async () => {
		mockMachine("admin");
		mockSecret(secretFixture());
		renderDetailPage();

		await screen.findByTestId("secret-detail-id");
		expect(screen.getByTestId("detail-back-button")).toHaveAttribute(
			"href",
			"/machines/m-1?tab=secrets",
		);
	});

	it("renders the loading state while the detail is pending", async () => {
		mockMachine("admin");
		server.use(
			http.get("*/api/machines/m-1/secrets/db-password", async () => {
				await new Promise((resolve) => setTimeout(resolve, 50));
				return HttpResponse.json(secretFixture());
			}),
		);
		renderDetailPage();

		expect(screen.getByTestId("detail-page-loading")).toBeInTheDocument();
		await screen.findByTestId("secret-detail-id");
	});

	it("renders the error state when the detail request fails", async () => {
		mockMachine("admin");
		server.use(
			http.get("*/api/machines/m-1/secrets/db-password", () =>
				errorResponse(403, "ACCESS_DENIED", "Admin access required", "req-7"),
			),
		);
		renderDetailPage();

		const alert = await screen.findByTestId("detail-page-error");
		expect(alert).toHaveTextContent("ACCESS_DENIED");
		expect(alert).toHaveTextContent("Admin access required");
	});

	it("renders the not-found state when the secret no longer exists", async () => {
		mockMachine("admin");
		server.use(
			http.get("*/api/machines/m-1/secrets/db-password", () =>
				errorResponse(404, "SECRET_NOT_FOUND", "Secret not found", "req-9"),
			),
		);
		renderDetailPage();

		expect(
			await screen.findByTestId("detail-page-not-found"),
		).toHaveTextContent("This item no longer exists on the machine.");
		expect(screen.queryByTestId("detail-page-error")).not.toBeInTheDocument();
	});
});

describe("SecretDetailPage reveal value", () => {
	it("reveals the value masked until the eye toggle is used", async () => {
		mockMachine("admin");
		let requestedUrl = "";
		server.use(
			http.get("*/api/machines/m-1/secrets/db-password", ({ request }) => {
				requestedUrl = request.url;
				return HttpResponse.json(
					request.url.includes("reveal=true")
						? { ...secretFixture(), secretData: "s3cr3t-value" }
						: secretFixture(),
				);
			}),
		);
		const { queryClient } = renderDetailPage();
		const user = userEvent.setup();

		await user.click(await screen.findByTestId("reveal-secret-button"));
		const value = await screen.findByTestId("secret-value");

		expect(requestedUrl).toContain("reveal=true");
		expect(value).toHaveAttribute("data-visible", "false");
		expect(value).not.toHaveTextContent("s3cr3t-value");

		await user.click(screen.getByTestId("toggle-secret-visibility"));
		expect(value).toHaveAttribute("data-visible", "true");
		expect(value).toHaveTextContent("s3cr3t-value");

		await user.click(screen.getByTestId("toggle-secret-visibility"));
		expect(value).toHaveAttribute("data-visible", "false");
		expect(value).not.toHaveTextContent("s3cr3t-value");

		// The value travels through a mutation, so the detail query's cache
		// entry never carries it.
		const cached = queryClient.getQueryData<SecretInspect>(
			secretQueryKey("m-1", "db-password"),
		);
		expect(cached?.secretData).toBeUndefined();
	});

	it.each(["read", "write"] as const)(
		"shows a hint instead of a reveal button for %s users",
		async (permission) => {
			mockMachine(permission);
			mockSecret(secretFixture());
			renderDetailPage();

			expect(
				await screen.findByTestId("reveal-permission-hint"),
			).toBeInTheDocument();
			expect(
				screen.queryByTestId("reveal-secret-button"),
			).not.toBeInTheDocument();
		},
	);

	it("renders neither the reveal button nor the hint while the machine query is pending", async () => {
		// The machine query is the gate's only input: while it is in flight
		// the caller's level is unknown, so the page must not flash the
		// admin hint at an admin caller.
		mockMachine("admin");
		mockSecret(secretFixture());
		let release: () => void = () => {};
		const gate = new Promise<void>((resolve) => {
			release = resolve;
		});
		server.use(
			http.get("*/api/machines/m-1", async () => {
				await gate;
				return HttpResponse.json(
					machineFixture({ permission: "admin", isOwner: true }),
				);
			}),
		);
		renderDetailPage();

		await screen.findByTestId("secret-detail-id");
		expect(
			screen.queryByTestId("reveal-secret-button"),
		).not.toBeInTheDocument();
		expect(
			screen.queryByTestId("reveal-permission-hint"),
		).not.toBeInTheDocument();

		release();
		await screen.findByTestId("reveal-secret-button");
	});

	it("renders the admin hint when the machine query fails", async () => {
		// A failed machine query leaves the caller's level unknown — the
		// hint renders rather than a bare gap, so a non-admin never reads
		// the missing control as "this secret has no reveal".
		mockSecret(secretFixture());
		server.use(
			http.get("*/api/machines/m-1", () =>
				errorResponse(500, "MACHINE_UNREACHABLE", "Machine unreachable"),
			),
		);
		renderDetailPage();

		await screen.findByTestId("secret-detail-id");
		expect(
			await screen.findByTestId("reveal-permission-hint"),
		).toBeInTheDocument();
		expect(
			screen.queryByTestId("reveal-secret-button"),
		).not.toBeInTheDocument();
	});

	it("renders a 403 reveal error through the shared error display", async () => {
		mockMachine("admin");
		server.use(
			http.get("*/api/machines/m-1/secrets/db-password", ({ request }) => {
				if (request.url.includes("reveal=true")) {
					return errorResponse(
						403,
						"ACCESS_DENIED",
						"Admin access required",
						"req-7",
					);
				}
				return HttpResponse.json(secretFixture());
			}),
		);
		renderDetailPage();
		const user = userEvent.setup();

		await user.click(await screen.findByTestId("reveal-secret-button"));

		expect(await screen.findByRole("alert")).toBeInTheDocument();
		expect(screen.getByText("ACCESS_DENIED")).toBeInTheDocument();
		expect(screen.getByText("Admin access required")).toBeInTheDocument();
	});
});

describe("SecretDetailPage inspect JSON", () => {
	it("fetches the inspect JSON lazily on first reveal and pretty-prints it", async () => {
		mockMachine("admin");
		mockSecret(secretFixture());
		let inspectRequests = 0;
		server.use(
			http.get("*/api/machines/m-1/secrets/db-password/inspect", () => {
				inspectRequests++;
				return HttpResponse.json(rawInspectFixture());
			}),
		);
		renderDetailPage();
		const user = userEvent.setup();

		await screen.findByTestId("secret-detail-id");
		expect(inspectRequests).toBe(0);
		expect(screen.queryByTestId("inspect-json")).not.toBeInTheDocument();

		await user.click(screen.getByTestId("toggle-inspect-button"));

		expect((await screen.findByTestId("inspect-json")).textContent).toBe(
			JSON.stringify(rawInspectFixture(), null, 2),
		);
		expect(inspectRequests).toBe(1);
	});

	it("never includes the revealed value in the inspect JSON", async () => {
		mockMachine("admin");
		mockSecret(secretFixture());
		server.use(
			http.get("*/api/machines/m-1/secrets/db-password/inspect", () =>
				HttpResponse.json(rawInspectFixture()),
			),
		);
		renderDetailPage();
		const user = userEvent.setup();

		await user.click(await screen.findByTestId("reveal-secret-button"));
		await screen.findByTestId("secret-value");
		await user.click(screen.getByTestId("toggle-inspect-button"));

		const json = await screen.findByTestId("inspect-json");
		expect(json.textContent).not.toContain("s3cr3t-value");
		expect(json.textContent).not.toContain("secretData");
	});
});
