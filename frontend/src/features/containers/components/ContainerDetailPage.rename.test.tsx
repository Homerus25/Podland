import { ContainerDetailPage } from "@features/containers/components/ContainerDetailPage";
import { useContainers } from "@features/containers/hooks/useContainers";
import { usePods } from "@features/pods/hooks/usePods";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { Route, Routes } from "react-router-dom";
import { describe, expect, it } from "vitest";
import { errorResponse } from "@/test/errors";
import {
	containerDetailFixture,
	containerFixture,
	machineFixture,
	podFixture,
} from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

// The rename flow's request counters: the rename POST itself, the detail
// GET (which must refetch so the page shows the new name), and the two
// sibling lists the rename invalidates — the containers list and the pods
// list, since container names surface in pod data.
interface Counters {
	rename: number;
	detail: number;
	containers: number;
	pods: number;
}

// The sibling lists the detail page does not render itself: mounting them
// alongside the page is how the invalidation is observed through MSW
// (feature tests assert refetch, never invalidation spies).
function SiblingLists({ machineId }: { machineId: string }) {
	useContainers(machineId);
	usePods(machineId);
	return null;
}

function setup(
	options: {
		permission?: "read" | "write" | "admin";
		renameResponder?: () => Response;
	} = {},
) {
	const counters: Counters = { rename: 0, detail: 0, containers: 0, pods: 0 };
	const permission = options.permission ?? "admin";
	// The detail GET answers the renamed container once the rename landed,
	// so the page's own refetch is observable in the rendered name.
	let renamed = false;

	server.use(
		http.get("*/api/machines/m-1", () =>
			HttpResponse.json(
				machineFixture({ permission, isOwner: permission === "admin" }),
			),
		),
		http.get("*/api/machines/m-1/containers/c-1", () => {
			counters.detail++;
			return HttpResponse.json(
				containerDetailFixture(renamed ? { name: "renamed" } : {}),
			);
		}),
		http.get("*/api/machines/m-1/containers", () => {
			counters.containers++;
			return HttpResponse.json({
				containers: [containerFixture()],
				total: 1,
				timestamp: "2026-08-20T10:00:00Z",
			});
		}),
		http.get("*/api/machines/m-1/pods", () => {
			counters.pods++;
			return HttpResponse.json({
				pods: [podFixture()],
				total: 1,
				timestamp: "2026-08-20T10:00:00Z",
			});
		}),
		http.post("*/api/machines/m-1/containers/c-1/rename", () => {
			counters.rename++;
			if (options.renameResponder) return options.renameResponder();
			renamed = true;
			return HttpResponse.json(containerDetailFixture({ name: "renamed" }));
		}),
	);

	return counters;
}

function renderDetailPage() {
	return renderWithQuery(
		<Routes>
			<Route
				path="/machines/:machineId/containers/:containerId"
				element={
					<>
						<ContainerDetailPage />
						<SiblingLists machineId="m-1" />
					</>
				}
			/>
		</Routes>,
		{ router: { initialEntries: ["/machines/m-1/containers/c-1"] } },
	);
}

