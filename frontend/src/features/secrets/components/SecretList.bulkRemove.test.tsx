import { SecretList } from "@features/secrets/components/SecretList";
import type { SecretSummary } from "@features/secrets/services/secretsApi";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http, type JsonBodyType } from "msw";
import { useLocation } from "react-router-dom";
import { beforeEach, describe, expect, it } from "vitest";
import { errorResponse } from "@/test/errors";
import { hostFixture, secretFixture } from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

// The bulk-remove flow (spec 461, ticket #469) at the MSW/HTTP seam: the
// request shape, the per-entry report, the list refetch, and the selection
// clearing. Secrets are the one resource with no force path at all — the
// backend's request DTO has no force field and the report offers no retry —
// so this suite pins the absence of both. The dialog skeleton itself is
// pinned once in the shared ReportDialog suite.

const victimA = secretFixture({ id: "s-1", name: "victim-a" });
const victimB = secretFixture({ id: "s-2", name: "victim-b" });

type RemoveBody = { ids: string[] };

let listRequests: { count: number; secrets: () => SecretSummary[] } = {
	count: 0,
	secrets: () => [],
};
let removeRequests: {
	count: number;
	pathname: string;
	bodies: RemoveBody[];
} = { count: 0, pathname: "", bodies: [] };

function resetCounters() {
	listRequests = { count: 0, secrets: () => [victimA, victimB] };
	removeRequests = { count: 0, pathname: "", bodies: [] };
}

function installHandlers(
	removeResponse: (body: RemoveBody) => JsonBodyType = (body) =>
		body.ids.map((id) => ({ id, success: true })),
	removeOnSuccess = false,
) {
	server.use(
		http.get("*/api/hosts/m-1/secrets", () => {
			listRequests.count += 1;
			return HttpResponse.json(listRequests.secrets());
		}),
		http.post("*/api/hosts/m-1/secrets/remove", async ({ request }) => {
			removeRequests.count += 1;
			removeRequests.pathname = new URL(request.url).pathname;
			const body = (await request.json()) as RemoveBody;
			removeRequests.bodies.push(body);
			if (removeOnSuccess) {
				const removed = new Set(body.ids);
				const before = listRequests.secrets;
				listRequests.secrets = () =>
					before().filter((secret) => !removed.has(secret.name));
			}
			return HttpResponse.json(removeResponse(body));
		}),
	);
}

function LocationProbe() {
	const location = useLocation();
	return <div data-testid="location">{location.pathname}</div>;
}

function renderSecretList(permission: "read" | "write" | "admin" = "admin") {
	renderWithQuery(
		<>
			<LocationProbe />
			<SecretList
				host={hostFixture({ permission, isOwner: permission === "admin" })}
			/>
		</>,
		{ chakra: true, router: { initialEntries: ["/hosts/m-1"] } },
	);
}

// The row checkbox's accessible name is the row's stable key (the shared
// selection contract derives it from `getKey`), which for a secret is its
// name — the identity the removal endpoint addresses, matching the detail
// page's single-item Remove.
async function openBulkRemove(...names: string[]) {
	renderSecretList();
	await screen.findByText("victim-a");
	const user = userEvent.setup();
	for (const name of names) {
		await user.click(screen.getByRole("checkbox", { name: `Select ${name}` }));
	}
	await user.click(screen.getByTestId("remove-selected-secrets-button"));
	expect(await screen.findByTestId("modal")).toBeInTheDocument();
	return user;
}

