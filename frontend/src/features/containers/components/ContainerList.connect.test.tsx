import { ContainerList } from "@features/containers/components/ContainerList";
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import {
	containerFixture,
	hostFixture,
	networkFixture,
} from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

// c-1 is already attached to app-net.
const attached = containerFixture({
	id: "c-1",
	names: ["/web"],
	networks: ["app-net"],
});

// The list-level connect suite keeps only the row-button surface: gating
// and dialog-open. The dialog's own behavior (options, submit, error) is
// pinned in the networks feature's ConnectDialog.test.tsx.
function mockLists() {
	return [
		http.get("*/api/hosts/m-1/containers", () =>
			HttpResponse.json({
				containers: [attached],
				total: 1,
				timestamp: "2026-08-26T00:00:00Z",
			}),
		),
		http.get("*/api/hosts/m-1/networks", () =>
			HttpResponse.json([
				networkFixture(),
				networkFixture({ name: "other-net", subnets: ["10.89.0.0/16"] }),
			]),
		),
	];
}

describe("ContainerList connect to network", () => {
	it("shows a Connect to network button per row at write level and opens the dialog with the container fixed", async () => {
		server.use(...mockLists());
		renderWithQuery(<ContainerList host={hostFixture()} />, {
			router: true,
		});

		const row = await screen.findByTestId("container-row");
		const user = userEvent.setup();
		await user.click(
			within(row).getByRole("button", { name: "Connect to network web" }),
		);

		expect(await screen.findByTestId("modal")).toBeInTheDocument();
		expect(screen.getByText("Connect web to a network")).toBeInTheDocument();
	});

	it("hides the button below manage-networks while Logs stays visible", async () => {
		server.use(...mockLists());
		renderWithQuery(
			<ContainerList
				host={hostFixture({ permission: "read", isOwner: false })}
			/>,
			{ router: true },
		);

		const row = await screen.findByTestId("container-row");
		expect(
			within(row).queryByRole("button", { name: /connect to network/i }),
		).not.toBeInTheDocument();
		expect(
			within(row).getByRole("button", { name: /logs/i }),
		).toBeInTheDocument();
	});
});
