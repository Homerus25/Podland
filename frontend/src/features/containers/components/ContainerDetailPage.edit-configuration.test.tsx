import { ContainerDetailPage } from "@features/containers/components/ContainerDetailPage";
import { useContainers } from "@features/containers/hooks/useContainers";
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
} from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

// The edit-configuration flow's request counters: the update POST itself,
// the detail GET (which must refetch so the page shows the new limits and
// healthcheck), and the containers list the update invalidates.
interface Counters {
	update: number;
	detail: number;
	containers: number;
}

// The sibling list the detail page does not render itself: mounting it
// alongside the page is how the invalidation is observed through MSW
// (feature tests assert refetch, never invalidation spies).
function SiblingLists({ machineId }: { machineId: string }) {
	useContainers(machineId);
	return null;
}

function setup(
	options: {
		permission?: "read" | "write" | "admin";
		updateResponder?: () => Response;
	} = {},
) {
	const counters: Counters = { update: 0, detail: 0, containers: 0 };
	const permission = options.permission ?? "admin";
	// The detail GET answers the updated container once the update
	// landed, so the page's own refetch is observable in the rendered
	// limits.
	let updated = false;
	let updateBody: unknown;

	server.use(
		http.get("*/api/machines/m-1", () =>
			HttpResponse.json(
				machineFixture({ permission, isOwner: permission === "admin" }),
			),
		),
		http.get("*/api/machines/m-1/containers/c-1", () => {
			counters.detail++;
			return HttpResponse.json(
				containerDetailFixture(
					updated ? { limits: { cpus: 1.5, memory: "1g" } } : {},
				),
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
		http.post(
			"*/api/machines/m-1/containers/c-1/update",
			async ({ request }) => {
				counters.update++;
				updateBody = await request.json();
				if (options.updateResponder) return options.updateResponder();
				updated = true;
				return HttpResponse.json(
					containerDetailFixture({ limits: { cpus: 1.5, memory: "1g" } }),
				);
			},
		),
	);

	return {
		counters,
		readUpdateBody: () => updateBody,
	};
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

describe("ContainerDetailPage edit configuration", () => {
	it("shows the Edit configuration button to a caller who can manage containers", async () => {
		setup({ permission: "write" });
		renderDetailPage();

		await screen.findByTestId("edit-configuration-button");
	});

	it("hides the Edit configuration button for a read-only caller", async () => {
		setup({ permission: "read" });
		renderDetailPage();

		await screen.findByTestId("container-detail-name");
		expect(
			screen.queryByTestId("edit-configuration-button"),
		).not.toBeInTheDocument();
	});

	it("renders no Edit configuration button while the machine query is pending", async () => {
		// The machine query is the gate's only input: while it is in flight
		// the caller's level is unknown, so the header must not flash the
		// button at a read-only caller.
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
			screen.queryByTestId("edit-configuration-button"),
		).not.toBeInTheDocument();

		release();
		await screen.findByTestId("edit-configuration-button");
	});

	it("opens the dialog prefilled with the container's current limits and healthcheck", async () => {
		setup();
		renderDetailPage();
		const user = userEvent.setup();

		await user.click(await screen.findByTestId("edit-configuration-button"));

		const dialog = screen.getByRole("dialog");
		expect(within(dialog).getByText("Edit configuration")).toBeInTheDocument();
		// The CPUs input is type=number, so its value reads back as a number.
		expect(screen.getByLabelText("CPUs")).toHaveValue(1.5);
		expect(screen.getByLabelText("Memory")).toHaveValue("512m");
		expect(screen.getByLabelText("Command")).toHaveValue(
			"curl -f http://localhost/health || exit 1",
		);
		expect(screen.getByLabelText("Interval")).toHaveValue("30s");
		expect(screen.getByLabelText("Timeout")).toHaveValue("5s");
		expect(screen.getByLabelText("Start period")).toHaveValue("0m30s");
		expect(screen.getByLabelText("Retries")).toHaveValue(3);
		expect(
			screen.getByRole("checkbox", { name: "Disable healthcheck" }),
		).not.toBeChecked();
	});

	it("closes the dialog on cancel without sending anything", async () => {
		const { counters } = setup();
		renderDetailPage();
		const user = userEvent.setup();

		await user.click(await screen.findByTestId("edit-configuration-button"));
		await user.click(screen.getByRole("button", { name: "Cancel" }));

		expect(screen.queryByLabelText("CPUs")).not.toBeInTheDocument();
		expect(counters.update).toBe(0);
	});

	// The resource-limit fields render through the shared module (spec
	// 437, ticket #431) with the container id prefix; the module's own
	// suite pins the labels, checks, and stacking once.
	it("renders the resource-limit fields through the shared module with the container id prefix", async () => {
		setup();
		renderDetailPage();
		const user = userEvent.setup();

		await user.click(await screen.findByTestId("edit-configuration-button"));

		expect(screen.getByLabelText("CPUs")).toHaveAttribute(
			"id",
			"container-cpus",
		);
		expect(screen.getByLabelText("Memory")).toHaveAttribute(
			"id",
			"container-memory",
		);
	});

	it("blocks the submit while a value is unparseable and explains why", async () => {
		setup();
		renderDetailPage();
		const user = userEvent.setup();

		await user.click(await screen.findByTestId("edit-configuration-button"));
		expect(screen.getByTestId("edit-configuration-submit")).toBeEnabled();

		const memory = screen.getByLabelText("Memory");
		await user.type(memory, "abc");
		expect(
			screen.getByText("Use podman units syntax, e.g. 512m or 1g"),
		).toBeInTheDocument();
		expect(screen.getByTestId("edit-configuration-submit")).toBeDisabled();

		await user.clear(memory);
		expect(screen.getByTestId("edit-configuration-submit")).toBeEnabled();
		const cpus = screen.getByLabelText("CPUs");
		await user.clear(cpus);
		await user.type(cpus, "0");
		expect(
			screen.getByText("CPU limit must be a positive number"),
		).toBeInTheDocument();
		expect(screen.getByTestId("edit-configuration-submit")).toBeDisabled();

		await user.clear(cpus);
		expect(screen.getByTestId("edit-configuration-submit")).toBeEnabled();
		const interval = screen.getByLabelText("Interval");
		await user.type(interval, "abc");
		expect(
			screen.getByText("Use a Go duration, e.g. 30s or 1m30s"),
		).toBeInTheDocument();
		expect(screen.getByTestId("edit-configuration-submit")).toBeDisabled();

		await user.clear(interval);
		const retries = screen.getByLabelText("Retries");
		await user.clear(retries);
		await user.type(retries, "1.5");
		expect(
			screen.getByText("Retries must be a non-negative integer"),
		).toBeInTheDocument();
		expect(screen.getByTestId("edit-configuration-submit")).toBeDisabled();
	});

	it("sends only the changed fields", async () => {
		const { readUpdateBody } = setup();
		renderDetailPage();
		const user = userEvent.setup();

		await user.click(await screen.findByTestId("edit-configuration-button"));
		await user.clear(screen.getByLabelText("CPUs"));
		await user.type(screen.getByLabelText("CPUs"), "2");
		await user.clear(screen.getByLabelText("Memory"));
		await user.type(screen.getByLabelText("Memory"), "1g");
		await user.clear(screen.getByLabelText("Command"));
		await user.type(
			screen.getByLabelText("Command"),
			"wget -q http://localhost/",
		);

		await user.click(screen.getByTestId("edit-configuration-submit"));

		await waitFor(() => expect(readUpdateBody()).toBeDefined());
		expect(readUpdateBody()).toEqual({
			cpus: 2,
			memory: "1g",
			healthcheck: { cmd: "wget -q http://localhost/" },
		});
	});

	it("sends an empty payload when nothing changed", async () => {
		const { readUpdateBody } = setup();
		renderDetailPage();
		const user = userEvent.setup();

		await user.click(await screen.findByTestId("edit-configuration-button"));
		await user.click(screen.getByTestId("edit-configuration-submit"));

		await waitFor(() => expect(readUpdateBody()).toBeDefined());
		expect(readUpdateBody()).toEqual({});
	});

	it("sends only the disable flag when the toggle is set", async () => {
		const { readUpdateBody } = setup();
		renderDetailPage();
		const user = userEvent.setup();

		await user.click(await screen.findByTestId("edit-configuration-button"));
		await user.click(
			screen.getByRole("checkbox", { name: "Disable healthcheck" }),
		);

		// The toggle is exclusive: the other healthcheck inputs grey out,
		// so no other healthcheck field can ride the request.
		expect(screen.getByLabelText("Command")).toBeDisabled();
		expect(screen.getByLabelText("Interval")).toBeDisabled();

		await user.click(screen.getByTestId("edit-configuration-submit"));

		await waitFor(() => expect(readUpdateBody()).toBeDefined());
		expect(readUpdateBody()).toEqual({ healthcheck: { disabled: true } });
	});

	it("updates, closes the dialog, and refetches the detail and containers queries", async () => {
		const { counters } = setup();
		renderDetailPage();
		const user = userEvent.setup();

		await waitFor(() => expect(counters.containers).toBe(1));
		await waitFor(() => expect(counters.detail).toBe(1));

		await user.click(await screen.findByTestId("edit-configuration-button"));
		await user.clear(screen.getByLabelText("Memory"));
		await user.type(screen.getByLabelText("Memory"), "1g");
		await user.click(screen.getByTestId("edit-configuration-submit"));

		// The Memory row shows the new limit, which is the detail query's
		// refetch landing.
		await waitFor(() =>
			expect(screen.getByTestId("container-detail-memory")).toHaveTextContent(
				"1g",
			),
		);
		expect(counters.update).toBe(1);
		expect(counters.detail).toBe(2);
		expect(counters.containers).toBe(2);

		// Success closes the dialog.
		await waitFor(() =>
			expect(screen.queryByLabelText("CPUs")).not.toBeInTheDocument(),
		);
	});

	it("shows a server error in the dialog and keeps it open", async () => {
		setup({
			updateResponder: () =>
				errorResponse(
					400,
					"VALIDATION_FAILED",
					"Memory value 'abc' is not a podman unit",
				),
		});
		renderDetailPage();
		const user = userEvent.setup();

		await user.click(await screen.findByTestId("edit-configuration-button"));
		await user.clear(screen.getByLabelText("Memory"));
		await user.type(screen.getByLabelText("Memory"), "1g");
		await user.click(screen.getByTestId("edit-configuration-submit"));

		const errorRegion = await screen.findByTestId("mutation-dialog-error");
		expect(errorRegion).toHaveTextContent("VALIDATION_FAILED");
		expect(errorRegion).toHaveTextContent(
			"Memory value 'abc' is not a podman unit",
		);
		// The dialog stays open on failure.
		expect(screen.getByLabelText("CPUs")).toBeInTheDocument();
	});

	it("shows the container's current limits and healthcheck in the overview", async () => {
		setup();
		renderDetailPage();

		await screen.findByTestId("container-detail-name");
		expect(screen.getByTestId("container-detail-cpus")).toHaveTextContent(
			"1.5",
		);
		expect(screen.getByTestId("container-detail-memory")).toHaveTextContent(
			"512m",
		);
		const healthcheck = screen.getByTestId("container-detail-healthcheck");
		expect(
			within(healthcheck).getByText(
				"curl -f http://localhost/health || exit 1",
			),
		).toBeInTheDocument();
	});
});