describe("SecretList bulk remove", () => {
	beforeEach(resetCounters);

	it("offers Remove selected only while something is selected", async () => {
		installHandlers();
		renderSecretList();
		await screen.findByText("victim-a");
		const user = userEvent.setup();

		expect(
			screen.queryByTestId("remove-selected-secrets-button"),
		).not.toBeInTheDocument();

		const checkbox = screen.getByRole("checkbox", {
			name: `Select ${victimA.name}`,
		});
		await user.click(checkbox);
		expect(
			screen.getByTestId("remove-selected-secrets-button"),
		).toBeInTheDocument();

		await user.click(checkbox);
		expect(
			screen.queryByTestId("remove-selected-secrets-button"),
		).not.toBeInTheDocument();
	});

	it("hides the selection from read-level users", async () => {
		installHandlers();
		renderSecretList("read");
		await screen.findByText("victim-a");

		expect(screen.queryAllByRole("checkbox")).toHaveLength(0);
		expect(
			screen.queryByTestId("remove-selected-secrets-button"),
		).not.toBeInTheDocument();
	});

	it("selects a row without navigating to its detail page", async () => {
		installHandlers();
		renderSecretList();
		await screen.findByText("victim-a");
		const user = userEvent.setup();

		await user.click(
			screen.getByRole("checkbox", { name: `Select ${victimA.name}` }),
		);

		expect(screen.getByTestId("location").textContent).toBe("/hosts/m-1");
	});

	it("confirms with the count and the selected names, sending nothing yet", async () => {
		installHandlers();
		await openBulkRemove(victimA.name, victimB.name);

		expect(screen.getByText(/remove 2 secrets/i)).toBeInTheDocument();
		const names = screen.getByTestId("bulk-remove-names");
		expect(within(names).getByText("victim-a")).toBeInTheDocument();
		expect(within(names).getByText("victim-b")).toBeInTheDocument();
		expect(removeRequests.count).toBe(0);
	});

	it("sends one ids-only bulk request and refetches the list", async () => {
		installHandlers(undefined, true);
		const user = await openBulkRemove(victimA.name, victimB.name);
		const listCountBefore = listRequests.count;

		await user.click(screen.getByTestId("confirm-bulk-remove-button"));

		await waitFor(() => expect(removeRequests.count).toBe(1));
		expect(removeRequests.pathname).toBe("/api/hosts/m-1/secrets/remove");
		// Secrets have no force path at all: the body carries ids only.
		expect(removeRequests.bodies[0]).toEqual({
			ids: [victimA.name, victimB.name],
		});
		expect("force" in removeRequests.bodies[0]).toBe(false);
		await waitFor(() =>
			expect(listRequests.count).toBeGreaterThan(listCountBefore),
		);
	});

	it("reports every entry, successes and failures with their messages", async () => {
		installHandlers((body) =>
			body.ids.map((id) =>
				id === victimB.name
					? {
							id,
							success: false,
							error: {
								code: "SECRET_IN_USE",
								message: "secret is in use by container abc",
							},
						}
					: { id, success: true },
			),
		);
		const user = await openBulkRemove(victimA.name, victimB.name);

		await user.click(screen.getByTestId("confirm-bulk-remove-button"));

		const report = await screen.findByTestId("bulk-remove-report");
		const rows = within(report).getAllByTestId("bulk-remove-report-row");
		expect(rows).toHaveLength(2);
		expect(within(rows[0]).getByText("victim-a")).toBeInTheDocument();
		expect(within(rows[0]).getByText("Removed")).toBeInTheDocument();
		expect(within(rows[1]).getByText("victim-b")).toBeInTheDocument();
		expect(
			within(rows[1]).getByText("secret is in use by container abc"),
		).toBeInTheDocument();
	});

	it("reports in-use refusals without offering a force retry", async () => {
		installHandlers((body) =>
			body.ids.map((id) => ({
				id,
				success: false,
				error: {
					code: "SECRET_IN_USE",
					message: "secret is in use by container abc",
				},
			})),
		);
		const user = await openBulkRemove(victimA.name, victimB.name);

		await user.click(screen.getByTestId("confirm-bulk-remove-button"));

		const report = await screen.findByTestId("bulk-remove-report");
		expect(
			within(report).getAllByText("secret is in use by container abc"),
		).toHaveLength(2);
		// No force path exists for secrets, so the retry seam is absent
		// rather than disabled.
		expect(
			screen.queryByTestId("force-remove-failed-button"),
		).not.toBeInTheDocument();
		expect(removeRequests.count).toBe(1);
	});

	// #468: "The list refetches after the run; the selection clears." The
	// clear rides the run's success, not the dialog's close, so the report
	// stays open over a list that has already let go of its selection. The
	// toolbar button is the selection's visible edge and, unlike the row
	// checkboxes, it stays queryable while the modal hides the page.
	it("clears the selection after a successful run, while the report stays open", async () => {
		installHandlers();
		const user = await openBulkRemove(victimA.name, victimB.name);

		await user.click(screen.getByTestId("confirm-bulk-remove-button"));
		await screen.findByTestId("bulk-remove-report");

		expect(screen.getByTestId("modal")).toBeInTheDocument();
		expect(
			screen.queryByTestId("remove-selected-secrets-button"),
		).not.toBeInTheDocument();

		await user.click(screen.getByTestId("close-bulk-remove-report-button"));
		for (const checkbox of screen.getAllByRole("checkbox")) {
			expect(checkbox).not.toBeChecked();
		}
	});

	// A failed run removed nothing, so the selection survives: the user
	// retries the same rows without re-picking them.
	it("keeps the selection when the run fails", async () => {
		installHandlers();
		server.use(
			http.post("*/api/hosts/m-1/secrets/remove", () =>
				errorResponse(500, "INTERNAL_ERROR", "boom"),
			),
		);
		const user = await openBulkRemove(victimA.name, victimB.name);

		await user.click(screen.getByTestId("confirm-bulk-remove-button"));

		expect(await screen.findByRole("alert")).toBeInTheDocument();
		expect(
			screen.getByTestId("remove-selected-secrets-button"),
		).toBeInTheDocument();

		await user.click(screen.getByRole("button", { name: "Cancel" }));
		for (const checkbox of screen.getAllByRole("checkbox")) {
			expect(checkbox).toBeChecked();
		}
	});
});
