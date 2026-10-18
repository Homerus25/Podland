import { SecretList } from "@features/secrets/components/SecretList";
import type { SecretSummary } from "@features/secrets/services/secretsApi";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import { errorResponse } from "@/test/errors";
import {
	machineFixture,
	rawInspectFixture,
	secretFixture,
	secretMovePreviewFixture,
} from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

function mockSecrets(secrets: SecretSummary[]) {
	server.use(
		http.get("*/api/machines/m-1/secrets", () => HttpResponse.json(secrets)),
	);
}

function inspectHandler() {
	return http.get("*/api/machines/m-1/secrets/db-password/inspect", () =>
		HttpResponse.json(rawInspectFixture()),
	);
}

function renderSecretList() {
	return renderWithQuery(<SecretList machine={machineFixture()} />);
}

describe("SecretList inspect", () => {
	it("opens the inspect dialog titled with the secret's name and shows its JSON", async () => {
		const requestedPaths: string[] = [];
		mockSecrets([secretFixture()]);
		server.use(
			http.get(
				"*/api/machines/m-1/secrets/db-password/inspect",
				({ request }) => {
					requestedPaths.push(new URL(request.url).pathname);
					return HttpResponse.json(rawInspectFixture());
				},
			),
		);
		renderSecretList();
		const user = userEvent.setup();

		await user.click(await screen.findByTestId("secret-row"));

		const modal = await screen.findByTestId("modal");
		expect(within(modal).getByText("db-password")).toBeInTheDocument();
		expect(screen.getByTestId("inspect-dialog-json").textContent).toBe(
			JSON.stringify(rawInspectFixture(), null, 2),
		);
		expect(requestedPaths).toEqual([
			"/api/machines/m-1/secrets/db-password/inspect",
		]);
	});

	it("closing the dialog returns to the list", async () => {
		mockSecrets([secretFixture()]);
		server.use(inspectHandler());
		renderSecretList();
		const user = userEvent.setup();

		await user.click(await screen.findByTestId("secret-row"));
		await screen.findByTestId("inspect-dialog-json");
		await user.click(screen.getByRole("button", { name: "Close dialog" }));

		await waitFor(() =>
			expect(screen.queryByTestId("modal")).not.toBeInTheDocument(),
		);
		expect(screen.getByTestId("secret-row")).toBeInTheDocument();
	});

	it("opens the detail modal from the name link without opening the JSON window", async () => {
		let inspectRequests = 0;
		mockSecrets([secretFixture()]);
		server.use(
			http.get("*/api/machines/m-1/secrets/db-password", () =>
				HttpResponse.json(secretFixture()),
			),
			http.get("*/api/machines/m-1/secrets/db-password/inspect", () => {
				inspectRequests++;
				return HttpResponse.json(rawInspectFixture());
			}),
		);
		renderSecretList();
		const user = userEvent.setup();

		await user.click(await screen.findByTestId("secret-detail-button"));

		expect(await screen.findByTestId("secret-detail-id")).toBeInTheDocument();
		expect(inspectRequests).toBe(0);
		expect(
			screen.queryByTestId("inspect-dialog-loading"),
		).not.toBeInTheDocument();
		expect(screen.queryByTestId("inspect-dialog-json")).not.toBeInTheDocument();
		expect(
			screen.queryByTestId("inspect-dialog-error"),
		).not.toBeInTheDocument();
	});

	it("does not open the inspect dialog from the row's action buttons", async () => {
		let inspectRequests = 0;
		mockSecrets([secretFixture()]);
		server.use(
			http.get("*/api/machines", () =>
				HttpResponse.json([machineFixture({ id: "m-2", name: "staging-01" })]),
			),
			http.get("*/api/machines/m-1/secrets/db-password/move-preview", () =>
				HttpResponse.json(secretMovePreviewFixture()),
			),
			http.get("*/api/machines/m-1/secrets/db-password/inspect", () => {
				inspectRequests++;
				return HttpResponse.json(rawInspectFixture());
			}),
		);
		renderSecretList();
		const user = userEvent.setup();
		await screen.findByTestId("secret-row");
		// Re-queried per click, mirroring the containers inspect suite.
		const rowAt = () => within(screen.getByTestId("secret-row"));

		// move/copy — opens the transfer dialog, not the inspect one
		await user.click(rowAt().getByTestId("move-secret-button"));
		expect(await screen.findByTestId("modal")).toBeInTheDocument();
		await user.click(screen.getByRole("button", { name: "Close dialog" }));
		await waitFor(() =>
			expect(screen.queryByTestId("modal")).not.toBeInTheDocument(),
		);

		// delete — opens the confirm dialog, not the inspect one
		await user.click(rowAt().getByTestId("delete-secret-button"));
		expect(
			await screen.findByTestId("confirm-delete-button"),
		).toBeInTheDocument();

		expect(inspectRequests).toBe(0);
		expect(
			screen.queryByTestId("inspect-dialog-loading"),
		).not.toBeInTheDocument();
		expect(screen.queryByTestId("inspect-dialog-json")).not.toBeInTheDocument();
		expect(
			screen.queryByTestId("inspect-dialog-error"),
		).not.toBeInTheDocument();
	});

	it("renders the error region inside the dialog when the endpoint 404s", async () => {
		mockSecrets([secretFixture()]);
		server.use(
			http.get("*/api/machines/m-1/secrets/db-password/inspect", () =>
				errorResponse(404, "SECRET_NOT_FOUND", "Secret not found"),
			),
		);
		renderSecretList();
		const user = userEvent.setup();

		await user.click(await screen.findByTestId("secret-row"));

		const alert = await screen.findByTestId("inspect-dialog-error");
		expect(alert).toHaveTextContent("SECRET_NOT_FOUND");
		expect(alert).toHaveTextContent("Secret not found");
		expect(screen.queryByTestId("inspect-dialog-json")).not.toBeInTheDocument();
	});
});
