import { ContainerDetailPage } from "@features/containers/components/ContainerDetailPage";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { Route, Routes } from "react-router-dom";
import { describe, expect, it } from "vitest";
import { errorResponse } from "@/test/errors";
import { containerDetailFixture, rawInspectFixture } from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

function mockDetail(responder: () => ReturnType<typeof HttpResponse.json>) {
	server.use(http.get("*/api/machines/m-1/containers/c-1", responder));
}

function renderDetailPage() {
	return renderWithQuery(
		<Routes>
			<Route
				path="/machines/:machineId/containers/:containerId"
				element={<ContainerDetailPage />}
			/>
		</Routes>,
		{ router: { initialEntries: ["/machines/m-1/containers/c-1"] } },
	);
}

describe("ContainerDetailPage", () => {
	it("renders the overview with the container's attributes", async () => {
		mockDetail(() => HttpResponse.json(containerDetailFixture()));
		renderDetailPage();

		expect(
			await screen.findByTestId("container-detail-name"),
		).toHaveTextContent("web");
		expect(screen.getByTestId("detail-page-title")).toHaveTextContent("web");
		expect(screen.getByTestId("container-detail-id")).toHaveTextContent("c-1");
		expect(screen.getByTestId("state-badge")).toHaveTextContent("running");
		expect(screen.getByTestId("container-image-link")).toHaveTextContent(
			"docker.io/library/nginx:1.27",
		);

		const volumes = screen.getByTestId("container-detail-volumes");
		expect(volumes).toHaveTextContent("app-data");
		expect(volumes).toHaveTextContent(
			"/var/lib/containers/storage/volumes/app-data/_data",
		);
		expect(volumes).toHaveTextContent("/data");
		expect(volumes).toHaveTextContent("rw");

		expect(screen.getByTestId("container-detail-mounts")).toHaveTextContent(
			"type=volume,source=/var/lib/containers/storage/volumes/app-data/_data,destination=/data",
		);
		expect(screen.getByTestId("container-detail-networks")).toHaveTextContent(
			"app-net",
		);
		expect(
			screen.getByTestId("container-detail-exposed-ports"),
		).toHaveTextContent("8080/tcp");
		expect(screen.getByTestId("container-detail-entrypoint")).toHaveTextContent(
			"/docker-entrypoint.sh",
		);

		const environment = screen.getByTestId("container-detail-environment");
		expect(environment).toHaveTextContent("FOO");
		expect(environment).toHaveTextContent("bar");
		expect(environment).toHaveTextContent("MODE");
		expect(environment).toHaveTextContent("prod");

		expect(screen.getByTestId("container-detail-secrets")).toHaveTextContent(
			"db-password",
		);
		expect(
			screen.getByTestId("container-detail-created"),
		).not.toBeEmptyDOMElement();
		expect(screen.getByTestId("container-detail-command")).toHaveTextContent(
			"nginx -g daemon off;",
		);
	});

	it("links the image name to the image detail page", async () => {
		const detail = containerDetailFixture();
		mockDetail(() => HttpResponse.json(detail));
		renderDetailPage();

		const link = await screen.findByTestId("container-image-link");
		// The image id is "sha256:…", so the route kit percent-encodes the
		// segment; the router decodes it on the far side.
		expect(link).toHaveAttribute(
			"href",
			`/machines/m-1/images/${encodeURIComponent(detail.imageId!)}`,
		);
	});

	it("renders the image name as plain text when the container has no imageId", async () => {
		mockDetail(() =>
			HttpResponse.json(containerDetailFixture({ imageId: "" })),
		);
		renderDetailPage();

		expect(
			await screen.findByText("docker.io/library/nginx:1.27"),
		).toBeInTheDocument();
		expect(
			screen.queryByTestId("container-image-link"),
		).not.toBeInTheDocument();
	});

	it("links each secret name to the secret detail page", async () => {
		mockDetail(() =>
			HttpResponse.json(
				containerDetailFixture({
					secrets: [
						{ name: "db-password", uid: 0, gid: 0, mode: 420 },
						{ name: "api-token", uid: 0, gid: 0, mode: 420 },
					],
				}),
			),
		);
		renderDetailPage();

		expect(
			await screen.findByTestId("container-secret-link-db-password"),
		).toHaveAttribute("href", "/machines/m-1/secrets/db-password");
		expect(
			screen.getByTestId("container-secret-link-api-token"),
		).toHaveAttribute("href", "/machines/m-1/secrets/api-token");
	});

	it("links each volume name to the volume detail page, keeping the rest of the line plain text", async () => {
		mockDetail(() => HttpResponse.json(containerDetailFixture()));
		renderDetailPage();

		const link = await screen.findByTestId("container-volume-link-app-data");
		expect(link).toHaveAttribute("href", "/machines/m-1/volumes/app-data");
		expect(link).toHaveTextContent("app-data");
		// Only the name is the link: the source path, destination, and
		// rw/ro suffix stay outside it.
		expect(link).not.toHaveTextContent("/data");
		expect(link).not.toHaveTextContent("rw");
		const volumes = screen.getByTestId("container-detail-volumes");
		expect(volumes).toHaveTextContent(
			"/var/lib/containers/storage/volumes/app-data/_data",
		);
		expect(volumes).toHaveTextContent("/data");
		expect(volumes).toHaveTextContent("rw");
	});

	it("returns to the containers tab via the back button", async () => {
		mockDetail(() => HttpResponse.json(containerDetailFixture()));
		renderDetailPage();

		await screen.findByTestId("container-detail-name");
		expect(screen.getByTestId("detail-back-button")).toHaveAttribute(
			"href",
			"/machines/m-1?tab=containers",
		);
	});

	it("wires the inspect query to the reveal state (no request on mount, one on first reveal)", async () => {
		let inspectRequests = 0;
		mockDetail(() => HttpResponse.json(containerDetailFixture()));
		server.use(
			http.get("*/api/machines/m-1/containers/c-1/inspect", () => {
				inspectRequests++;
				return HttpResponse.json(rawInspectFixture());
			}),
		);
		renderDetailPage();
		const user = userEvent.setup();

		// The page owns enabled: revealed — the inspect query stays silent
		// until the first reveal click.
		expect(
			await screen.findByTestId("container-detail-name"),
		).toBeInTheDocument();
		expect(inspectRequests).toBe(0);

		await user.click(screen.getByTestId("toggle-inspect-button"));
		await screen.findByTestId("inspect-json");
		expect(inspectRequests).toBe(1);
	});

	it("renders the not-found state when the container no longer exists", async () => {
		mockDetail(() =>
			errorResponse(404, "CONTAINER_NOT_FOUND", "Container not found"),
		);
		renderDetailPage();

		expect(
			await screen.findByTestId("detail-page-not-found"),
		).toHaveTextContent("This item no longer exists on the machine.");
		expect(screen.queryByTestId("detail-page-error")).not.toBeInTheDocument();
	});

	it("renders the error state when the machine rejects the request", async () => {
		mockDetail(() =>
			errorResponse(
				403,
				"ACCESS_DENIED",
				"You do not have access to this machine",
				"r-2",
			),
		);
		renderDetailPage();

		const alert = await screen.findByTestId("detail-page-error");
		expect(alert).toHaveTextContent("ACCESS_DENIED");
		expect(alert).toHaveTextContent("You do not have access to this machine");
		expect(
			screen.queryByTestId("detail-page-not-found"),
		).not.toBeInTheDocument();
	});
});
