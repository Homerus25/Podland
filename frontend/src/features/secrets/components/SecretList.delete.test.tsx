import { SecretList } from "@features/secrets/components/SecretList";
import type { SecretSummary } from "@features/secrets/services/secretsApi";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http, type JsonBodyType } from "msw";
import { beforeEach, describe, expect, it } from "vitest";
import { errorJson } from "@/test/errors";
import { hostFixture, secretFixture } from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

const dbPassword = secretFixture();

let deleteRequests: { count: number; method?: string; url: URL | null } = {
	count: 0,
	method: undefined,
	url: null,
};

function secretsHandler(secrets: () => SecretSummary[]) {
	return http.get("*/api/hosts/m-1/secrets", () =>
		HttpResponse.json(secrets()),
	);
}

function deleteHandler(
	status = 204,
	body?: JsonBodyType,
	onDeleted?: () => void,
) {
	return http.delete("*/api/hosts/m-1/secrets/:secretId", ({ request }) => {
		deleteRequests.count++;
		deleteRequests.method = request.method;
		deleteRequests.url = new URL(request.url);
		if (status !== 204) return HttpResponse.json(body, { status });
		onDeleted?.();
		return new HttpResponse(null, { status: 204 });
	});
}

function renderSecretList(permission: "read" | "write" | "admin" = "admin") {
	// The row navigates on click, so the list renders inside a router.
	renderWithQuery(<SecretList host={hostFixture({ permission })} />, {
		router: true,
	});
}

async function openDeleteModal(name = "db-password") {
	renderSecretList();
	const user = userEvent.setup();
	await user.click(
		await screen.findByRole("button", { name: `Delete ${name}` }),
	);
	expect(await screen.findByTestId("modal")).toBeInTheDocument();
	return user;
}

describe("SecretList delete", () => {
	beforeEach(() => {
		deleteRequests = { count: 0, method: undefined, url: null };
	});

	it("opens a confirmation modal naming the secret without sending a request", async () => {
		server.use(
			secretsHandler(() => [dbPassword]),
			deleteHandler(),
		);
		await openDeleteModal();

		expect(screen.getByTestId("modal")).toHaveTextContent("db-password");
		expect(deleteRequests.count).toBe(0);
	});

	it("sends DELETE to the secret endpoint addressed by name on confirm", async () => {
		server.use(
			secretsHandler(() => [dbPassword]),
			deleteHandler(),
		);
		const user = await openDeleteModal();

		await user.click(screen.getByTestId("confirm-delete-button"));

		await waitFor(() => expect(deleteRequests.count).toBe(1));
		expect(deleteRequests.method).toBe("DELETE");
		expect(deleteRequests.url?.pathname).toBe(
			"/api/hosts/m-1/secrets/db-password",
		);
	});

	it("closes the modal and refetches the list so the deleted secret disappears", async () => {
		const survivor = secretFixture({ id: "9a8b7c6d", name: "api-token" });
		let secrets = [dbPassword, survivor];
		server.use(
			secretsHandler(() => secrets),
			deleteHandler(204, undefined, () => {
				secrets = [survivor];
			}),
		);
		const user = await openDeleteModal();

		await user.click(screen.getByTestId("confirm-delete-button"));

		await waitFor(() =>
			expect(screen.queryByTestId("modal")).not.toBeInTheDocument(),
		);
		expect(await screen.findByText("api-token")).toBeInTheDocument();
		await waitFor(() =>
			expect(screen.queryByText("db-password")).not.toBeInTheDocument(),
		);
		expect(deleteRequests.count).toBe(1);
	});

	it("keeps the modal open and shows the SECRET_IN_USE envelope when deletion is rejected", async () => {
		server.use(
			secretsHandler(() => [dbPassword]),
			deleteHandler(
				409,
				errorJson(
					"SECRET_IN_USE",
					"secret db-password is in use by container web",
					"req-61",
				),
			),
		);
		const user = await openDeleteModal();

		await user.click(screen.getByTestId("confirm-delete-button"));

		expect(await screen.findByRole("alert")).toBeInTheDocument();
		expect(screen.getByText("SECRET_IN_USE")).toBeInTheDocument();
		expect(
			screen.getByText("secret db-password is in use by container web"),
		).toBeInTheDocument();
		expect(screen.getByTestId("modal")).toBeInTheDocument();
		expect(screen.getByTestId("confirm-delete-button")).toBeInTheDocument();
	});

	it("shows the SECRET_NOT_FOUND envelope when the secret vanished before deletion", async () => {
		server.use(
			secretsHandler(() => [dbPassword]),
			deleteHandler(
				404,
				errorJson("SECRET_NOT_FOUND", "Secret not found", "req-62"),
			),
		);
		const user = await openDeleteModal();

		await user.click(screen.getByTestId("confirm-delete-button"));

		expect(await screen.findByRole("alert")).toBeInTheDocument();
		expect(screen.getByText("SECRET_NOT_FOUND")).toBeInTheDocument();
		expect(screen.getByText("Secret not found")).toBeInTheDocument();
		expect(screen.getByTestId("modal")).toBeInTheDocument();
		expect(screen.getByTestId("confirm-delete-button")).toBeInTheDocument();
	});

	it("hides the delete action from read users", async () => {
		server.use(
			secretsHandler(() => [dbPassword]),
			deleteHandler(),
		);
		renderSecretList("read");

		expect(await screen.findByTestId("secret-row")).toBeInTheDocument();
		expect(
			screen.queryByRole("button", { name: /delete/i }),
		).not.toBeInTheDocument();
	});
});
