import { SecretList } from "@features/secrets/components/SecretList";
import type { SecretSummary } from "@features/secrets/services/secretsApi";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { Route, Routes, useParams } from "react-router-dom";
import { describe, expect, it } from "vitest";
import {
	hostFixture,
	secretFixture,
	secretMovePreviewFixture,
} from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

function mockSecrets(secrets: SecretSummary[]) {
	server.use(
		http.get("*/api/hosts/m-1/secrets", () => HttpResponse.json(secrets)),
	);
}

// The detail route's stub: the row click must land here, with the secret's
// name as the route param (the backend addresses secrets by name).
function SecretDetailStub() {
	const { secretId } = useParams<{ secretId: string }>();
	return <div data-testid="secret-detail-page">{secretId}</div>;
}

function renderSecretList(permission: "read" | "write" | "admin" = "admin") {
	return renderWithQuery(
		<Routes>
			<Route
				path="/hosts/:hostId"
				element={
					<SecretList
						host={hostFixture({
							permission,
							isOwner: permission === "admin",
						})}
					/>
				}
			/>
			<Route
				path="/hosts/:hostId/secrets/:secretId"
				element={<SecretDetailStub />}
			/>
		</Routes>,
		{ router: { initialEntries: ["/hosts/m-1"] } },
	);
}

describe("SecretList row navigation", () => {
	it("navigates to the secret's detail page when the row is clicked", async () => {
		mockSecrets([secretFixture()]);
		renderSecretList();
		const user = userEvent.setup();

		await user.click(await screen.findByTestId("secret-row"));

		expect(await screen.findByTestId("secret-detail-page")).toHaveTextContent(
			"db-password",
		);
		expect(screen.queryByTestId("secret-row")).not.toBeInTheDocument();
		// The former detail modal is gone: no dialog opens on row click.
		expect(screen.queryByTestId("modal")).not.toBeInTheDocument();
		expect(
			screen.queryByTestId("secret-detail-button"),
		).not.toBeInTheDocument();
	});

	it("navigates for read-only users too", async () => {
		mockSecrets([secretFixture()]);
		renderSecretList("read");
		const user = userEvent.setup();

		await user.click(await screen.findByTestId("secret-row"));

		expect(await screen.findByTestId("secret-detail-page")).toHaveTextContent(
			"db-password",
		);
	});

	it("does not navigate when the delete button is clicked", async () => {
		mockSecrets([secretFixture()]);
		renderSecretList();
		const user = userEvent.setup();

		await user.click(
			await screen.findByRole("button", { name: "Delete db-password" }),
		);

		expect(
			await screen.findByTestId("confirm-delete-button"),
		).toBeInTheDocument();
		expect(screen.queryByTestId("secret-detail-page")).not.toBeInTheDocument();
		expect(screen.getByTestId("secret-row")).toBeInTheDocument();
	});

	it("does not navigate when the move/copy button is clicked", async () => {
		mockSecrets([secretFixture()]);
		server.use(
			http.get("*/api/hosts", () =>
				HttpResponse.json([hostFixture({ id: "m-2", name: "staging-01" })]),
			),
			http.get("*/api/hosts/m-1/secrets/db-password/move-preview", () =>
				HttpResponse.json(secretMovePreviewFixture()),
			),
		);
		renderSecretList();
		const user = userEvent.setup();

		await user.click(
			await screen.findByRole("button", { name: "Move or copy db-password" }),
		);

		expect(await screen.findByTestId("modal")).toBeInTheDocument();
		expect(screen.queryByTestId("secret-detail-page")).not.toBeInTheDocument();
		expect(screen.getByTestId("secret-row")).toBeInTheDocument();
	});
});
