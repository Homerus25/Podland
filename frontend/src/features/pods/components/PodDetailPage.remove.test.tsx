import { PodDetailPage } from "@features/pods/components/PodDetailPage";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http, type JsonBodyType } from "msw";
import { Route, Routes, useSearchParams } from "react-router-dom";
import { beforeEach, describe, expect, it } from "vitest";
import { errorJson, errorResponse } from "@/test/errors";
import { hostFixture, podDetailFixture } from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

// The pod detail page's Remove action (spec 461, ticket #465): the
// header-actions slot, the capability-gate wiring, the force-refusal
// confirm, and the navigate-away on success. The gate's classification is
// pinned once in the hook's suite and the dialog skeleton once in the
// shared MutationDialog suite; this suite keeps domain assertions only.

let removeRequests: {
	count: number;
	pathname: string;
	force: string | null;
} = { count: 0, pathname: "", force: null };

function resetCounters() {
	removeRequests = { count: 0, pathname: "", force: null };
}

// The page reads the host's per-caller facts for the manage gates.
function mockHost(permission: "read" | "write" | "admin" = "admin") {
	server.use(
		http.get("*/api/hosts/m-1", () =>
			HttpResponse.json(
				hostFixture({ permission, isOwner: permission === "admin" }),
			),
		),
	);
}

function mockPodDetail() {
	server.use(
		http.get("*/api/hosts/m-1/pods/p-1", () =>
			HttpResponse.json(podDetailFixture()),
		),
	);
}

function mockRemove(
	responder: () => { status: number; body?: JsonBodyType } = () => ({
		status: 204,
	}),
) {
	server.use(
		http.delete("*/api/hosts/m-1/pods/*", ({ request }) => {
			removeRequests.count += 1;
			const url = new URL(request.url);
			removeRequests.pathname = url.pathname;
			removeRequests.force = url.searchParams.get("force");
			const response = responder();
			if (response.status !== 204) {
				return HttpResponse.json(response.body, { status: response.status });
			}
			return new HttpResponse(null, { status: 204 });
		}),
	);
}

// The navigation target: a stub standing in for the host page, rendering
// the tab search parameter so the test pins the exact URL navigated to.
function HostTabStub() {
	const [params] = useSearchParams();
	return <div data-testid="host-tab-page">{params.get("tab")}</div>;
}

function renderDetailPage() {
	return renderWithQuery(
		<Routes>
			<Route path="/hosts/:hostId/pods/:podId" element={<PodDetailPage />} />
			<Route path="/hosts/:hostId" element={<HostTabStub />} />
		</Routes>,
		{ router: { initialEntries: ["/hosts/m-1/pods/p-1"] } },
	);
}

describe("PodDetailPage remove", () => {
	beforeEach(resetCounters);

	it("renders the Remove action for a write-level caller", async () => {
		mockHost("write");
		mockPodDetail();
		renderDetailPage();

		expect(await screen.findByTestId("remove-pod-button")).toHaveTextContent(
			"Remove",
		);
		expect(
			screen.queryByTestId("pods-permission-hint"),
		).not.toBeInTheDocument();
	});

	it("renders the shared write hint instead of the Remove action for a read-level caller", async () => {
		mockHost("read");
		mockPodDetail();
		renderDetailPage();

		const hint = await screen.findByTestId("pods-permission-hint");
		expect(hint).toHaveAttribute("data-requirement", "write");
		expect(hint).toHaveTextContent(
			"You have read-only access to this host. Ask an owner or admin for write access to manage its pods.",
		);
		expect(screen.queryByTestId("remove-pod-button")).not.toBeInTheDocument();
	});

	it("renders neither the Remove action nor the hint while the gate is pending", async () => {
		mockPodDetail();
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

		await screen.findByTestId("pod-detail-name");
		expect(screen.queryByTestId("remove-pod-button")).not.toBeInTheDocument();
		expect(
			screen.queryByTestId("pods-permission-hint"),
		).not.toBeInTheDocument();

		release();
		await screen.findByTestId("remove-pod-button");
	});

	it("hides the Remove action when the pod no longer exists", async () => {
		mockHost();
		server.use(
			http.get("*/api/hosts/m-1/pods/p-1", () =>
				errorResponse(404, "POD_NOT_FOUND", "pod not found", "r-1"),
			),
		);
		renderDetailPage();

		expect(
			await screen.findByTestId("detail-page-not-found"),
		).toBeInTheDocument();
		expect(screen.queryByTestId("remove-pod-button")).not.toBeInTheDocument();
		expect(
			screen.queryByTestId("pods-permission-hint"),
		).not.toBeInTheDocument();
	});

	it("opens the confirm naming the pod and sends nothing before confirming", async () => {
		mockHost();
		mockPodDetail();
		mockRemove();
		renderDetailPage();
		const user = userEvent.setup();

		await user.click(await screen.findByTestId("remove-pod-button"));

		expect(await screen.findByTestId("modal")).toBeInTheDocument();
		expect(screen.getByText(/Remove pod “app-pod”/)).toBeInTheDocument();
		expect(removeRequests.count).toBe(0);
	});

	it("sends the plain remove on confirm and lands on the pods tab", async () => {
		mockHost();
		mockPodDetail();
		mockRemove();
		renderDetailPage();
		const user = userEvent.setup();

		await user.click(await screen.findByTestId("remove-pod-button"));
		await user.click(await screen.findByTestId("confirm-delete-button"));

		await waitFor(() => expect(removeRequests.count).toBe(1));
		expect(removeRequests.pathname).toBe("/api/hosts/m-1/pods/p-1");
		expect(removeRequests.force).toBeNull();
		expect(await screen.findByTestId("host-tab-page")).toHaveTextContent(
			"pods",
		);
	});

	it("swaps to the running hint on a refusal and force-removes on the retry", async () => {
		mockHost();
		mockPodDetail();
		let refused = true;
		mockRemove(() => {
			if (!refused) return { status: 204 };
			refused = false;
			return {
				status: 409,
				body: errorJson(
					"POD_STATE_CONFLICT",
					"pod has running containers",
					"req-9",
				),
			};
		});
		renderDetailPage();
		const user = userEvent.setup();

		await user.click(await screen.findByTestId("remove-pod-button"));
		await user.click(await screen.findByTestId("confirm-delete-button"));

		expect(
			await screen.findByTestId("pod-running-warning"),
		).toBeInTheDocument();
		expect(screen.getByText(/running containers/i)).toBeInTheDocument();
		expect(
			screen.queryByTestId("confirm-delete-button"),
		).not.toBeInTheDocument();

		await user.click(screen.getByTestId("force-remove-button"));

		await waitFor(() => expect(removeRequests.count).toBe(2));
		expect(removeRequests.force).toBe("true");
		expect(await screen.findByTestId("host-tab-page")).toHaveTextContent(
			"pods",
		);
	});
});
