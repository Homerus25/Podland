import { ImageList } from "@features/images/components/ImageList";
import { screen, waitFor } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import { afterEach, describe, expect, it, vi } from "vitest";
import { imageFixture } from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

const imagesFixture = [
	imageFixture({
		id: "sha256:abc123def4567890abc123def4567890abc123def4567890abc123def4567890",
	}),
];

function renderImageList() {
	return renderWithQuery(<ImageList machineId="m-1" permission="admin" />);
}

describe("ImageList", () => {
	it("renders one row per image with repo tag, human size, and relative date", async () => {
		server.use(
			http.get("*/api/machines/m-1/images", () =>
				HttpResponse.json(imagesFixture),
			),
		);
		renderImageList();

		await waitFor(() => {
			expect(screen.queryByTestId("images-loading")).not.toBeInTheDocument();
		});
		expect(
			await screen.findByText("docker.io/library/nginx:1.27"),
		).toBeInTheDocument();
		expect(screen.getByText("157.3 MB")).toBeInTheDocument();
	});

	it("shows all repo tags and falls back to <none> with the short id for a dangling image", async () => {
		server.use(
			http.get("*/api/machines/m-1/images", () =>
				HttpResponse.json([
					imageFixture({
						id: "sha256:abc123def4567890abc123def4567890abc123def4567890abc123def4567890",
						repoTags: [
							"docker.io/library/nginx:1.27",
							"docker.io/library/nginx:latest",
						],
					}),
					imageFixture({
						id: "sha256:fedcba0987654321fedcba0987654321fedcba0987654321fedcba0987654321",
						repoTags: null,
						createdAt: "2026-08-20T10:00:00Z",
						size: 3145728,
					}),
				]),
			),
		);
		renderImageList();

		await waitFor(() => {
			expect(screen.queryByTestId("images-loading")).not.toBeInTheDocument();
		});
		expect(
			await screen.findByText(
				"docker.io/library/nginx:1.27, docker.io/library/nginx:latest",
			),
		).toBeInTheDocument();
		expect(screen.getAllByText("<none>")).toHaveLength(1);
		expect(screen.getByText("fedcba098765")).toBeInTheDocument();
		expect(screen.getByText("3.1 MB")).toBeInTheDocument();
	});

	it("shows the relative creation age of each image", async () => {
		vi.useFakeTimers({ shouldAdvanceTime: true });
		vi.setSystemTime(new Date("2026-08-21T12:00:00Z"));
		server.use(
			http.get("*/api/machines/m-1/images", () =>
				HttpResponse.json(imagesFixture),
			),
		);
		renderImageList();

		expect(await screen.findByText("3 days ago")).toBeInTheDocument();
	});

	it("shows a loading state before images arrive, with the toolbar still above it", async () => {
		server.use(
			http.get("*/api/machines/m-1/images", async () => {
				await new Promise((resolve) => setTimeout(resolve, 50));
				return HttpResponse.json(imagesFixture);
			}),
		);
		renderImageList();

		expect(screen.getByTestId("images-loading")).toBeInTheDocument();
		expect(screen.getByTestId("pull-image-form")).toBeInTheDocument();
		expect(screen.getByTestId("prune-images-button")).toBeInTheDocument();
	});

	it("shows an explanatory empty state when the machine has no images", async () => {
		server.use(
			http.get("*/api/machines/m-1/images", () => HttpResponse.json([])),
		);
		renderImageList();

		const empty = await screen.findByTestId("images-empty");
		expect(empty.textContent).toMatch(/no images/i);
		expect(empty.textContent).toMatch(/pull one to get started/i);
		expect(screen.getByTestId("pull-image-form")).toBeInTheDocument();
		expect(screen.queryByTestId("image-row")).not.toBeInTheDocument();
	});
});

afterEach(() => {
	vi.useRealTimers();
});
