import { ImageList } from "@features/images/components/ImageList";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { useLocation } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import { hostFixture, imageFixture } from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

const imagesFixture = [
	imageFixture({
		id: "sha256:abc123def4567890abc123def4567890abc123def4567890abc123def4567890",
	}),
];

function renderImageList() {
	return renderWithQuery(<ImageList host={hostFixture()} />, {
		router: { initialEntries: ["/hosts/m-1"] },
	});
}

// The row click navigates, so the navigation tests observe the location
// through the public interface: a probe reading the router's pathname.
function LocationProbe() {
	const location = useLocation();
	return <div data-testid="location">{location.pathname}</div>;
}

function renderImageListWithLocationProbe() {
	renderWithQuery(
		<>
			<LocationProbe />
			<ImageList host={hostFixture()} />
		</>,
		{ router: { initialEntries: ["/hosts/m-1"] } },
	);
}

describe("ImageList", () => {
	it("renders one row per image with repo tag, human size, and relative date", async () => {
		server.use(
			http.get("*/api/hosts/m-1/images", () =>
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
			http.get("*/api/hosts/m-1/images", () =>
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
			http.get("*/api/hosts/m-1/images", () =>
				HttpResponse.json(imagesFixture),
			),
		);
		renderImageList();

		expect(await screen.findByText("3 days ago")).toBeInTheDocument();
	});

	it("shows a loading state before images arrive, with the toolbar still above it", async () => {
		server.use(
			http.get("*/api/hosts/m-1/images", async () => {
				await new Promise((resolve) => setTimeout(resolve, 50));
				return HttpResponse.json(imagesFixture);
			}),
		);
		renderImageList();

		expect(screen.getByTestId("images-loading")).toBeInTheDocument();
		expect(screen.getByTestId("pull-image-form")).toBeInTheDocument();
		expect(screen.getByTestId("prune-images-button")).toBeInTheDocument();
	});

	it("shows an explanatory empty state when the host has no images", async () => {
		server.use(http.get("*/api/hosts/m-1/images", () => HttpResponse.json([])));
		renderImageList();

		const empty = await screen.findByTestId("images-empty");
		expect(empty.textContent).toMatch(/no images/i);
		expect(empty.textContent).toMatch(/pull one to get started/i);
		expect(screen.getByTestId("pull-image-form")).toBeInTheDocument();
		expect(screen.queryByTestId("image-row")).not.toBeInTheDocument();
	});

	it("sorts rows when a column header is clicked", async () => {
		server.use(
			http.get("*/api/hosts/m-1/images", () =>
				HttpResponse.json([
					imageFixture({ repoTags: ["zebra:latest"] }),
					imageFixture({
						id: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
						repoTags: ["apple:latest"],
					}),
				]),
			),
		);
		renderImageList();
		await screen.findAllByTestId("image-row");
		const user = userEvent.setup();
		await user.click(screen.getByTestId("sort-image-0"));
		const rows = screen.getAllByTestId("image-row");
		expect(within(rows[0]).getByText("apple:latest")).toBeInTheDocument();
		expect(within(rows[1]).getByText("zebra:latest")).toBeInTheDocument();
	});

	it("navigates to the image's detail page when a row is clicked", async () => {
		server.use(
			http.get("*/api/hosts/m-1/images", () =>
				HttpResponse.json(imagesFixture),
			),
		);
		renderImageListWithLocationProbe();
		const user = userEvent.setup();

		await user.click(await screen.findByTestId("image-row"));

		expect(screen.getByTestId("location").textContent).toBe(
			`/hosts/m-1/images/${encodeURIComponent(imagesFixture[0].id)}`,
		);
		expect(screen.queryByTestId("modal")).not.toBeInTheDocument();
	});

	it("does not navigate when the row's delete button is clicked", async () => {
		server.use(
			http.get("*/api/hosts/m-1/images", () =>
				HttpResponse.json(imagesFixture),
			),
		);
		renderImageListWithLocationProbe();
		const user = userEvent.setup();

		const row = await screen.findByTestId("image-row");
		await user.click(
			within(row).getByRole("button", {
				name: "Delete docker.io/library/nginx:1.27",
			}),
		);

		// The delete confirm opens, not a navigation.
		expect(
			await screen.findByTestId("confirm-delete-button"),
		).toBeInTheDocument();
		expect(screen.getByTestId("location").textContent).toBe("/hosts/m-1");
	});
});

afterEach(() => {
	vi.useRealTimers();
});
