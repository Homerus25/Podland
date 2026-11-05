import { HostSettingsTab } from "@features/hosts/components/HostSettingsTab";
import { useHosts } from "@features/hosts/hooks/useHosts";
import type { Host } from "@shared/types/api";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { useLocation } from "react-router-dom";
import { beforeEach, describe, expect, it } from "vitest";
import { errorResponse } from "@/test/errors";
import { hostFixture } from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

let deleteRequested = false;
let hostsListRequests = 0;

function deleteHandler(status = 204, delayMs = 0) {
	return http.delete("*/api/hosts/m-1", async () => {
		if (delayMs) await new Promise((resolve) => setTimeout(resolve, delayMs));
		if (status !== 204) {
			return errorResponse(
				status,
				"DELETE_FAILED",
				"Failed to delete host",
				"req-5",
			);
		}
		deleteRequested = true;
		return new HttpResponse(null, { status: 204 });
	});
}

function hostsListHandler() {
	return http.get("*/api/hosts", () => {
		hostsListRequests++;
		return HttpResponse.json([hostFixture()]);
	});
}

// Delete success invalidates the hosts list; an active observer is needed
// so the invalidation actually refetches through MSW.
function HostsListProbe() {
	useHosts();
	return null;
}

function LocationProbe() {
	const location = useLocation();
	return <div data-testid="location">{location.pathname}</div>;
}

function renderSettingsTab(host: Host = hostFixture()) {
	renderWithQuery(
		<>
			<LocationProbe />
			<HostsListProbe />
			<HostSettingsTab host={host} />
		</>,
		{ router: { initialEntries: ["/hosts/m-1"] } },
	);
}

async function openDeleteModal() {
	const user = userEvent.setup();
	await user.click(screen.getByTestId("delete-host-button"));
	expect(await screen.findByTestId("modal")).toBeInTheDocument();
	return user;
}

describe("HostSettingsTab (delete host)", () => {
	beforeEach(() => {
		deleteRequested = false;
		hostsListRequests = 0;
	});

	it("deletes, refetches the hosts list, and navigates to /hosts on success", async () => {
		server.use(deleteHandler(), hostsListHandler());
		renderSettingsTab();
		const user = await openDeleteModal();
		const requestsBefore = hostsListRequests;

		await user.click(screen.getByTestId("confirm-delete-button"));

		await waitFor(() => expect(deleteRequested).toBe(true));
		expect(await screen.findByTestId("location")).toHaveTextContent("/hosts");
		// The delete mutation invalidated the hosts list query, whose
		// observer refetched through MSW.
		await waitFor(() =>
			expect(hostsListRequests).toBeGreaterThan(requestsBefore),
		);
	});

	it("renders the error envelope inside the modal, keeps it open, and navigates nowhere on failure", async () => {
		server.use(deleteHandler(409, 50), hostsListHandler());
		renderSettingsTab();
		const user = await openDeleteModal();

		await user.click(screen.getByTestId("confirm-delete-button"));

		const modal = screen.getByTestId("modal");
		expect(await within(modal).findByRole("alert")).toBeInTheDocument();
		expect(within(modal).getByText("DELETE_FAILED")).toBeInTheDocument();
		expect(
			within(modal).getByText("Failed to delete host"),
		).toBeInTheDocument();
		// Normalized failure: the modal stays open for another attempt and
		// the tab behind is untouched.
		expect(screen.getByTestId("modal")).toBeInTheDocument();
		expect(screen.getByTestId("location")).toHaveTextContent("/hosts/m-1");
	});
});
