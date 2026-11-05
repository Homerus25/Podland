import { ContainerList } from "@features/containers/components/ContainerList";
import type { ContainerSummary } from "@features/containers/services/containersApi";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import { errorResponse } from "@/test/errors";
import { containerFixture, hostFixture } from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

const first = containerFixture({ id: "c-1", names: ["/victim"] });
const second = containerFixture({
	id: "c-2",
	names: ["/survivor"],
	state: "exited",
});

let deleteRequests: {
	method: string;
	pathname: string;
	search: string;
	count: number;
} = {
	method: "",
	pathname: "",
	search: "",
	count: 0,
};

function resetCounters() {
	deleteRequests = { method: "", pathname: "", search: "", count: 0 };
}

function containersHandler(containers: () => ContainerSummary[]) {
	return http.get("*/api/hosts/m-1/containers", () =>
		HttpResponse.json({
			containers: containers(),
			total: containers().length,
			timestamp: "2026-08-26T00:00:00Z",
		}),
	);
}

function deleteHandler(onSuccess?: () => void) {
	return http.delete("*/api/hosts/m-1/containers/*", ({ request }) => {
		deleteRequests.count++;
		deleteRequests.method = request.method;
		const url = new URL(request.url);
		deleteRequests.pathname = url.pathname;
		deleteRequests.search = url.search;
		onSuccess?.();
		return new HttpResponse(null, { status: 204 });
	});
}

function renderContainerList() {
	renderWithQuery(<ContainerList host={hostFixture()} />, {
		router: true,
	});
}

async function openDeleteModal() {
	renderContainerList();
	await screen.findByText("victim");
	const user = userEvent.setup();
	await user.click(screen.getByRole("button", { name: /delete victim/i }));
	return user;
}

describe("ContainerList delete", () => {
	it("opens the confirmation modal and sends no delete request before confirming", async () => {
		resetCounters();
		server.use(
			containersHandler(() => [first]),
			deleteHandler(),
		);
		await openDeleteModal();

		expect(screen.getByTestId("modal")).toBeInTheDocument();
		expect(deleteRequests.count).toBe(0);
	});

	it("confirming deletes the targeted container with the right request", async () => {
		resetCounters();
		let remaining = [first, second];
		server.use(
			containersHandler(() => remaining),
			deleteHandler(() => {
				remaining = [second];
			}),
		);
		const user = await openDeleteModal();

		await user.click(screen.getByTestId("confirm-delete-button"));

		await waitFor(() => expect(deleteRequests.count).toBe(1));
		expect(deleteRequests.method).toBe("DELETE");
		expect(deleteRequests.pathname).toBe("/api/hosts/m-1/containers/c-1");
		expect(deleteRequests.search).toBe("");
		expect(await screen.findByText("survivor")).toBeInTheDocument();
		await waitFor(() =>
			expect(screen.queryByText("victim")).not.toBeInTheDocument(),
		);
	});

	it("shows the error envelope and keeps the row when the delete fails", async () => {
		resetCounters();
		server.use(
			containersHandler(() => [first]),
			http.delete("*/api/hosts/m-1/containers/*", () =>
				errorResponse(
					409,
					"CONTAINER_RUNNING",
					"container is running",
					"req-13",
				),
			),
		);
		const user = await openDeleteModal();

		const confirm = screen.getByTestId("confirm-delete-button");
		await user.click(confirm);

		expect(await screen.findByRole("alert")).toBeInTheDocument();
		expect(screen.getByText("CONTAINER_RUNNING")).toBeInTheDocument();
		expect(screen.getByText("container is running")).toBeInTheDocument();
		await waitFor(() => expect(screen.getByText("victim")).toBeInTheDocument());
	});
});
