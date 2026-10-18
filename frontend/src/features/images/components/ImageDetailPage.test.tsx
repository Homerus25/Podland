import { ImageDetailPage } from "@features/images/components/ImageDetailPage";
import { screen, within } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import { Route, Routes } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import { errorResponse } from "@/test/errors";
import { imageDetailFixture } from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

const image = imageDetailFixture();

// The image id is "sha256:…", so the API path percent-encodes the segment;
// the :imageId param matches the encoded segment, and the router decodes it
// on the far side.
function detailHandler(responder: () => ReturnType<typeof HttpResponse.json>) {
	server.use(http.get("*/api/machines/m-1/images/:imageId", responder));
}

function renderDetailPage() {
	return renderWithQuery(
		<Routes>
			<Route
				path="/machines/:machineId/images/:imageId"
				element={<ImageDetailPage />}
			/>
		</Routes>,
		{
			router: {
				initialEntries: [
					`/machines/m-1/images/${encodeURIComponent(image.id)}`,
				],
			},
		},
	);
}

describe("ImageDetailPage", () => {
	it("renders the overview with all of the image's attributes", async () => {
		vi.useFakeTimers({ shouldAdvanceTime: true });
		vi.setSystemTime(new Date("2026-08-21T12:00:00Z"));
		detailHandler(() => HttpResponse.json(image));
		renderDetailPage();

		const overview = await screen.findByTestId("detail-page-overview");
		expect(screen.getByTestId("detail-page-title")).toHaveTextContent(
			"docker.io/library/nginx:1.27, docker.io/library/nginx:latest",
		);
		for (const label of [
			"Name",
			"Id",
			"Created",
			"User",
			"Env",
			"Entrypoint",
			"Cmd",
			"Labels",
			"Size",
		]) {
			expect(within(overview).getByText(label)).toBeInTheDocument();
		}
		expect(overview).toHaveTextContent(image.id);
		expect(overview).toHaveTextContent("3 days ago");
		expect(overview).toHaveTextContent("appuser");
		expect(overview).toHaveTextContent("/docker-entrypoint.sh");
		expect(overview).toHaveTextContent("nginx -g daemon off;");
		expect(overview).toHaveTextContent("157.3 MB");

		const env = within(overview).getByTestId("image-detail-env");
		expect(env).toHaveTextContent("PATH");
		expect(env).toHaveTextContent("/usr/local/sbin:/usr/local/bin");
		expect(env).toHaveTextContent("NGINX_VERSION");
		expect(env).toHaveTextContent("1.27.1");

		const labels = within(overview).getByTestId("image-detail-labels");
		expect(labels).toHaveTextContent("maintainer");
		expect(labels).toHaveTextContent("NGINX Official Image");
		expect(labels).toHaveTextContent("tier");
		expect(labels).toHaveTextContent("frontend");
	});

	it("shows the loading state while the detail is pending", () => {
		detailHandler(
			() =>
				new Promise(() => {
					/* never resolves */
				}) as unknown as ReturnType<typeof HttpResponse.json>,
		);
		renderDetailPage();

		expect(screen.getByTestId("detail-page-loading")).toBeInTheDocument();
		expect(
			screen.queryByTestId("detail-page-overview"),
		).not.toBeInTheDocument();
	});

	it("renders an em-dash for an empty user and None for empty env and labels", async () => {
		detailHandler(() =>
			HttpResponse.json(
				imageDetailFixture({
					user: "",
					env: {},
					labels: {},
					entrypoint: [],
					cmd: [],
				}),
			),
		);
		renderDetailPage();

		const overview = await screen.findByTestId("detail-page-overview");
		// User, Entrypoint, and Cmd all render the em-dash placeholder.
		expect(within(overview).getAllByText("—")).toHaveLength(3);
		expect(within(overview).getByTestId("image-detail-env")).toHaveTextContent(
			"None",
		);
		expect(
			within(overview).getByTestId("image-detail-labels"),
		).toHaveTextContent("None");
	});

	it("renders the None placeholders when the detail response omits the optional fields", async () => {
		// The wire shape for an image whose config carries no user, env,
		// entrypoint, cmd, or labels: the page must render its placeholders,
		// not throw during render.
		detailHandler(() =>
			HttpResponse.json({
				id: image.id,
				repoTags: image.repoTags,
				repoDigests: image.repoDigests,
				createdAt: image.createdAt,
				size: image.size,
			}),
		);
		renderDetailPage();

		const overview = await screen.findByTestId("detail-page-overview");
		expect(within(overview).getAllByText("—")).toHaveLength(3);
		expect(within(overview).getByTestId("image-detail-env")).toHaveTextContent(
			"None",
		);
		expect(
			within(overview).getByTestId("image-detail-labels"),
		).toHaveTextContent("None");
	});

	it("renders the error state when the detail endpoint fails", async () => {
		detailHandler(() =>
			errorResponse(
				403,
				"ACCESS_DENIED",
				"You do not have access to this machine",
				"r-1",
			),
		);
		renderDetailPage();

		const alert = await screen.findByTestId("detail-page-error");
		expect(alert).toHaveTextContent("ACCESS_DENIED");
		expect(alert).toHaveTextContent("You do not have access to this machine");
		expect(
			screen.queryByTestId("detail-page-overview"),
		).not.toBeInTheDocument();
	});

	it("renders the not-found state when the image no longer exists (404)", async () => {
		detailHandler(() =>
			errorResponse(404, "IMAGE_NOT_FOUND", "Image not found", "r-2"),
		);
		renderDetailPage();

		expect(
			await screen.findByTestId("detail-page-not-found"),
		).toHaveTextContent("This item no longer exists on the machine.");
		expect(screen.queryByTestId("detail-page-error")).not.toBeInTheDocument();
		expect(
			screen.queryByTestId("detail-page-overview"),
		).not.toBeInTheDocument();
	});

	it("links the back button to the machine's images tab", async () => {
		detailHandler(() => HttpResponse.json(image));
		renderDetailPage();

		await screen.findByTestId("detail-page-overview");
		expect(screen.getByTestId("detail-back-button")).toHaveAttribute(
			"href",
			"/machines/m-1?tab=images",
		);
	});
});

afterEach(() => {
	vi.useRealTimers();
});
