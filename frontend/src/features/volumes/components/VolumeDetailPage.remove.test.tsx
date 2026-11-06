import { VolumeDetailPage } from "@features/volumes/components/VolumeDetailPage";
import { useVolumes } from "@features/volumes/hooks/useVolumes";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http, type JsonBodyType } from "msw";
import type { ReactElement } from "react";
import { Route, Routes, useSearchParams } from "react-router-dom";
import { beforeEach, describe, expect, it } from "vitest";
import { errorJson, errorResponse } from "@/test/errors";
import { hostFixture, volumeFixture } from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

// The volume detail page's Remove action (spec 461, ticket #465): the
// header-actions slot, the capability-gate wiring, the force-refusal
// confirm, and the navigate-away on success. The gate's classification is
// pinned once in the hook's suite and the dialog skeleton once in the
// shared MutationDialog suite; this suite keeps domain assertions only.

const volume = volumeFixture({ name: "app-data" });

let deleteRequests: {
	count: number;
	pathname: string;
	force: string | null;
} = { count: 0, pathname: "", force: null };

function resetCounters() {
	deleteRequests = { count: 0, pathname: "", force: null };
}

// The page reads the host's per-caller facts for the manage gate.
function mockHost(permission: "read" | "write" | "admin" = "admin") {
	server.use(
		http.get("*/api/hosts/m-1", () =>
			HttpResponse.json(
				hostFixture({ permission, isOwner: permission === "admin" }),
			),
		),
	);
}

function mockVolumeDetail() {
	server.use(
		http.get("*/api/hosts/m-1/volumes/app-data", () =>
			HttpResponse.json(volume),
		),
	);
}

