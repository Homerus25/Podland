import { ContainerStatsDialog } from "@features/containers/components/ContainerStatsDialog";
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { useState } from "react";
import { describe, expect, it } from "vitest";
import { errorResponse } from "@/test/errors";
import {
	containerStatsFixture,
	containerStatsListFixture,
} from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

function mockAllStats(responder: () => ReturnType<typeof HttpResponse.json>) {
	server.use(http.get("*/api/machines/m-1/containers/stats", responder));
}

function mockContainerStats(
	responder: () => ReturnType<typeof HttpResponse.json>,
) {
	server.use(http.get("*/api/machines/m-1/containers/c-1/stats", responder));
}

// The pages mount the dialog conditionally (mount is open, unmount is
// close), so the lifecycle tests ride the same host shape.
function StatsDialogHost({ containerId }: { containerId?: string }) {
	const [open, setOpen] = useState(false);
	return (
		<>
			<button type="button" onClick={() => setOpen(true)}>
				open
			</button>
			{open ? (
				<ContainerStatsDialog
					machineId="m-1"
					containerId={containerId}
					onClose={() => setOpen(false)}
				/>
			) : null}
		</>
	);
}

function renderStatsDialog(containerId?: string) {
	return renderWithQuery(<StatsDialogHost containerId={containerId} />, {
		chakra: true,
	});
}

async function openDialog() {
	await userEvent.click(screen.getByRole("button", { name: "open" }));
}

describe("ContainerStatsDialog", () => {
	it("renders the snapshot table with human-readable values for every running container", async () => {
		mockAllStats(() =>
			HttpResponse.json(
				containerStatsListFixture([
					containerStatsFixture(),
					containerStatsFixture({
						id: "c-2",
						name: "db",
						cpuPercent: 0.5,
						memoryUsage: 500_000_000,
						memoryLimit: 4_000_000_000,
						memoryPercent: 12.5,
						networkBytesIn: 2_500_000,
						networkBytesOut: 75_000,
						blockBytesIn: 10_000_000_000,
						blockBytesOut: 0,
						pids: 7,
					}),
				]),
			),
		);
		renderStatsDialog();
		await openDialog();

		const table = await screen.findByTestId("container-stats-table");
		const rows = within(table).getAllByTestId("container-stats-row");
		expect(rows).toHaveLength(2);

		const web = rows[0];
		expect(web).toHaveTextContent("web");
		expect(web).toHaveTextContent("12.34%");
		expect(web).toHaveTextContent("1.5 KB / 2 GB (42.1%)");
		expect(web).toHaveTextContent("999 B in / 1.5 MB out");
		expect(web).toHaveTextContent("250 B in / 3 GB out");
		expect(web).toHaveTextContent("42");

		const db = rows[1];
		expect(db).toHaveTextContent("db");
		expect(db).toHaveTextContent("0.5%");
		expect(db).toHaveTextContent("500 MB / 4 GB (12.5%)");
		expect(db).toHaveTextContent("2.5 MB in / 75 KB out");
		expect(db).toHaveTextContent("10 GB in / 0 B out");
		expect(db).toHaveTextContent("7");
	});

	it("scopes the table to the one container on the details page", async () => {
		mockContainerStats(() => HttpResponse.json(containerStatsFixture()));
		renderStatsDialog("c-1");
		await openDialog();

		const table = await screen.findByTestId("container-stats-table");
		const rows = within(table).getAllByTestId("container-stats-row");
		expect(rows).toHaveLength(1);
		expect(rows[0]).toHaveTextContent("web");
		expect(rows[0]).toHaveTextContent("12.34%");
	});

	it("shows the loading state while the snapshot is sampled", async () => {
		mockAllStats(
			() =>
				new Promise(() => {
					/* never resolves: the ~1s sample pause */
				}) as unknown as ReturnType<typeof HttpResponse.json>,
		);
		renderStatsDialog();
		await openDialog();

		expect(
			await screen.findByTestId("container-stats-loading"),
		).toBeInTheDocument();
		expect(
			screen.queryByTestId("container-stats-table"),
		).not.toBeInTheDocument();
	});

	it("renders the error envelope's code and message in the error region", async () => {
		mockAllStats(() =>
			errorResponse(403, "ACCESS_DENIED", "no access to this machine"),
		);
		renderStatsDialog();
		await openDialog();

		const error = await screen.findByTestId("container-stats-error");
		expect(error).toHaveTextContent("ACCESS_DENIED");
		expect(error).toHaveTextContent("no access to this machine");
		expect(
			screen.queryByTestId("container-stats-table"),
		).not.toBeInTheDocument();
	});

	it("surfaces the backend's 404 message for a stopped container", async () => {
		mockContainerStats(() =>
			errorResponse(
				404,
				"CONTAINER_NOT_FOUND",
				"container c-1 has no stats because it is not running",
			),
		);
		renderStatsDialog("c-1");
		await openDialog();

		const error = await screen.findByTestId("container-stats-error");
		expect(error).toHaveTextContent("CONTAINER_NOT_FOUND");
		expect(error).toHaveTextContent(
			"container c-1 has no stats because it is not running",
		);
	});

	it("renders an explicit empty state when no containers are running", async () => {
		mockAllStats(() => HttpResponse.json(containerStatsListFixture([])));
		renderStatsDialog();
		await openDialog();

		expect(
			await screen.findByTestId("container-stats-empty"),
		).toHaveTextContent(/no containers are running/i);
		expect(
			screen.queryByTestId("container-stats-table"),
		).not.toBeInTheDocument();
	});

	it("fetches a fresh snapshot on every open and none while closed", async () => {
		let requests = 0;
		mockAllStats(() => {
			requests++;
			return HttpResponse.json(
				containerStatsListFixture([containerStatsFixture()]),
			);
		});
		renderStatsDialog();

		expect(requests).toBe(0);
		await openDialog();
		await screen.findByTestId("container-stats-table");
		expect(requests).toBe(1);

		await userEvent.click(screen.getByTestId("container-stats-close"));
		expect(
			screen.queryByTestId("container-stats-table"),
		).not.toBeInTheDocument();
		expect(requests).toBe(1);

		await openDialog();
		await screen.findByTestId("container-stats-table");
		expect(requests).toBe(2);
	});
});
