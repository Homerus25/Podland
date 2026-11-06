import { SecretDetailPage } from "@features/secrets/components/SecretDetailPage";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { Route, Routes, useLocation, useParams } from "react-router-dom";
import { beforeEach, describe, expect, it } from "vitest";
import { errorResponse } from "@/test/errors";
import { hostFixture, secretFixture } from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

// The delete request the page sends: the secret is addressed by name, and
// the endpoint has no force parameter at all.
interface DeleteRequests {
	count: number;
	method?: string;
	pathname: string;
	search: string;
}

let deleteRequests: DeleteRequests;

function resetDeleteRequests() {
	deleteRequests = { count: 0, method: undefined, pathname: "", search: "" };
}

// The host tab the page navigates to on success: the real host page reads
// the ?tab= search parameter, so the probe observes it the same way.
function HostTabProbe() {
	const { hostId } = useParams<{ hostId: string }>();
	const { search } = useLocation();
	return (
		<div>
			<span data-testid="probe-host">{hostId}</span>
			<span data-testid="probe-search">{search}</span>
		</div>
	);
}

function installHandlers(
	options: {
		permission?: "read" | "write" | "admin";
		deleteResponder?: () => Response;
	} = {},
) {
	const permission = options.permission ?? "write";
	server.use(
		http.get("*/api/hosts/m-1", () =>
			HttpResponse.json(
				hostFixture({ permission, isOwner: permission === "admin" }),
			),
		),
		http.get("*/api/hosts/m-1/secrets/db-password", () =>
			HttpResponse.json(secretFixture()),
		),
		http.delete("*/api/hosts/m-1/secrets/db-password", ({ request }) => {
			deleteRequests.count += 1;
			deleteRequests.method = request.method;
			const url = new URL(request.url);
			deleteRequests.pathname = url.pathname;
			deleteRequests.search = url.search;
			if (options.deleteResponder) return options.deleteResponder();
			return new HttpResponse(null, { status: 204 });
		}),
	);
}

function renderDetailPage() {
	return renderWithQuery(
		<Routes>
			<Route
				path="/hosts/:hostId/secrets/:secretId"
				element={<SecretDetailPage />}
			/>
			<Route path="/hosts/:hostId" element={<HostTabProbe />} />
		</Routes>,
		{ router: { initialEntries: ["/hosts/m-1/secrets/db-password"] } },
	);
}

describe("SecretDetailPage remove", () => {
	beforeEach(resetDeleteRequests);

	it("shows the Remove button to a caller who can manage secrets", async () => {
		installHandlers({ permission: "write" });
		renderDetailPage();

		expect(await screen.findByTestId("remove-secret-button")).toHaveTextContent(
			"Remove",
		);
	});

	it("renders the write-permission hint instead of Remove for a read-only caller", async () => {
		installHandlers({ permission: "read" });
		renderDetailPage();

		const hint = await screen.findByTestId("secrets-permission-hint");
		expect(hint).toHaveAttribute("data-requirement", "write");
		expect(
			screen.queryByTestId("remove-secret-button"),
		).not.toBeInTheDocument();
	});

	it("renders neither the Remove button nor the hint while the host query is pending", async () => {
		// The host query is the gate's only input: while it is in flight the
		// caller's level is unknown, so the header must not flash the
		// read-only hint at a write-capable caller.
		installHandlers();
		let release: () => void = () => {};
		const gate = new Promise<void>((resolve) => {
			release = resolve;
		});
		server.use(
			http.get("*/api/hosts/m-1", async () => {
				await gate;
				return HttpResponse.json(hostFixture({ permission: "write" }));
			}),
		);
		renderDetailPage();

		await screen.findByTestId("secret-detail-name");
		expect(
			screen.queryByTestId("remove-secret-button"),
		).not.toBeInTheDocument();
		expect(
			screen.queryByTestId("secrets-permission-hint"),
		).not.toBeInTheDocument();

		release();
		await screen.findByTestId("remove-secret-button");
	});

	it("hides the Remove button when the secret no longer exists", async () => {
		// The header actions render outside the layout's state ternary, so
		// the page hides the action itself on a 404 overview.
		installHandlers();
		server.use(
			http.get("*/api/hosts/m-1/secrets/db-password", () =>
				errorResponse(404, "SECRET_NOT_FOUND", "Secret not found", "req-73"),
			),
		);
		renderDetailPage();

		await screen.findByTestId("detail-page-not-found");
		expect(
			screen.queryByTestId("remove-secret-button"),
		).not.toBeInTheDocument();
		// Move/Copy self-gates on the overview DTO, so a 404 page renders no
		// control that would open nothing.
		expect(screen.queryByTestId("move-secret-button")).not.toBeInTheDocument();
	});

	it("opens a confirmation naming the secret without sending a request", async () => {
		installHandlers();
		renderDetailPage();
		const user = userEvent.setup();

		await user.click(await screen.findByTestId("remove-secret-button"));

		const modal = await screen.findByTestId("modal");
		expect(modal).toHaveTextContent("db-password");
		expect(deleteRequests.count).toBe(0);
	});

	it("sends DELETE to the secret endpoint addressed by name on confirm", async () => {
		installHandlers();
		renderDetailPage();
		const user = userEvent.setup();

		await user.click(await screen.findByTestId("remove-secret-button"));
		await user.click(screen.getByTestId("confirm-delete-button"));

		await waitFor(() => expect(deleteRequests.count).toBe(1));
		expect(deleteRequests.method).toBe("DELETE");
		expect(deleteRequests.pathname).toBe("/api/hosts/m-1/secrets/db-password");
		expect(deleteRequests.search).toBe("");
	});

	it("explains an in-use refusal with no force path", async () => {
		installHandlers({
			deleteResponder: () =>
				errorResponse(
					409,
					"SECRET_IN_USE",
					"secret db-password is in use by container web",
					"req-74",
				),
		});
		renderDetailPage();
		const user = userEvent.setup();

		await user.click(await screen.findByTestId("remove-secret-button"));
		await user.click(screen.getByTestId("confirm-delete-button"));

		expect(
			await screen.findByTestId("secret-in-use-warning"),
		).toBeInTheDocument();
		// The envelope still renders beside the hint, and the dialog keeps
		// its plain confirm: the endpoint has no force parameter.
		expect(screen.getByRole("alert")).toHaveTextContent("SECRET_IN_USE");
		expect(screen.getByTestId("confirm-delete-button")).toBeInTheDocument();
		expect(screen.queryByTestId("force-remove-button")).not.toBeInTheDocument();
	});

	it("navigates to the secrets tab on success", async () => {
		installHandlers();
		renderDetailPage();
		const user = userEvent.setup();

		await user.click(await screen.findByTestId("remove-secret-button"));
		await user.click(screen.getByTestId("confirm-delete-button"));

		expect(await screen.findByTestId("probe-host")).toHaveTextContent("m-1");
		expect(screen.getByTestId("probe-search")).toHaveTextContent(
			"?tab=secrets",
		);
	});
});
