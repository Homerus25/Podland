import { ContainerList } from "@features/containers/components/ContainerList";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import { errorResponse } from "@/test/errors";
import {
	containerFixture,
	machineFixture,
	networkFixture,
	rawInspectFixture,
} from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

const container = containerFixture({ id: "c-1", names: ["/web"] });

function containersHandler() {
	return http.get("*/api/machines/m-1/containers", () =>
		HttpResponse.json({
			containers: [container],
			total: 1,
			timestamp: "2026-08-26T00:00:00Z",
		}),
	);
}

function inspectHandler() {
	return http.get("*/api/machines/m-1/containers/c-1/inspect", () =>
		HttpResponse.json(rawInspectFixture()),
	);
}

function renderContainerList() {
	renderWithQuery(<ContainerList machine={machineFixture()} />, {
		router: true,
	});
}

describe("ContainerList inspect", () => {
	it("opens the inspect dialog titled with the container's name and shows its JSON", async () => {
		const requestedPaths: string[] = [];
		server.use(
			containersHandler(),
			http.get("*/api/machines/m-1/containers/c-1/inspect", ({ request }) => {
				requestedPaths.push(new URL(request.url).pathname);
				return HttpResponse.json(rawInspectFixture());
			}),
		);
		renderContainerList();
		const user = userEvent.setup();

		await user.click(await screen.findByTestId("container-row"));

		const modal = await screen.findByTestId("modal");
		expect(within(modal).getByText("web")).toBeInTheDocument();
		expect(screen.getByTestId("inspect-dialog-json").textContent).toBe(
			JSON.stringify(rawInspectFixture(), null, 2),
		);
		expect(requestedPaths).toEqual([
			"/api/machines/m-1/containers/c-1/inspect",
		]);
	});

	it("closing the dialog returns to the list", async () => {
		server.use(containersHandler(), inspectHandler());
		renderContainerList();
		const user = userEvent.setup();

		await user.click(await screen.findByTestId("container-row"));
		await screen.findByTestId("inspect-dialog-json");
		await user.click(screen.getByRole("button", { name: "Close dialog" }));

		await waitFor(() =>
			expect(screen.queryByTestId("modal")).not.toBeInTheDocument(),
		);
		expect(screen.getByTestId("container-row")).toBeInTheDocument();
	});

	it("renders the error region inside the dialog when the endpoint 404s", async () => {
		server.use(
			containersHandler(),
			http.get("*/api/machines/m-1/containers/c-1/inspect", () =>
				errorResponse(404, "CONTAINER_NOT_FOUND", "Container not found"),
			),
		);
		renderContainerList();
		const user = userEvent.setup();

		await user.click(await screen.findByTestId("container-row"));

		const alert = await screen.findByTestId("inspect-dialog-error");
		expect(alert).toHaveTextContent("CONTAINER_NOT_FOUND");
		expect(alert).toHaveTextContent("Container not found");
		expect(screen.queryByTestId("inspect-dialog-json")).not.toBeInTheDocument();
	});

	it("does not open the inspect dialog from the row's action buttons", async () => {
		let inspectRequests = 0;
		server.use(
			http.get("*/api/machines/m-1/containers", () =>
				HttpResponse.json({
					containers: [
						containerFixture({ id: "c-1", names: ["/web"], state: "running" }),
						containerFixture({ id: "c-2", names: ["/db"], state: "exited" }),
					],
					total: 2,
					timestamp: "2026-08-26T00:00:00Z",
				}),
			),
			http.post("*/api/machines/m-1/containers/*/stop", () =>
				HttpResponse.json({ id: "c-1" }),
			),
			http.post("*/api/machines/m-1/containers/*/start", () =>
				HttpResponse.json({ id: "c-2" }),
			),
			http.get("*/api/machines/m-1/networks", () =>
				HttpResponse.json([networkFixture()]),
			),
			http.get("*/api/machines/m-1/containers/*/inspect", () => {
				inspectRequests++;
				return HttpResponse.json(rawInspectFixture());
			}),
		);
		renderContainerList();
		const user = userEvent.setup();
		await screen.findAllByTestId("container-row");
		// Re-queried per click: the lifecycle mutations refetch the list.
		const rowAt = (index: number) =>
			within(screen.getAllByTestId("container-row")[index]);

		// start/stop, both lifecycle directions
		await user.click(rowAt(0).getByTestId("stop-container-button"));
		await user.click(rowAt(1).getByTestId("start-container-button"));
		// logs
		await user.click(rowAt(0).getByRole("button", { name: "Logs web" }));
		// connect — opens the networks feature's dialog, not the inspect one
		await user.click(
			rowAt(0).getByRole("button", { name: "Connect to network web" }),
		);
		expect(
			await screen.findByText("Connect web to a network"),
		).toBeInTheDocument();
		await user.click(screen.getByRole("button", { name: "Close dialog" }));
		// delete — opens the confirm dialog, not the inspect one
		await user.click(rowAt(0).getByRole("button", { name: "Delete web" }));
		expect(
			await screen.findByTestId("confirm-delete-button"),
		).toBeInTheDocument();

		expect(inspectRequests).toBe(0);
		expect(
			screen.queryByTestId("inspect-dialog-loading"),
		).not.toBeInTheDocument();
		expect(screen.queryByTestId("inspect-dialog-json")).not.toBeInTheDocument();
		expect(
			screen.queryByTestId("inspect-dialog-error"),
		).not.toBeInTheDocument();
	});
});
