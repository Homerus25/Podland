import { NetworkList } from "@features/networks/components/NetworkList";
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import {
	containerFixture,
	machineFixture,
	networkFixture,
} from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

// The list-level connect suite keeps only the row-button surface: gating
// and dialog-open. The dialog's own behavior (options, submit, error) is
// pinned in ConnectDialog.test.tsx.
function mockLists() {
	return [
		http.get("*/api/machines/m-1/networks", () =>
			HttpResponse.json([networkFixture()]),
		),
		http.get("*/api/machines/m-1/containers", () =>
			HttpResponse.json({
				containers: [containerFixture()],
				total: 1,
				timestamp: "2026-08-26T00:00:00Z",
			}),
		),
	];
}

describe("NetworkList connect", () => {
	it("opens the connect dialog from the row button with the network fixed", async () => {
		server.use(...mockLists());
		renderWithQuery(<NetworkList machine={machineFixture()} />, {
			router: true,
		});
		const row = await screen.findByTestId("network-row");
		const user = userEvent.setup();
		await user.click(
			within(row).getByRole("button", { name: "Connect container app-net" }),
		);

		expect(await screen.findByTestId("modal")).toBeInTheDocument();
		expect(
			screen.getByText("Connect container to app-net"),
		).toBeInTheDocument();
	});

	it("hides the row's connect button below manage-networks", async () => {
		server.use(...mockLists());
		renderWithQuery(
			<NetworkList
				machine={machineFixture({ permission: "read", isOwner: false })}
			/>,
			{ router: true },
		);

		const row = await screen.findByTestId("network-row");
		expect(
			within(row).queryByRole("button", { name: /connect container/i }),
		).not.toBeInTheDocument();
	});
});
