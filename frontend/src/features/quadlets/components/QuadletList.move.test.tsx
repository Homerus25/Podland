import { QuadletList } from "@features/quadlets/components/QuadletList";
import type { QuadletSummary } from "@features/quadlets/services/quadletsApi";
import { screen, within } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import { hostFixture, quadletFixture } from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

// What is left of the quadlets list's move/copy suite (spec 486, ticket
// #489). The row's Move/Copy button is gone — a transfer is a
// selection-scoped action now — so the dialog this file used to drive from
// that button is covered where it still lives, in
// QuadletDetailPage.move.test.tsx, and the batch adapter in
// QuadletList.moveSelected.test.tsx. What stays here is the table arithmetic
// the row change touched: the move/copy column went with the button, so the
// table is five columns for every caller instead of six for a
// manage-quadlets one.

const web = quadletFixture({ id: "web", name: "web", state: "stopped" });

function mockQuadlets(quadlets: QuadletSummary[]) {
	server.use(
		http.get("*/api/hosts/m-1/quadlets", () => HttpResponse.json(quadlets)),
	);
}

function renderQuadletList(permission: "read" | "write" | "admin" = "admin") {
	return renderWithQuery(
		<QuadletList
			host={hostFixture({ permission, isOwner: permission === "admin" })}
		/>,
		{ router: true },
	);
}

describe("QuadletList move/copy column", () => {
	it("keeps five data columns for a manage-quadlets caller, with no move/copy cell", async () => {
		mockQuadlets([web]);
		renderQuadletList();

		const row = await screen.findByTestId("quadlet-row");
		// Name, resource, state, the lifecycle/Logs cell, and the edit cell —
		// plus the selection checkbox column the list owner prepends for a
		// write-capable caller.
		expect(within(row).getAllByRole("cell")).toHaveLength(6);
		expect(screen.getAllByRole("columnheader")).toHaveLength(6);
		expect(
			within(row).queryByRole("button", { name: /move/i }),
		).not.toBeInTheDocument();
	});

	it("keeps the same five data columns for a read-only caller, without the checkbox column", async () => {
		mockQuadlets([web]);
		renderQuadletList("read");

		const row = await screen.findByTestId("quadlet-row");
		expect(within(row).getAllByRole("cell")).toHaveLength(5);
		expect(screen.getAllByRole("columnheader")).toHaveLength(5);
		expect(
			within(row).queryByRole("button", { name: /move/i }),
		).not.toBeInTheDocument();
	});
});
