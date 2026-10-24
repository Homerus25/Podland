import {
	PullProgressDisplay,
	projectPullProgress,
} from "@features/images/components/PullProgressDisplay";
import type { PullProgress } from "@features/images/hooks/usePullImage";
import type { StreamEventState } from "@shared/streaming/ndjsonStream";
import { screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { renderWithQuery } from "@/test/render";

function pullProgress(state?: StreamEventState): PullProgress {
	return { operationId: "op-1", state };
}

describe("projectPullProgress", () => {
	it("falls back to the starting status before the first progress event", () => {
		expect(projectPullProgress(null)).toEqual({
			status: "Starting pull…",
		});
	});

	it("projects the streamed state: status, message, percent, byte line", () => {
		expect(
			projectPullProgress(
				pullProgress({
					current: 30,
					total: 100,
					status: "Downloading layers",
					message: "layer 1/2",
				}),
			),
		).toEqual({
			status: "Downloading layers",
			message: "layer 1/2",
			percent: 30,
			byteLine: "30 B / 100 B",
		});
	});

	it("omits the percent and byte line when the stream reports no amounts", () => {
		expect(
			projectPullProgress(
				pullProgress({ current: 5, status: "Copying blobs" }),
			),
		).toEqual({
			status: "Copying blobs",
		});
	});
});

describe("PullProgressDisplay", () => {
	it("renders the status fallback before the first progress event", () => {
		renderWithQuery(
			<PullProgressDisplay progress={null} data-testid="pull-progress" />,
		);

		expect(screen.getByText("Starting pull…")).toBeInTheDocument();
		expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
	});

	it("shows the message line when the stream reports one", () => {
		renderWithQuery(
			<PullProgressDisplay
				progress={pullProgress({
					status: "Downloading layers",
					message: "layer 1/2",
				})}
				data-testid="pull-progress"
			/>,
		);

		expect(screen.getByText("layer 1/2")).toBeInTheDocument();
	});

	it("hides the message line when the stream reports none", () => {
		renderWithQuery(
			<PullProgressDisplay
				progress={pullProgress({ status: "Downloading layers" })}
				data-testid="pull-progress"
			/>,
		);

		expect(screen.queryByText("layer 1/2")).not.toBeInTheDocument();
	});

	it("renders the percent bar and byte line only when the stream reports amounts", () => {
		renderWithQuery(
			<PullProgressDisplay
				progress={pullProgress({ status: "Copying blobs" })}
				data-testid="pull-progress"
			/>,
		);
		expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
		expect(screen.queryByText(/\/ 100 B/)).not.toBeInTheDocument();

		renderWithQuery(
			<PullProgressDisplay
				progress={pullProgress({
					current: 30,
					total: 100,
					status: "Downloading layers",
				})}
				data-testid="pull-progress"
			/>,
		);
		expect(screen.getByRole("progressbar")).toHaveAttribute(
			"aria-valuenow",
			"30",
		);
		expect(screen.getByText("30 B / 100 B")).toBeInTheDocument();
	});

	it("spreads extra props onto its root so call sites keep their own test ids", () => {
		renderWithQuery(
			<PullProgressDisplay
				progress={null}
				data-testid="container-pull-progress"
			/>,
		);

		expect(screen.getByTestId("container-pull-progress")).toBeInTheDocument();
	});
});
