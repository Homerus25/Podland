import { QuadletList } from "@features/quadlets/components/QuadletList";
import type { QuadletSummary } from "@features/quadlets/services/quadletsApi";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http, type JsonBodyType } from "msw";
import { beforeEach, describe, expect, it } from "vitest";
import { errorResponse } from "@/test/errors";
import { hostFixture, quadletFixture } from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

// The bulk-remove flow (spec 461, ticket #469) at the MSW/HTTP seam: the
// request shapes (the plain first pass, the forced retry carrying only the
// failures), the per-entry report, the list refetch, and the selection
// clearing. The dialog skeleton itself is pinned once in the shared
// ReportDialog suite; this suite keeps the quadlet domain assertions.
//
// Quadlets carry no extra options in the UI: stopping dependent containers
// is the backend's silent default, so the body is exactly {ids, force} —
// the exact-body assertions below are what pins that.

const web = quadletFixture({ id: "web", name: "web", state: "stopped" });
const db = quadletFixture({
	id: "db",
	name: "db",
	unit: "db.service",
	state: "stopped",
});

type RemoveBody = { ids: string[]; force?: boolean };

let listRequests: { count: number; quadlets: () => QuadletSummary[] } = {
	count: 0,
	quadlets: () => [],
};
let removeRequests: {
	count: number;
	pathname: string;
	bodies: RemoveBody[];
} = { count: 0, pathname: "", bodies: [] };

function resetCounters() {
	listRequests = { count: 0, quadlets: () => [web, db] };
	removeRequests = { count: 0, pathname: "", bodies: [] };
}

function installHandlers(
	removeResponse: (body: RemoveBody) => JsonBodyType = (body) =>
		body.ids.map((id) => ({ id, success: true })),
	removeOnSuccess = false,
) {
	server.use(
		http.get("*/api/hosts/m-1/quadlets", () => {
			listRequests.count += 1;
			return HttpResponse.json(listRequests.quadlets());
		}),
		http.post("*/api/hosts/m-1/quadlets/remove", async ({ request }) => {
			removeRequests.count += 1;
			removeRequests.pathname = new URL(request.url).pathname;
			const body = (await request.json()) as RemoveBody;
			removeRequests.bodies.push(body);
			if (removeOnSuccess) {
				const removed = new Set(body.ids);
				const before = listRequests.quadlets;
				listRequests.quadlets = () =>
					before().filter((quadlet) => !removed.has(quadlet.id));
			}
			return HttpResponse.json(removeResponse(body));
		}),
	);
}

function renderQuadletList(permission: "read" | "write" | "admin" = "admin") {
	renderWithQuery(
		<QuadletList
			host={hostFixture({ permission, isOwner: permission === "admin" })}
		/>,
		{ router: true },
	);
}

// The row checkbox's accessible name is the row's stable key (the shared
// selection contract derives it from `getKey`), which for a quadlet is its
// id — the name is the display label, not the identity.
async function openBulkRemove(...ids: string[]) {
	renderQuadletList();
	await screen.findByText("web");
	const user = userEvent.setup();
	for (const id of ids) {
		await user.click(screen.getByRole("checkbox", { name: `Select ${id}` }));
	}
	await user.click(screen.getByTestId("remove-selected-quadlets-button"));
	expect(await screen.findByTestId("modal")).toBeInTheDocument();
	return user;
}

