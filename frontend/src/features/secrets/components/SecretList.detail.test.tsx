import { SecretList } from "@features/secrets/components/SecretList";
import type {
	SecretInspect,
	SecretSummary,
} from "@features/secrets/services/secretsApi";
import { secretQueryKey } from "@shared/queries/queryKeys";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import { secretFixture } from "@/test/factories";
import { server } from "@/test/server";

function mockSecrets(secrets: SecretSummary[]) {
	server.use(
		http.get("*/api/machines/m-1/secrets", () => HttpResponse.json(secrets)),
	);
}

function mockInspect(secret: SecretInspect) {
	return server.use(
		http.get("*/api/machines/m-1/secrets/db-password", ({ request }) =>
			HttpResponse.json(
				request.url.includes("reveal=true")
					? { ...secret, secretData: "s3cr3t-value" }
					: secret,
			),
		),
	);
}

async function openDetail() {
	await screen.findByTestId("secret-row");
	await userEvent.click(screen.getByTestId("secret-detail-button"));
	await screen.findByTestId("secret-detail-id");
}

function renderSecretList(permission: "read" | "write" | "admin" = "admin") {
	const queryClient = new QueryClient({
		defaultOptions: { queries: { retry: false } },
	});
	render(
		<QueryClientProvider client={queryClient}>
			<SecretList machineId="m-1" permission={permission} />
		</QueryClientProvider>,
	);
	return { queryClient };
}

describe("SecretDetailModal", () => {
	it("renders detail fields from the inspect endpoint", async () => {
		mockSecrets([secretFixture({ updatedAt: "2026-08-21T10:00:00Z" })]);
		mockInspect(
			secretFixture({
				updatedAt: "2026-08-21T10:00:00Z",
				labels: { env: "prod" },
			}),
		);
		renderSecretList();

		await openDetail();

		expect(screen.getByTestId("secret-detail-id")).toHaveTextContent(
			"3f2e1d0c",
		);
		expect(screen.getByTestId("secret-detail-name")).toHaveTextContent(
			"db-password",
		);
		expect(screen.getByTestId("secret-detail-driver")).toHaveTextContent(
			"file",
		);
		expect(screen.getByTestId("secret-detail-labels")).toHaveTextContent(
			"env=prod",
		);
		expect(
			screen.getByTestId("secret-detail-created"),
		).not.toBeEmptyDOMElement();
		expect(
			screen.getByTestId("secret-detail-updated"),
		).not.toBeEmptyDOMElement();
	});

	it("sends reveal=true to the inspect endpoint when an admin reveals", async () => {
		mockSecrets([secretFixture({ updatedAt: "2026-08-21T10:00:00Z" })]);
		let requestedUrl = "";
		server.use(
			http.get("*/api/machines/m-1/secrets/db-password", ({ request }) => {
				requestedUrl = request.url;
				return HttpResponse.json({
					...secretFixture({ updatedAt: "2026-08-21T10:00:00Z" }),
					secretData: "s3cr3t-value",
				});
			}),
		);
		renderSecretList();

		await openDetail();
		await userEvent.click(screen.getByTestId("reveal-secret-button"));

		await screen.findByTestId("secret-value");
		expect(requestedUrl).toContain("reveal=true");
	});

	it.each(["read", "write"] as const)(
		"shows a hint instead of a reveal button for %s users",
		async (permission) => {
			mockSecrets([secretFixture({ updatedAt: "2026-08-21T10:00:00Z" })]);
			mockInspect(secretFixture({ updatedAt: "2026-08-21T10:00:00Z" }));
			renderSecretList(permission);

			await openDetail();

			expect(
				screen.queryByTestId("reveal-secret-button"),
			).not.toBeInTheDocument();
			expect(screen.getByTestId("reveal-permission-hint")).toBeInTheDocument();
		},
	);

	it("renders the revealed value masked until the eye toggle is used", async () => {
		mockSecrets([secretFixture({ updatedAt: "2026-08-21T10:00:00Z" })]);
		mockInspect(secretFixture({ updatedAt: "2026-08-21T10:00:00Z" }));
		renderSecretList();

		await openDetail();
		await userEvent.click(screen.getByTestId("reveal-secret-button"));
		const value = await screen.findByTestId("secret-value");

		expect(value).toHaveAttribute("data-visible", "false");
		expect(value).not.toHaveTextContent("s3cr3t-value");

		await userEvent.click(screen.getByTestId("toggle-secret-visibility"));
		expect(value).toHaveAttribute("data-visible", "true");
		expect(value).toHaveTextContent("s3cr3t-value");
	});

	it("renders a 403 reveal error through the shared error display", async () => {
		mockSecrets([secretFixture({ updatedAt: "2026-08-21T10:00:00Z" })]);
		server.use(
			http.get("*/api/machines/m-1/secrets/db-password", ({ request }) => {
				if (request.url.includes("reveal=true")) {
					return HttpResponse.json(
						{
							requestId: "req-7",
							error: {
								message: "Admin access required",
								code: "ACCESS_DENIED",
							},
						},
						{ status: 403 },
					);
				}
				return HttpResponse.json(
					secretFixture({ updatedAt: "2026-08-21T10:00:00Z" }),
				);
			}),
		);
		renderSecretList();

		await openDetail();
		await userEvent.click(screen.getByTestId("reveal-secret-button"));

		expect(await screen.findByRole("alert")).toBeInTheDocument();
		expect(screen.getByText("ACCESS_DENIED")).toBeInTheDocument();
		expect(screen.getByText("Admin access required")).toBeInTheDocument();
	});

	it("renders an inspect 404 through the shared error display", async () => {
		mockSecrets([secretFixture({ updatedAt: "2026-08-21T10:00:00Z" })]);
		server.use(
			http.get("*/api/machines/m-1/secrets/db-password", () =>
				HttpResponse.json(
					{
						requestId: "req-9",
						error: { message: "Secret not found", code: "SECRET_NOT_FOUND" },
					},
					{ status: 404 },
				),
			),
		);
		renderSecretList();

		await screen.findByTestId("secret-row");
		await userEvent.click(screen.getByTestId("secret-detail-button"));

		expect(await screen.findByRole("alert")).toBeInTheDocument();
		expect(screen.getByText("SECRET_NOT_FOUND")).toBeInTheDocument();
		expect(screen.getByText("Secret not found")).toBeInTheDocument();
	});

	it("renders a connection failure through the shared error display", async () => {
		mockSecrets([secretFixture({ updatedAt: "2026-08-21T10:00:00Z" })]);
		server.use(
			http.get("*/api/machines/m-1/secrets/db-password", () =>
				HttpResponse.error(),
			),
		);
		renderSecretList();

		await screen.findByTestId("secret-row");
		await userEvent.click(screen.getByTestId("secret-detail-button"));

		expect(await screen.findByRole("alert")).toBeInTheDocument();
	});

	it("keeps no inspect data in the React Query cache after the view closes", async () => {
		mockSecrets([secretFixture({ updatedAt: "2026-08-21T10:00:00Z" })]);
		mockInspect(secretFixture({ updatedAt: "2026-08-21T10:00:00Z" }));
		const { queryClient } = renderSecretList();

		await openDetail();
		expect(
			queryClient.getQueryData(secretQueryKey("m-1", "db-password")),
		).toBeTruthy();

		await userEvent.click(screen.getByRole("button", { name: /close/i }));
		await waitFor(() => {
			expect(
				queryClient.getQueryData(secretQueryKey("m-1", "db-password")),
			).toBeUndefined();
		});
	});
});