function mockDelete(
	responder: () => { status: number; body?: JsonBodyType } = () => ({
		status: 204,
	}),
) {
	server.use(
		http.delete("*/api/hosts/m-1/volumes/*", ({ request }) => {
			deleteRequests.count += 1;
			const url = new URL(request.url);
			deleteRequests.pathname = url.pathname;
			deleteRequests.force = url.searchParams.get("force");
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

function renderDetailPage(ui: ReactElement = <VolumeDetailPage />) {
	return renderWithQuery(
		<Routes>
			<Route path="/hosts/:hostId/volumes/:volumeName" element={ui} />
			<Route path="/hosts/:hostId" element={<HostTabStub />} />
		</Routes>,
		{ router: { initialEntries: ["/hosts/m-1/volumes/app-data"] } },
	);
}

describe("VolumeDetailPage remove", () => {
	beforeEach(resetCounters);

	it("renders the Remove action for a write-level caller", async () => {
		mockHost("write");
		mockVolumeDetail();
		renderDetailPage();

		expect(await screen.findByTestId("remove-volume-button")).toHaveTextContent(
			"Remove",
		);
		expect(
			screen.queryByTestId("volumes-permission-hint"),
		).not.toBeInTheDocument();
	});

	it("renders the shared write hint instead of the Remove action for a read-level caller", async () => {
		mockHost("read");
		mockVolumeDetail();
		renderDetailPage();

		const hint = await screen.findByTestId("volumes-permission-hint");
		expect(hint).toHaveAttribute("data-requirement", "write");
		expect(hint).toHaveTextContent(
			"You have read-only access to this host. Ask an owner or admin for write access to manage its volumes.",
		);
		expect(
			screen.queryByTestId("remove-volume-button"),
		).not.toBeInTheDocument();
	});

	it("renders neither the Remove action nor the hint while the gate is pending", async () => {
		mockVolumeDetail();
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

		await screen.findByTestId("volume-detail-overview");
		expect(
			screen.queryByTestId("remove-volume-button"),
		).not.toBeInTheDocument();
		expect(
			screen.queryByTestId("volumes-permission-hint"),
		).not.toBeInTheDocument();

		release();
		await screen.findByTestId("remove-volume-button");
	});

	it("hides the Remove action when the volume no longer exists", async () => {
		mockHost();
		server.use(
			http.get("*/api/hosts/m-1/volumes/app-data", () =>
				errorResponse(404, "VOLUME_NOT_FOUND", "Volume not found", "r-1"),
			),
			// The page's stale-row effect refetches the list on a 404.
			http.get("*/api/hosts/m-1/volumes", () => HttpResponse.json([])),
		);
		renderDetailPage();

		expect(
			await screen.findByTestId("detail-page-not-found"),
		).toBeInTheDocument();
		expect(
			screen.queryByTestId("remove-volume-button"),
		).not.toBeInTheDocument();
		expect(
			screen.queryByTestId("volumes-permission-hint"),
		).not.toBeInTheDocument();
	});

	it("opens the confirm naming the volume and sends nothing before confirming", async () => {
		mockHost();
		mockVolumeDetail();
		mockDelete();
		renderDetailPage();
		const user = userEvent.setup();

		await user.click(await screen.findByTestId("remove-volume-button"));

		expect(await screen.findByTestId("modal")).toBeInTheDocument();
		expect(screen.getByText(/Remove volume app-data/)).toBeInTheDocument();
		expect(deleteRequests.count).toBe(0);
	});

	it("sends the plain delete on confirm and lands on the volumes tab", async () => {
		mockHost();
		mockVolumeDetail();
		mockDelete();
		renderDetailPage();
		const user = userEvent.setup();

		await user.click(await screen.findByTestId("remove-volume-button"));
		await user.click(await screen.findByTestId("confirm-delete-button"));

		await waitFor(() => expect(deleteRequests.count).toBe(1));
		expect(deleteRequests.pathname).toBe("/api/hosts/m-1/volumes/app-data");
		expect(deleteRequests.force).toBeNull();
		expect(await screen.findByTestId("host-tab-page")).toHaveTextContent(
			"volumes",
		);
	});

	it("refetches the volumes list through the mutation's own invalidation", async () => {
		mockHost();
		mockVolumeDetail();
		let listRequests = 0;
		let listed = [volume];
		server.use(
			http.get("*/api/hosts/m-1/volumes", () => {
				listRequests += 1;
				return HttpResponse.json(listed);
			}),
		);
		mockDelete(() => {
			listed = [];
			return { status: 204 };
		});
		// A live list observer makes the invalidation observable through the
		// public interface: the removed volume leaves the list via a real
		// refetch, not an optimistic edit.
		function VolumeListProbe() {
			useVolumes("m-1");
			return null;
		}
		renderDetailPage(
			<>
				<VolumeListProbe />
				<VolumeDetailPage />
			</>,
		);
		const user = userEvent.setup();

		await user.click(await screen.findByTestId("remove-volume-button"));
		await user.click(await screen.findByTestId("confirm-delete-button"));

		await waitFor(() => expect(listRequests).toBe(2));
	});

	it("swaps to the in-use hint on a refusal and force-deletes on the retry", async () => {
		mockHost();
		mockVolumeDetail();
		let refused = true;
		mockDelete(() => {
			if (!refused) return { status: 204 };
			refused = false;
			return {
				status: 409,
				body: errorJson(
					"VOLUME_IN_USE",
					"volume is in use by container abc",
					"req-42",
				),
			};
		});
		renderDetailPage();
		const user = userEvent.setup();

		await user.click(await screen.findByTestId("remove-volume-button"));
		await user.click(await screen.findByTestId("confirm-delete-button"));

		expect(
			await screen.findByTestId("volume-in-use-warning"),
		).toBeInTheDocument();
		expect(screen.getByText(/containers use this volume/i)).toBeInTheDocument();
		expect(
			screen.queryByTestId("confirm-delete-button"),
		).not.toBeInTheDocument();

		await user.click(screen.getByTestId("force-remove-button"));

		await waitFor(() => expect(deleteRequests.count).toBe(2));
		expect(deleteRequests.force).toBe("true");
		expect(await screen.findByTestId("host-tab-page")).toHaveTextContent(
			"volumes",
		);
	});
});
