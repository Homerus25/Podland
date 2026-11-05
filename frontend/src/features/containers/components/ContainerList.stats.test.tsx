import { ContainerList } from "@features/containers/components/ContainerList";
import type { Host, HostPermission } from "@shared/types/api";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import {
	containerFixture,
	containerStatsFixture,
	containerStatsListFixture,
	hostFixture,
} from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

function mockContainers() {
	server.use(
		http.get("*/api/hosts/m-1/containers", () =>
			HttpResponse.json({
				containers: [containerFixture()],
				total: 1,
				timestamp: "2026-10-30T00:00:00Z",
			}),
		),
	);
}

function mockAllStats() {
	let requests = 0;
	server.use(
		http.get("*/api/hosts/m-1/containers/stats", () => {
			requests++;
			return HttpResponse.json(
				containerStatsListFixture([
					containerStatsFixture({ name: "my-container" }),
				]),
			);
		}),
	);
	return {
		requestCount: () => requests,
	};
}

function renderContainerList(host: Host = hostFixture()) {
	return renderWithQuery(<ContainerList host={host} />, {
		router: true,
	});
}

function granteeWith(permission: HostPermission): Host {
	return hostFixture({ permission, isOwner: false });
}

describe("ContainerList stats dialog wiring", () => {
	it("opens the all-stats dialog from the toolbar's Stats button", async () => {
		mockContainers();
		const stats = mockAllStats();
		renderContainerList();

		expect(stats.requestCount()).toBe(0);
		await userEvent.click(await screen.findByTestId("container-stats-button"));

		expect(
			await screen.findByTestId("container-stats-table"),
		).toBeInTheDocument();
		expect(stats.requestCount()).toBe(1);
		expect(screen.getByTestId("container-stats-row")).toHaveTextContent(
			"my-container",
		);
	});

	it("keeps the Stats button visible and wired at read level", async () => {
		mockContainers();
		const stats = mockAllStats();
		renderContainerList(granteeWith("read"));

		const statsButton = await screen.findByTestId("container-stats-button");
		expect(statsButton).toBeInTheDocument();
		expect(
			screen.queryByTestId("create-container-button"),
		).not.toBeInTheDocument();
		expect(
			screen.queryByTestId("prune-containers-button"),
		).not.toBeInTheDocument();

		await userEvent.click(statsButton);
		expect(
			await screen.findByTestId("container-stats-table"),
		).toBeInTheDocument();
		expect(stats.requestCount()).toBe(1);
	});
});
