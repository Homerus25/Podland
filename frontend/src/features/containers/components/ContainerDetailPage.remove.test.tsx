import { ContainerDetailPage } from "@features/containers/components/ContainerDetailPage";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { Route, Routes, useLocation, useParams } from "react-router-dom";
import { beforeEach, describe, expect, it } from "vitest";
import { errorResponse } from "@/test/errors";
import { containerDetailFixture, hostFixture } from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

// The delete request the page sends. The force flag rides the query string,
// so the plain-first / force-on-retry contract is asserted on the wire.
interface DeleteRequests {
	count: number;
	method?: string;
	pathname: string;
	search: string;
}

let deleteRequests: DeleteRequests;

function resetDeleteRequests() {
	deleteRequests = { count: 0, method: undefined, pathname: "", search: "" };
}

// The host tab the page navigates to on success: the real host page reads
// the ?tab= search parameter, so the probe observes it the same way.
function HostTabProbe() {
	const { hostId } = useParams<{ hostId: string }>();
	const { search } = useLocation();
	return (
		<div>
			<span data-testid="probe-host">{hostId}</span>
			<span data-testid="probe-search">{search}</span>
		</div>
	);
}

function installHandlers(
	options: {
		permission?: "read" | "write" | "admin";
		deleteResponder?: () => Response;
	} = {},
) {
	const permission = options.permission ?? "write";
	server.use(
		http.get("*/api/hosts/m-1", () =>
			HttpResponse.json(
				hostFixture({ permission, isOwner: permission === "admin" }),
			),
		),
		http.get("*/api/hosts/m-1/containers/c-1", () =>
			HttpResponse.json(containerDetailFixture()),
		),
		http.delete("*/api/hosts/m-1/containers/c-1", ({ request }) => {
			deleteRequests.count += 1;
			deleteRequests.method = request.method;
			const url = new URL(request.url);
			deleteRequests.pathname = url.pathname;
			deleteRequests.search = url.search;
			if (options.deleteResponder) return options.deleteResponder();
			return new HttpResponse(null, { status: 204 });
		}),
	);
}

function renderDetailPage() {
	return renderWithQuery(
		<Routes>
			<Route
				path="/hosts/:hostId/containers/:containerId"
				element={<ContainerDetailPage />}
			/>
			<Route path="/hosts/:hostId" element={<HostTabProbe />} />
		</Routes>,
		{ router: { initialEntries: ["/hosts/m-1/containers/c-1"] } },
	);
}

describe("ContainerDetailPage remove", () => {
	beforeEach(resetDeleteRequests);

	it("shows the Remove button to a caller who can manage containers", async () => {
		installHandlers({ permission: "write" });
		renderDetailPage();

		expect(
			await screen.findByTestId("remove-container-button"),
		).toHaveTextContent("Remove");
	});

	it("renders the write-permission hint instead of Remove for a read-only caller", async () => {
		installHandlers({ permission: "read" });
		renderDetailPage();

		const hint = await screen.findByTestId("containers-permission-hint");
		expect(hint).toHaveAttribute("data-requirement", "write");
		expect(
			screen.queryByTestId("remove-container-button"),
		).not.toBeInTheDocument();
	});

	it("renders neither the Remove button nor the hint while the host query is pending", async () => {
		// The host query is the gate's only input: while it is in flight the
		// caller's level is unknown, so the header must not flash the
		// read-only hint at a write-capable caller.
		installHandlers();
		let release: () => void = () => {};
		const gate = new Promise<void>((resolve) => {
			release = resolve;
		});
		server.use(
			http.get("*/api/hosts/m-1", async () => {
				await gate;
				return HttpResponse.json(hostFixture({ permission: "write" }));
			}),
		);
		renderDetailPage();

		await screen.findByTestId("container-detail-name");
		expect(
			screen.queryByTestId("remove-container-button"),
		).not.toBeInTheDocument();
		expect(
			screen.queryByTestId("containers-permission-hint"),
		).not.toBeInTheDocument();

		release();
		await screen.findByTestId("remove-container-button");
	});

	it("hides the Remove button when the container no longer exists", async () => {
		// The header actions render outside the layout's state ternary, so
		// the page hides the action itself on a 404 overview.
		installHandlers();
		server.use(
			http.get("*/api/hosts/m-1/containers/c-1", () =>
				errorResponse(
					404,
					"CONTAINER_NOT_FOUND",
					"Container not found",
					"req-71",
				),
			),
		);
		renderDetailPage();

		await screen.findByTestId("detail-page-not-found");
		expect(
			screen.queryByTestId("remove-container-button"),
		).not.toBeInTheDocument();
	});

	it("opens a confirmation naming the container without sending a request", async () => {
		installHandlers();
		renderDetailPage();
		const user = userEvent.setup();

		await user.click(await screen.findByTestId("remove-container-button"));

		const modal = await screen.findByTestId("modal");
		expect(modal).toHaveTextContent("web");
		expect(deleteRequests.count).toBe(0);
	});

	it("sends the plain delete first, without the force flag", async () => {
		installHandlers();
		renderDetailPage();
		const user = userEvent.setup();

		await user.click(await screen.findByTestId("remove-container-button"));
		await user.click(screen.getByTestId("confirm-delete-button"));

		await waitFor(() => expect(deleteRequests.count).toBe(1));
		expect(deleteRequests.method).toBe("DELETE");
		expect(deleteRequests.pathname).toBe("/api/hosts/m-1/containers/c-1");
		expect(deleteRequests.search).toBe("");
	});

	it("explains a running-container refusal and force-removes on the retry", async () => {
		installHandlers({
			deleteResponder: () =>
				errorResponse(
					409,
					"CONTAINER_RUNNING",
					"container web is running and cannot be removed without force",
					"req-72",
				),
		});
		renderDetailPage();
		const user = userEvent.setup();

		await user.click(await screen.findByTestId("remove-container-button"));
		await user.click(screen.getByTestId("confirm-delete-button"));

		expect(
			await screen.findByTestId("container-running-warning"),
		).toBeInTheDocument();
		// The refusal swaps the footer to the force button.
		expect(
			screen.queryByTestId("confirm-delete-button"),
		).not.toBeInTheDocument();

		await user.click(screen.getByTestId("force-remove-button"));

		await waitFor(() => expect(deleteRequests.count).toBe(2));
		expect(deleteRequests.search).toBe("?force=true");
	});

	it("navigates to the containers tab on success", async () => {
		installHandlers();
		renderDetailPage();
		const user = userEvent.setup();

		await user.click(await screen.findByTestId("remove-container-button"));
		await user.click(screen.getByTestId("confirm-delete-button"));

		expect(await screen.findByTestId("probe-host")).toHaveTextContent("m-1");
		expect(screen.getByTestId("probe-search")).toHaveTextContent(
			"?tab=containers",
		);
	});
});
