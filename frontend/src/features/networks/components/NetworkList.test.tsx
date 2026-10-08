import { NetworkList } from "@features/networks/components/NetworkList";
import type { NetworkSummary } from "@features/networks/services/networksApi";
import { networksQueryKey } from "@shared/queries/queryKeys";
import { screen, within } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import { afterEach, describe, expect, it, vi } from "vitest";
import { errorResponse } from "@/test/errors";
import { networkFixture } from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

function mockNetworks(networks: NetworkSummary[]) {
	server.use(
		http.get("*/api/machines/m-1/networks", () => HttpResponse.json(networks)),
	);
}

function renderNetworkList() {
	return renderWithQuery(<NetworkList machineId="m-1" />);
}

afterEach(() => {
	vi.useRealTimers();
});

describe("NetworkList", () => {
	it("renders one row per network with name, driver, and subnets", async () => {
		mockNetworks([
			networkFixture(),
			networkFixture({
				name: "host-net",
				driver: "host",
				subnets: [],
				createdAt: "2026-08-22T12:00:00Z",
			}),
		]);
		renderNetworkList();

		const rows = await screen.findAllByTestId("network-row");
		expect(rows).toHaveLength(2);

		const first = within(rows[0]);
		expect(first.getByText("app-net")).toBeInTheDocument();
		expect(first.getByText("bridge")).toBeInTheDocument();
		expect(first.getByText("10.88.0.0/16")).toBeInTheDocument();

		const second = within(rows[1]);
		expect(second.getByText("host-net")).toBeInTheDocument();
		expect(second.getByText("host")).toBeInTheDocument();
	});

	it("shows the relative creation age of each network", async () => {
		vi.useFakeTimers({ shouldAdvanceTime: true });
		vi.setSystemTime(new Date("2026-08-23T12:00:00Z"));
		mockNetworks([networkFixture({ createdAt: "2026-08-20T10:00:00Z" })]);
		renderNetworkList();

		expect(await screen.findByText("3 days ago")).toBeInTheDocument();
	});

	it("joins multiple subnets as CIDRs in one cell", async () => {
		mockNetworks([
			networkFixture({ subnets: ["10.88.0.0/16", "fd00:88::/64"] }),
		]);
		renderNetworkList();

		const row = await screen.findByTestId("network-row");
		expect(
			within(row).getByText("10.88.0.0/16, fd00:88::/64"),
		).toBeInTheDocument();
	});

	it("caches networks under the per-machine query key hierarchy", async () => {
		mockNetworks([networkFixture()]);
		const { queryClient } = renderNetworkList();

		await screen.findByText("app-net");
		const cached = queryClient.getQueryData<NetworkSummary[]>(
			networksQueryKey("m-1"),
		);
		expect(cached).toHaveLength(1);
		expect(cached?.[0].name).toBe("app-net");
	});

	it("shows a loading state before networks arrive", async () => {
		server.use(
			http.get("*/api/machines/m-1/networks", async () => {
				await new Promise((resolve) => setTimeout(resolve, 50));
				return HttpResponse.json([networkFixture()]);
			}),
		);
		renderNetworkList();

		expect(screen.getByTestId("networks-loading")).toBeInTheDocument();
	});

	it("shows the empty state when the machine has no networks", async () => {
		mockNetworks([]);
		renderNetworkList();

		const empty = await screen.findByTestId("networks-empty");
		expect(empty).toBeInTheDocument();
		expect(screen.queryByTestId("network-row")).not.toBeInTheDocument();
	});

	it("shows the error state through the shared error display", async () => {
		server.use(
			http.get("*/api/machines/m-1/networks", () =>
				errorResponse(403, "ACCESS_DENIED", "Access denied", "req-1"),
			),
		);
		renderNetworkList();

		const error = await screen.findByTestId("networks-error");
		expect(error).toHaveTextContent("ACCESS_DENIED");
		expect(error).toHaveTextContent("Access denied");
		expect(screen.queryByTestId("network-row")).not.toBeInTheDocument();
	});

	it("renders a row's cells in column order", async () => {
		vi.useFakeTimers({ shouldAdvanceTime: true });
		vi.setSystemTime(new Date("2026-08-23T12:00:00Z"));
		mockNetworks([networkFixture()]);
		renderNetworkList();

		const row = await screen.findByTestId("network-row");
		const cells = within(row).getAllByRole("cell");
		expect(cells.map((cell) => cell.textContent)).toEqual([
			"app-net",
			"bridge",
			"10.88.0.0/16",
			"3 days ago",
		]);
	});

	it("renders no toolbar and no permission hint — the surface is ungated", async () => {
		mockNetworks([networkFixture()]);
		renderNetworkList();

		await screen.findByTestId("network-row");
		expect(
			screen.queryByTestId("networks-permission-hint"),
		).not.toBeInTheDocument();
	});
});