describe("QuadletList bulk remove", () => {
	beforeEach(resetCounters);

	it("renders Remove selected permanently, disabled while nothing is selected", async () => {
		installHandlers();
		renderQuadletList();
		await screen.findByText("web");
		const user = userEvent.setup();

		expect(
			screen.getByTestId("remove-selected-quadlets-button"),
		).toBeDisabled();

		const checkbox = screen.getByRole("checkbox", { name: `Select ${web.id}` });
		await user.click(checkbox);
		expect(screen.getByTestId("remove-selected-quadlets-button")).toBeEnabled();

		await user.click(checkbox);
		expect(
			screen.getByTestId("remove-selected-quadlets-button"),
		).toBeDisabled();
	});

	// Spec 461, ticket #470: quadlets have no detail page, so a single quadlet
	// is removed by selecting it and using the same bulk button — the one
	// removal path the resource has left.
	it("removes a single quadlet through the bulk button", async () => {
		installHandlers(undefined, true);
		const user = await openBulkRemove(web.id);

		expect(screen.getByText(/remove 1 quadlet/i)).toBeInTheDocument();
		await user.click(screen.getByTestId("confirm-bulk-remove-button"));

		await waitFor(() => expect(removeRequests.count).toBe(1));
		expect(removeRequests.bodies[0]).toEqual({ ids: [web.id] });

		// The report names the removed quadlet, so close it before asserting
		// the row itself is gone from the refetched table.
		await screen.findByTestId("bulk-remove-report");
		await user.click(screen.getByTestId("close-bulk-remove-report-button"));
		await waitFor(() =>
			expect(screen.queryByText("web")).not.toBeInTheDocument(),
		);
		expect(screen.getByText("db")).toBeInTheDocument();
	});

	it("hides the selection from read-level users", async () => {
		installHandlers();
		renderQuadletList("read");
		await screen.findByText("web");

		expect(screen.queryAllByRole("checkbox")).toHaveLength(0);
		expect(
			screen.queryByTestId("remove-selected-quadlets-button"),
		).not.toBeInTheDocument();
	});

	it("confirms with the count and the selected names, sending nothing yet", async () => {
		installHandlers();
		await openBulkRemove(web.id, db.id);

		expect(screen.getByText(/remove 2 quadlets/i)).toBeInTheDocument();
		const names = screen.getByTestId("bulk-remove-names");
		expect(within(names).getByText("web")).toBeInTheDocument();
		expect(within(names).getByText("db")).toBeInTheDocument();
		expect(removeRequests.count).toBe(0);
	});

	it("sends one plain bulk request with the selected ids and refetches the list", async () => {
		installHandlers(undefined, true);
		const user = await openBulkRemove(web.id, db.id);
		const listCountBefore = listRequests.count;

		await user.click(screen.getByTestId("confirm-bulk-remove-button"));

		await waitFor(() => expect(removeRequests.count).toBe(1));
		expect(removeRequests.pathname).toBe("/api/hosts/m-1/quadlets/remove");
		// The first pass is plain and carries nothing but the ids: no force,
		// and no stop/restart options either — the backend's silent default
		// stops dependent containers.
		expect(removeRequests.bodies[0]).toEqual({ ids: [web.id, db.id] });
		await waitFor(() =>
			expect(listRequests.count).toBeGreaterThan(listCountBefore),
		);
	});

	it("reports every entry, successes and failures with their messages", async () => {
		installHandlers((body) =>
			body.ids.map((id) =>
				id === db.id
					? {
							id,
							success: false,
							error: {
								code: "QUADLET_STOP_FAILED",
								message: "unit db.service is masked",
							},
						}
					: { id, success: true },
			),
		);
		const user = await openBulkRemove(web.id, db.id);

		await user.click(screen.getByTestId("confirm-bulk-remove-button"));

		const report = await screen.findByTestId("bulk-remove-report");
		const rows = within(report).getAllByTestId("bulk-remove-report-row");
		expect(rows).toHaveLength(2);
		expect(within(rows[0]).getByText("web")).toBeInTheDocument();
		expect(within(rows[0]).getByText("Removed")).toBeInTheDocument();
		expect(within(rows[1]).getByText("db")).toBeInTheDocument();
		expect(
			within(rows[1]).getByText("unit db.service is masked"),
		).toBeInTheDocument();
	});

	it("force-removes only the failed ids on the retry", async () => {
		installHandlers((body) =>
			body.ids.map((id) =>
				id === db.id && !body.force
					? {
							id,
							success: false,
							error: {
								code: "QUADLET_STOP_FAILED",
								message: "unit db.service is masked",
							},
						}
					: { id, success: true },
			),
		);
		const user = await openBulkRemove(web.id, db.id);

		await user.click(screen.getByTestId("confirm-bulk-remove-button"));

		const retry = await screen.findByTestId("force-remove-failed-button");
		expect(retry).toHaveTextContent("Force remove 1 failed");
		await user.click(retry);

		await waitFor(() => expect(removeRequests.count).toBe(2));
		expect(removeRequests.bodies[1]).toEqual({ ids: [db.id], force: true });
		await waitFor(() =>
			expect(
				screen.queryByTestId("force-remove-failed-button"),
			).not.toBeInTheDocument(),
		);
	});

	// #468: "The list refetches after the run; the selection clears." The
	// clear rides the run's success, not the dialog's close, so the report
	// stays open over a list that has already let go of its selection. The
	// toolbar button is the selection's visible edge and, unlike the row
	// checkboxes, it stays queryable while the modal hides the page —
	// rendered permanently (ticket #493), its disabled state is the
	// cleared selection's evidence.
	it("clears the selection after a successful run, while the report stays open", async () => {
		installHandlers();
		const user = await openBulkRemove(web.id, db.id);

		await user.click(screen.getByTestId("confirm-bulk-remove-button"));
		await screen.findByTestId("bulk-remove-report");

		expect(screen.getByTestId("modal")).toBeInTheDocument();
		expect(
			screen.getByTestId("remove-selected-quadlets-button"),
		).toBeDisabled();

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
			http.post("*/api/hosts/m-1/quadlets/remove", () =>
				errorResponse(500, "INTERNAL_ERROR", "boom"),
			),
		);
		const user = await openBulkRemove(web.id, db.id);

		await user.click(screen.getByTestId("confirm-bulk-remove-button"));

		expect(await screen.findByRole("alert")).toBeInTheDocument();
		expect(
			screen.getByTestId("remove-selected-quadlets-button"),
		).toBeInTheDocument();

		await user.click(screen.getByRole("button", { name: "Cancel" }));
		for (const checkbox of screen.getAllByRole("checkbox")) {
			expect(checkbox).toBeChecked();
		}
	});
});
