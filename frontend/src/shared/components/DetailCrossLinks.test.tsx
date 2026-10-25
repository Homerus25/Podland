import { ContainerDetailPage } from "@features/containers/components/ContainerDetailPage";
import { ImageDetailPage } from "@features/images/components/ImageDetailPage";
import { NetworkDetailPage } from "@features/networks/components/NetworkDetailPage";
import { SecretDetailPage } from "@features/secrets/components/SecretDetailPage";
import { VolumeDetailPage } from "@features/volumes/components/VolumeDetailPage";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { Route, Routes, useLocation } from "react-router-dom";
import { describe, expect, it } from "vitest";
import { MachineDetailPage } from "@/pages/MachineDetailPage";
import {
	containerDetailFixture,
	containerFixture,
	imageDetailFixture,
	imageFixture,
	machineFixture,
	rawInspectFixture,
	secretFixture,
} from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

// The cross-link chain (spec 301, ticket #311): the containers list row
// opens the container detail page, whose image link opens the image detail
// page and whose secret links open the secret detail page. This suite
// drives the whole chain through the real routes and the real pages, so a
// broken link target anywhere in the chain fails here.
const container = containerFixture({ id: "c-1", names: ["/web"] });
const containerDetail = containerDetailFixture();
const image = imageDetailFixture({ id: containerDetail.imageId });
const secret = secretFixture();

// Every endpoint the chain touches: the machine, the containers list, the
// container/image/secret detail endpoints, and the lazy inspect endpoints
// (registered though the chain never reveals the JSON).
function mockChainEndpoints() {
	server.use(
		http.get("*/api/machines/m-1", () => HttpResponse.json(machineFixture())),
		http.get("*/api/machines/m-1/containers", () =>
			HttpResponse.json({
				containers: [container],
				total: 1,
				timestamp: "2026-08-26T00:00:00Z",
			}),
		),
		http.get("*/api/machines/m-1/containers/c-1", () =>
			HttpResponse.json(containerDetail),
		),
		http.get("*/api/machines/m-1/containers/c-1/inspect", () =>
			HttpResponse.json(rawInspectFixture()),
		),
		http.get("*/api/machines/m-1/images", () =>
			HttpResponse.json([imageFixture({ id: containerDetail.imageId })]),
		),
		http.get("*/api/machines/m-1/images/:imageId", () =>
			HttpResponse.json(image),
		),
		http.get("*/api/machines/m-1/images/:imageId/inspect", () =>
			HttpResponse.json(rawInspectFixture()),
		),
		http.get("*/api/machines/m-1/secrets/db-password", () =>
			HttpResponse.json(secret),
		),
		http.get("*/api/machines/m-1/secrets/db-password/inspect", () =>
			HttpResponse.json(rawInspectFixture()),
		),
	);
}

// The chain navigates, so the suite observes the location through the
// public interface: a probe reading the router's pathname and search.
function LocationProbe() {
	const location = useLocation();
	return (
		<div data-testid="location">
			{location.pathname}
			{location.search}
		</div>
	);
}

// The real route table from App.tsx (minus the protected layout): the
// machine detail page plus the five resource detail routes.
function renderAppRoutes() {
	return renderWithQuery(
		<>
			<LocationProbe />
			<Routes>
				<Route path="/machines/:machineId" element={<MachineDetailPage />} />
				<Route
					path="/machines/:machineId/containers/:containerId"
					element={<ContainerDetailPage />}
				/>
				<Route
					path="/machines/:machineId/volumes/:volumeName"
					element={<VolumeDetailPage />}
				/>
				<Route
					path="/machines/:machineId/images/:imageId"
					element={<ImageDetailPage />}
				/>
				<Route
					path="/machines/:machineId/networks/:networkName"
					element={<NetworkDetailPage />}
				/>
				<Route
					path="/machines/:machineId/secrets/:secretId"
					element={<SecretDetailPage />}
				/>
			</Routes>
		</>,
		{ router: { initialEntries: ["/machines/m-1?tab=containers"] } },
	);
}

describe("detail page cross-links", () => {
	it("navigates the chain: containers list → container detail → image detail → back → container detail → secret detail", async () => {
		mockChainEndpoints();
		renderAppRoutes();
		const user = userEvent.setup();

		// Land on the Containers tab: the list renders the container's row.
		expect(await screen.findByTestId("container-row")).toBeInTheDocument();
		expect(screen.getByTestId("location")).toHaveTextContent(
			"/machines/m-1?tab=containers",
		);

		// The row opens the container's detail page.
		await user.click(screen.getByTestId("container-row"));
		expect(
			await screen.findByTestId("container-detail-name"),
		).toHaveTextContent("web");
		expect(screen.getByTestId("location")).toHaveTextContent(
			"/machines/m-1/containers/c-1",
		);

		// The image name links to the image's detail page, addressed by the
		// container's imageId.
		await user.click(screen.getByTestId("container-image-link"));
		expect(await screen.findByTestId("image-detail-env")).toBeInTheDocument();
		expect(screen.getByTestId("detail-page-title")).toHaveTextContent(
			"docker.io/library/nginx:1.27, docker.io/library/nginx:latest",
		);
		// The image id is "sha256:…", so the navigated path carries the
		// percent-encoded segment (the route kit's C11 discipline).
		expect(screen.getByTestId("location")).toHaveTextContent(
			`/machines/m-1/images/${encodeURIComponent(containerDetail.imageId!)}`,
		);

		// The back button is history-aware: the image detail page was
		// reached by a cross-link, so Back rides the history stack and
		// returns to the referring container detail page.
		await user.click(screen.getByTestId("detail-back-button"));
		expect(
			await screen.findByTestId("container-detail-name"),
		).toHaveTextContent("web");
		expect(screen.getByTestId("location")).toHaveTextContent(
			"/machines/m-1/containers/c-1",
		);

		// The secret name links to the secret's detail page, addressed by
		// the secret's name.
		await user.click(screen.getByTestId("container-secret-link-db-password"));
		expect(await screen.findByTestId("secret-detail-name")).toHaveTextContent(
			"db-password",
		);
		expect(screen.getByTestId("detail-page-title")).toHaveTextContent(
			"db-password",
		);
		expect(screen.getByTestId("location")).toHaveTextContent(
			"/machines/m-1/secrets/db-password",
		);
	});
});
