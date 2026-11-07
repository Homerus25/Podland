import { ImageList } from "@features/images/components/ImageList";
import type { ImageSummary } from "@features/images/services/imagesApi";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http, type JsonBodyType } from "msw";
import { useLocation } from "react-router-dom";
import { beforeEach, describe, expect, it } from "vitest";
import { errorResponse } from "@/test/errors";
import { hostFixture, imageFixture } from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

// The bulk-remove flow (spec 461, ticket #469) at the MSW/HTTP seam: the
// request shapes (the plain first pass, the forced retry carrying only the
// failures), the per-entry report, the list refetch, and the selection
// clearing. The dialog skeleton itself is pinned once in the shared
// ReportDialog suite; this suite keeps the image domain assertions —
// including the id→label map the adapter captures at open time, because an
// image's id is opaque and the selection is cleared on close.

const victimA = imageFixture({
	id: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	repoTags: ["docker.io/library/nginx:1.27"],
});
const victimB = imageFixture({
	id: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	repoTags: ["docker.io/library/redis:7"],
});

type RemoveBody = { ids: string[]; force?: boolean };

let listRequests: { count: number; images: () => ImageSummary[] } = {
	count: 0,
	images: () => [],
};
let removeRequests: {
	count: number;
	pathname: string;
	bodies: RemoveBody[];
} = { count: 0, pathname: "", bodies: [] };

function resetCounters() {
	listRequests = { count: 0, images: () => [victimA, victimB] };
	removeRequests = { count: 0, pathname: "", bodies: [] };
}

function installHandlers(
	removeResponse: (body: RemoveBody) => JsonBodyType = (body) =>
		body.ids.map((id) => ({ id, success: true })),
	removeOnSuccess = false,
) {
	server.use(
		http.get("*/api/hosts/m-1/images", () => {
			listRequests.count += 1;
			return HttpResponse.json(listRequests.images());
		}),
		http.post("*/api/hosts/m-1/images/remove", async ({ request }) => {
			removeRequests.count += 1;
			removeRequests.pathname = new URL(request.url).pathname;
			const body = (await request.json()) as RemoveBody;
			removeRequests.bodies.push(body);
			if (removeOnSuccess) {
				const removed = new Set(body.ids);
				const before = listRequests.images;
				listRequests.images = () =>
					before().filter((image) => !removed.has(image.id));
			}
			return HttpResponse.json(removeResponse(body));
		}),
	);
}

function LocationProbe() {
	const location = useLocation();
	return <div data-testid="location">{location.pathname}</div>;
}

function renderImageList(permission: "read" | "write" | "admin" = "admin") {
	renderWithQuery(
		<>
			<LocationProbe />
			<ImageList
				host={hostFixture({ permission, isOwner: permission === "admin" })}
			/>
		</>,
		{ chakra: true, router: { initialEntries: ["/hosts/m-1"] } },
	);
}

// The row checkbox's accessible name is the row's stable key (the shared
// selection contract derives it from `getKey`), which for an image is its
// opaque id — the tag is the display label, not the identity.
async function openBulkRemove(...ids: string[]) {
	renderImageList();
	await screen.findByText("docker.io/library/nginx:1.27");
	const user = userEvent.setup();
	for (const id of ids) {
		await user.click(screen.getByRole("checkbox", { name: `Select ${id}` }));
	}
	await user.click(screen.getByTestId("remove-selected-images-button"));
	expect(await screen.findByTestId("modal")).toBeInTheDocument();
	return user;
}