describe("ContainerDetailPage rename", () => {
	it("shows the pencil rename button to a caller who can manage containers", async () => {
		setup({ permission: "write" });
		renderDetailPage();

		const button = await screen.findByTestId("rename-container-button");
		expect(button).toHaveAttribute("aria-label", "Rename web");
		expect(
			screen.queryByTestId("rename-permission-hint"),
		).not.toBeInTheDocument();
	});

	it("hides the button and shows the write-permission hint in the Name row for a read-only caller", async () => {
		setup({ permission: "read" });
		renderDetailPage();

		await screen.findByTestId("container-detail-name");
		await waitFor(() =>
			expect(screen.getByTestId("rename-permission-hint")).toBeInTheDocument(),
		);
		expect(
			screen.queryByTestId("rename-container-button"),
		).not.toBeInTheDocument();

		const hint = screen.getByTestId("rename-permission-hint");
		expect(hint).toHaveAttribute("data-requirement", "write");
		expect(hint).toHaveTextContent(
			"You have read-only access to this machine. Ask an owner or admin for write access to manage its containers.",
		);
		// The hint sits in the Name row, beside the name it guards.
		const nameRow = screen.getByTestId("container-detail-name").closest("tr");
		expect(within(nameRow!).getByTestId("rename-permission-hint")).toBe(hint);
	});

	it("opens the dialog prefilled with the current name and keeps Rename disabled while unchanged", async () => {
		setup();
		renderDetailPage();
		const user = userEvent.setup();

		await user.click(await screen.findByTestId("rename-container-button"));

		expect(screen.getByText("Rename container")).toBeInTheDocument();
		const input = await screen.findByLabelText(/^name/i);
		expect(input).toHaveValue("web");
		expect(screen.getByTestId("rename-container-submit")).toBeDisabled();
	});

	it("renders neither the button nor the hint while the machine query is pending", async () => {
		// The machine query is the gate's only input: while it is in flight
		// the caller's level is unknown, so the Name row must not flash the
		// read-only hint at a write-capable caller.
		setup();
		let release: () => void = () => {};
		const gate = new Promise<void>((resolve) => {
			release = resolve;
		});
		server.use(
			http.get("*/api/machines/m-1", async () => {
				await gate;
				return HttpResponse.json(machineFixture({ permission: "write" }));
			}),
		);
		renderDetailPage();

		await screen.findByTestId("container-detail-name");
		expect(
			screen.queryByTestId("rename-container-button"),
		).not.toBeInTheDocument();
		expect(
			screen.queryByTestId("rename-permission-hint"),
		).not.toBeInTheDocument();

		release();
		await screen.findByTestId("rename-container-button");
	});

	it("keeps Rename disabled while the input is empty", async () => {
		setup();
		renderDetailPage();
		const user = userEvent.setup();

		await user.click(await screen.findByTestId("rename-container-button"));
		const input = await screen.findByLabelText(/^name/i);
		await user.clear(input);

		expect(screen.getByTestId("rename-container-submit")).toBeDisabled();
	});

	it("renames, closes the dialog, and refetches the detail, containers, and pods queries", async () => {
		const counters = setup();
		renderDetailPage();
		const user = userEvent.setup();

		await waitFor(() => expect(counters.containers).toBe(1));
		await waitFor(() => expect(counters.pods).toBe(1));
		await waitFor(() => expect(counters.detail).toBe(1));

		await user.click(await screen.findByTestId("rename-container-button"));
		const input = await screen.findByLabelText(/^name/i);
		await user.clear(input);
		await user.type(input, "renamed");
		await user.click(screen.getByTestId("rename-container-submit"));

		// The page title and the Name row show the new name, which is the
		// detail query's refetch landing.
		await waitFor(() =>
			expect(screen.getByTestId("container-detail-name")).toHaveTextContent(
				"renamed",
			),
		);
		expect(screen.getByTestId("detail-page-title")).toHaveTextContent(
			"renamed",
		);
		expect(counters.rename).toBe(1);
		expect(counters.detail).toBe(2);
		expect(counters.containers).toBe(2);
		expect(counters.pods).toBe(2);

		// Success closes the dialog.
		await waitFor(() =>
			expect(screen.queryByLabelText(/^name/i)).not.toBeInTheDocument(),
		);
	});

	it("shows a server error in the dialog and keeps it open", async () => {
		setup({
			renameResponder: () =>
				errorResponse(
					409,
					"CONTAINER_ALREADY_EXISTS",
					"Container with this name already exists",
				),
		});
		renderDetailPage();
		const user = userEvent.setup();

		await user.click(await screen.findByTestId("rename-container-button"));
		const input = await screen.findByLabelText(/^name/i);
		await user.clear(input);
		await user.type(input, "taken");
		await user.click(screen.getByTestId("rename-container-submit"));

		const errorRegion = await screen.findByTestId("mutation-dialog-error");
		expect(errorRegion).toHaveTextContent("CONTAINER_ALREADY_EXISTS");
		expect(errorRegion).toHaveTextContent(
			"Container with this name already exists",
		);
		// The dialog stays open on failure.
		expect(screen.getByLabelText(/^name/i)).toBeInTheDocument();
	});
});
