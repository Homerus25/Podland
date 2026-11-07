import { ContainerList } from "@features/containers/components/ContainerList";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import { errorJson } from "@/test/errors";
import {
	containerFixture,
	hostFixture,
	networkFixture,
} from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

// The containers list's bulk-connect adapter smokes (spec #486, ticket #491).
// The batch contract itself — one network chosen once, sequential request
// order, the per-item report, onFinished carrying only the succeeded ids, the
// already-attached-everywhere exclusion — is pinned at the connect dialog's
// component seam (ConnectDialog.bulk.test.tsx) and the runner's
// (useNetworks.test.tsx). This suite keeps only the adapter wiring: the
// toolbar button's presence, gate, and disabled state, the row's lost button,
// the dialog opening on the selection, and the selection clearing the
// succeeded containers while a failure stays.

// web and db are both on app-net, so other-net is the only eligible network.
const web = containerFixture({
	id: "c-1",
	names: ["/web"],
	networks: ["app-net"],
});
const db = containerFixture({
	id: "c-2",
	names: ["/db"],
	networks: ["app-net"],
});

function mockLists() {
	return [
		http.get("*/api/hosts/m-1/containers", () =>
			HttpResponse.json({
				containers: [web, db],
				total: 2,
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

// c-2's connect fails; c-1's succeeds.
function connectHandler() {
	return http.post(
		"*/api/hosts/m-1/networks/:networkName/connect",
		async ({ request }) => {
			const body = (await request.json()) as { container: string };
			if (body.container === "c-2") {
				return HttpResponse.json(
					errorJson("CONTAINER_NOT_FOUND", "container not found", "req-41"),
					{ status: 404 },
				);
			}
			return new HttpResponse(null, { status: 200 });
		},
	);
}

async function selectRow(
	user: ReturnType<typeof userEvent.setup>,
	containerId: string,
) {
	await user.click(
		await screen.findByRole("checkbox", { name: `Select ${containerId}` }),
	);
}

describe("ContainerList connect selected to network", () => {
	it("renders the toolbar button for a manage-networks caller, disabled while nothing is selected", async () => {
		server.use(...mockLists());
		renderWithQuery(<ContainerList host={hostFixture()} />, { router: true });

		const button = await screen.findByTestId(
			"connect-selected-containers-button",
		);
		expect(button).toHaveTextContent("Connect selected to network");
		expect(button).toBeDisabled();

		const user = userEvent.setup();
		await selectRow(user, "c-1");
		await waitFor(() =>
			expect(
				screen.getByTestId("connect-selected-containers-button"),
			).toBeEnabled(),
		);
	});

	it("opens the bulk dialog naming the selected containers", async () => {
		server.use(...mockLists());
		renderWithQuery(<ContainerList host={hostFixture()} />, { router: true });
		const user = userEvent.setup();

		await selectRow(user, "c-1");
		await selectRow(user, "c-2");
		await user.click(screen.getByTestId("connect-selected-containers-button"));

		expect(await screen.findByTestId("modal")).toBeInTheDocument();
		expect(
			screen.getByText("Connect 2 containers to a network"),
		).toBeInTheDocument();
	});

	it("keeps the lifecycle, Logs, and Stats actions on the row while the connect button is gone", async () => {
		server.use(...mockLists());
		renderWithQuery(<ContainerList host={hostFixture()} />, { router: true });

		const row = (await screen.findAllByTestId("container-row"))[0];
		expect(
			within(row).queryByTestId("connect-container-button"),
		).not.toBeInTheDocument();
		expect(
			within(row).queryByRole("button", { name: /connect to network/i }),
		).not.toBeInTheDocument();
		expect(
			within(row).getByRole("button", { name: "Stop" }),
		).toBeInTheDocument();
		expect(
			within(row).getByRole("button", { name: /restart/i }),
		).toBeInTheDocument();
		expect(
			within(row).getByRole("button", { name: /logs/i }),
		).toBeInTheDocument();
		expect(screen.getByTestId("container-stats-button")).toBeInTheDocument();
	});

	it("renders no connect affordance below manage-networks and keeps the permission hint", async () => {
		server.use(...mockLists());
		renderWithQuery(
			<ContainerList
				host={hostFixture({ permission: "read", isOwner: false })}
			/>,
			{ router: true },
		);

		const row = (await screen.findAllByTestId("container-row"))[0];
		expect(
			within(row).queryByTestId("connect-container-button"),
		).not.toBeInTheDocument();
		expect(
			screen.queryByTestId("connect-selected-containers-button"),
		).not.toBeInTheDocument();
		expect(
			screen.getByTestId("containers-permission-hint"),
		).toBeInTheDocument();
		// Logs reads at read level, so the row keeps it.
		expect(
			within(row).getByRole("button", { name: /logs/i }),
		).toBeInTheDocument();
	});

	it("clears the succeeded containers from the selection and keeps the failed one after a run", async () => {
		server.use(...mockLists(), connectHandler());
		renderWithQuery(<ContainerList host={hostFixture()} />, { router: true });
		const user = userEvent.setup();

		await selectRow(user, "c-1");
		await selectRow(user, "c-2");
		await user.click(screen.getByTestId("connect-selected-containers-button"));

		// One network for the whole selection, then the report.
		await user.click(await screen.findByTestId("searchable-select-input"));
		await user.click(
			await screen.findByTestId("searchable-select-option-other-net"),
		);
		await user.click(screen.getByTestId("connect-selected-submit"));
		await screen.findByTestId("connect-selected-report");
		// The report stays open; the selection is already updated behind it.
		await user.click(
			screen.getByTestId("close-connect-selected-report-button"),
		);

		// The succeeded container leaves the selection; the failed one stays
		// selected for the retry.
		await waitFor(() =>
			expect(
				screen.getByRole("checkbox", { name: "Select c-1" }),
			).not.toBeChecked(),
		);
		expect(screen.getByRole("checkbox", { name: "Select c-2" })).toBeChecked();
	});
});
