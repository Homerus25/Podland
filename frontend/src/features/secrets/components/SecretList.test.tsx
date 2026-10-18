import { SecretList } from "@features/secrets/components/SecretList";
import type { SecretSummary } from "@features/secrets/services/secretsApi";
import { secretsQueryKey } from "@shared/queries/queryKeys";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import { machineFixture, secretFixture } from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

function mockSecrets(secrets: SecretSummary[]) {
	server.use(
		http.get("*/api/machines/m-1/secrets", () => HttpResponse.json(secrets)),
	);
}

function renderSecretList(permission: "read" | "write" | "admin" = "admin") {
	return renderWithQuery(
		<SecretList machine={machineFixture({ permission })} />,
		// The row navigates on click, so the list renders inside a router.
		{ router: true },
	);
}

describe("SecretList permission gating", () => {
	it("hides write controls from read users and explains the access level instead", async () => {
		mockSecrets([secretFixture()]);
		renderSecretList("read");

		expect(await screen.findByTestId("secret-row")).toBeInTheDocument();
		expect(
			screen.queryByTestId("create-secret-button"),
		).not.toBeInTheDocument();
		const hint = screen.getByTestId("secrets-permission-hint");
		expect(hint).toHaveAttribute("data-requirement", "write");
		expect(hint.textContent).toMatch(/read-only/i);
	});

	it.each(["write", "admin"] as const)(
		"shows the create affordance area to %s users",
		async (permission) => {
			mockSecrets([secretFixture()]);
			renderSecretList(permission);

			expect(
				await screen.findByTestId("create-secret-button"),
			).toBeInTheDocument();
			expect(
				screen.queryByTestId("secrets-permission-hint"),
			).not.toBeInTheDocument();
		},
	);
});

describe("SecretList", () => {
	it("renders one row per secret with name, driver, and created date", async () => {
		mockSecrets([
			secretFixture(),
			secretFixture({
				id: "9a8b7c6d",
				name: "api-token",
				driver: "file",
				createdAt: "2026-08-22T12:00:00Z",
				updatedAt: "2026-08-22T12:00:00Z",
			}),
		]);
		renderSecretList();

		await waitFor(() => {
			expect(screen.queryAllByTestId("secret-row")).toHaveLength(2);
		});
		expect(screen.getByText("db-password")).toBeInTheDocument();
		expect(screen.getByText("api-token")).toBeInTheDocument();
		expect(screen.getAllByText("file")).toHaveLength(2);
	});

	it("caches secrets under the per-machine query key hierarchy", async () => {
		mockSecrets([secretFixture()]);
		const { queryClient } = renderSecretList();

		await screen.findByText("db-password");
		const cached = queryClient.getQueryData<SecretSummary[]>(
			secretsQueryKey("m-1"),
		);
		expect(cached).toHaveLength(1);
		expect(cached?.[0].id).toBe("3f2e1d0c");
	});

	it("shows a loading state before secrets arrive", async () => {
		server.use(
			http.get("*/api/machines/m-1/secrets", async () => {
				await new Promise((resolve) => setTimeout(resolve, 50));
				return HttpResponse.json([secretFixture()]);
			}),
		);
		renderSecretList();

		expect(screen.getByTestId("secrets-loading")).toBeInTheDocument();
	});

	it("sorts rows when a column header is clicked", async () => {
		mockSecrets([
			secretFixture({ id: "z-1", name: "zebra-secret" }),
			secretFixture({ id: "a-1", name: "apple-secret" }),
		]);
		renderSecretList();
		await screen.findAllByTestId("secret-row");
		const user = userEvent.setup();
		await user.click(screen.getByTestId("sort-secret-0"));
		const rows = screen.getAllByTestId("secret-row");
		expect(within(rows[0]).getByText("apple-secret")).toBeInTheDocument();
		expect(within(rows[1]).getByText("zebra-secret")).toBeInTheDocument();
	});
});