describe("ImageList bulk remove", () => {
	beforeEach(resetCounters);

	it("renders Remove selected permanently, disabled while nothing is selected", async () => {
		installHandlers();
		renderImageList();
		await screen.findByText("docker.io/library/nginx:1.27");
		const user = userEvent.setup();

		expect(screen.getByTestId("remove-selected-images-button")).toBeDisabled();

		const checkbox = screen.getByRole("checkbox", {
			name: `Select ${victimA.id}`,
		});
		await user.click(checkbox);
		expect(screen.getByTestId("remove-selected-images-button")).toBeEnabled();

		await user.click(checkbox);
		expect(screen.getByTestId("remove-selected-images-button")).toBeDisabled();
	});

	it("hides the selection from read-level users", async () => {
		installHandlers();
		renderImageList("read");
		await screen.findByText("docker.io/library/nginx:1.27");

		expect(screen.queryAllByRole("checkbox")).toHaveLength(0);
		expect(
			screen.queryByTestId("remove-selected-images-button"),
		).not.toBeInTheDocument();
	});

	it("selects a row without navigating to its detail page", async () => {
		installHandlers();
		renderImageList();
		await screen.findByText("docker.io/library/nginx:1.27");
		const user = userEvent.setup();

		await user.click(
			screen.getByRole("checkbox", { name: `Select ${victimA.id}` }),
		);

		expect(screen.getByTestId("location").textContent).toBe("/hosts/m-1");
	});

	it("confirms with the count and the selected labels, sending nothing yet", async () => {
		installHandlers();
		await openBulkRemove(victimA.id, victimB.id);

		expect(screen.getByText(/remove 2 images/i)).toBeInTheDocument();
		const names = screen.getByTestId("bulk-remove-names");
		expect(
			within(names).getByText("docker.io/library/nginx:1.27 (aaaaaaaaaaaa)"),
		).toBeInTheDocument();
		expect(
			within(names).getByText("docker.io/library/redis:7 (bbbbbbbbbbbb)"),
		).toBeInTheDocument();
		expect(removeRequests.count).toBe(0);
	});

	it("sends one plain bulk request with the selected ids and refetches the list", async () => {
		installHandlers(undefined, true);
		const user = await openBulkRemove(victimA.id, victimB.id);
		const listCountBefore = listRequests.count;

		await user.click(screen.getByTestId("confirm-bulk-remove-button"));

		await waitFor(() => expect(removeRequests.count).toBe(1));
		expect(removeRequests.pathname).toBe("/api/hosts/m-1/images/remove");
		// The first pass is plain: the body carries no force at all.
		expect(removeRequests.bodies[0]).toEqual({
			ids: [victimA.id, victimB.id],
		});
		await waitFor(() =>
			expect(listRequests.count).toBeGreaterThan(listCountBefore),
		);
	});

	it("reports every entry by its captured label, successes and failures with their messages", async () => {
		installHandlers((body) =>
			body.ids.map((id) =>
				id === victimB.id
					? {
							id,
							success: false,
							error: {
								code: "IMAGE_USED_BY_CONTAINERS",
								message: "image is in use by container abc",
							},
						}
					: { id, success: true },
			),
		);
		const user = await openBulkRemove(victimA.id, victimB.id);

		await user.click(screen.getByTestId("confirm-bulk-remove-button"));

		const report = await screen.findByTestId("bulk-remove-report");
		const rows = within(report).getAllByTestId("bulk-remove-report-row");
		expect(rows).toHaveLength(2);
		expect(
			within(rows[0]).getByText("docker.io/library/nginx:1.27 (aaaaaaaaaaaa)"),
		).toBeInTheDocument();
		expect(within(rows[0]).getByText("Removed")).toBeInTheDocument();
		expect(
			within(rows[1]).getByText("docker.io/library/redis:7 (bbbbbbbbbbbb)"),
		).toBeInTheDocument();
		expect(
			within(rows[1]).getByText("image is in use by container abc"),
		).toBeInTheDocument();
	});

	it("force-removes only the failed ids on the retry", async () => {
		installHandlers((body) =>
			body.ids.map((id) =>
				id === victimB.id && !body.force
					? {
							id,
							success: false,
							error: {
								code: "IMAGE_USED_BY_CONTAINERS",
								message: "image is in use by container abc",
							},
						}
					: { id, success: true },
			),
		);
		const user = await openBulkRemove(victimA.id, victimB.id);

		await user.click(screen.getByTestId("confirm-bulk-remove-button"));

		const retry = await screen.findByTestId("force-remove-failed-button");
		expect(retry).toHaveTextContent("Force remove 1 failed");
		await user.click(retry);

		await waitFor(() => expect(removeRequests.count).toBe(2));
		expect(removeRequests.bodies[1]).toEqual({
			ids: [victimB.id],
			force: true,
		});
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
		const user = await openBulkRemove(victimA.id, victimB.id);

		await user.click(screen.getByTestId("confirm-bulk-remove-button"));
		await screen.findByTestId("bulk-remove-report");

		expect(screen.getByTestId("modal")).toBeInTheDocument();
		expect(screen.getByTestId("remove-selected-images-button")).toBeDisabled();

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
			http.post("*/api/hosts/m-1/images/remove", () =>
				errorResponse(500, "INTERNAL_ERROR", "boom"),
			),
		);
		const user = await openBulkRemove(victimA.id, victimB.id);

		await user.click(screen.getByTestId("confirm-bulk-remove-button"));

		expect(await screen.findByRole("alert")).toBeInTheDocument();
		expect(
			screen.getByTestId("remove-selected-images-button"),
		).toBeInTheDocument();

		await user.click(screen.getByRole("button", { name: "Cancel" }));
		for (const checkbox of screen.getAllByRole("checkbox")) {
			expect(checkbox).toBeChecked();
		}
	});
});
