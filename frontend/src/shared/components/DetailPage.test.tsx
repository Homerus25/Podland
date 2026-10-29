import {
	DetailPage,
	type DetailPageProps,
	type InspectQuerySlice,
} from "@shared/components/DetailPage";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { type ReactElement, useState } from "react";
import { Route, Routes, useLocation } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";
import { axiosError } from "@/test/errors";
import { rawInspectFixture } from "@/test/factories";
import { renderWithQuery } from "@/test/render";

// The module renders the slices the caller hands it, never react-query
// itself: the stub below is the whole query interface under test.
function makeQuery(
	overrides: Partial<InspectQuerySlice> = {},
): InspectQuerySlice {
	return {
		isPending: false,
		error: null,
		data: undefined,
		...overrides,
	};
}

function defaultProps(
	overrides: Partial<DetailPageProps> = {},
): DetailPageProps {
	return {
		title: "web",
		backTo: "containers",
		isPending: false,
		error: null,
		notFound: false,
		inspectQuery: makeQuery(),
		revealed: false,
		onToggleInspect: () => {},
		children: <div data-testid="overview-name-row">Name: web</div>,
		...overrides,
	};
}

// The layout reads machineId from the route (the deep-link branch's back
// button targets /machines/:machineId?tab=<resource>), so every render goes
// through a matching route inside the harness's MemoryRouter. The initial
// entries seed the history stack: one entry is a deep link (the session's
// initial entry), two entries simulate in-app navigation to the detail page.
function renderDetailPage(
	ui: ReactElement,
	initialEntries: string[] = ["/machines/m-1/containers/c-1"],
) {
	return renderWithQuery(
		<Routes>
			<Route path="/machines/:machineId/containers/:containerId" element={ui} />
			<Route path="/machines/:machineId/images/:imageId" element={ui} />
			<Route path="*" element={<LocationProbe />} />
		</Routes>,
		{ router: { initialEntries } },
	);
}

// The back button navigates, so the suite observes the router through the
// public interface: a probe reading the current pathname.
function LocationProbe() {
	const location = useLocation();
	return <div data-testid="location-probe">{location.pathname}</div>;
}

// The reveal toggle is page-owned state: this harness wires it the way a
// detail page does, so the toggle tests exercise the real contract.
function StatefulDetailPage(
	props: Omit<DetailPageProps, "revealed" | "onToggleInspect">,
) {
	const [revealed, setRevealed] = useState(false);
	return (
		<DetailPage
			{...props}
			revealed={revealed}
			onToggleInspect={() => setRevealed((value) => !value)}
		/>
	);
}

// A test double for the lazy inspect query: it counts "fetches" and only
// delivers data on the first reveal, pinning the lazy-fetch contract
// without importing a feature hook (the features→shared dependency
// direction stays one-way).
function LazyInspectDouble({ onFetch }: { onFetch: () => void }) {
	const [revealed, setRevealed] = useState(false);
	const [data, setData] = useState<unknown>(undefined);
	const inspectQuery: InspectQuerySlice = {
		isPending: false,
		error: null,
		data,
	};
	return (
		<DetailPage
			title="web"
			backTo="containers"
			isPending={false}
			error={null}
			notFound={false}
			inspectQuery={inspectQuery}
			revealed={revealed}
			onToggleInspect={() => {
				setRevealed((value) => {
					const next = !value;
					if (next && data === undefined) {
						onFetch();
						setData(rawInspectFixture());
					}
					return next;
				});
			}}
		>
			<div data-testid="overview-name-row">Name: web</div>
		</DetailPage>
	);
}

describe("DetailPage", () => {
	it("links to the machine's resource tab when the detail page is the session's initial entry (deep link)", () => {
		renderDetailPage(<DetailPage {...defaultProps()} />);

		expect(screen.getByTestId("detail-page-title")).toHaveTextContent("web");
		expect(screen.getByTestId("detail-back-button")).toHaveAttribute(
			"href",
			"/machines/m-1?tab=containers",
		);
	});

	it("returns to the referring page via history back when the detail page was reached by in-app navigation", async () => {
		// The detail page was reached by in-app navigation (here: from the
		// machine page; the cross-link detail→detail case is pinned in
		// DetailCrossLinks.test.tsx), so Back rides the history stack and
		// the referring page returns, exactly like the browser's back
		// button.
		const user = userEvent.setup();
		renderDetailPage(<DetailPage {...defaultProps({ backTo: "images" })} />, [
			"/machines/m-1",
			"/machines/m-1/images/i-1",
		]);

		const back = screen.getByTestId("detail-back-button");
		// The history branch is a click handler, not a link — the accepted
		// trade-off: no middle-click/open-in-new-tab in this branch.
		expect(back).not.toHaveAttribute("href");

		await user.click(back);

		expect(await screen.findByTestId("location-probe")).toHaveTextContent(
			"/machines/m-1",
		);
	});

	it("renders the loading state while the overview data is pending", () => {
		renderDetailPage(<DetailPage {...defaultProps({ isPending: true })} />);

		expect(screen.getByTestId("detail-page-loading")).toHaveTextContent(
			"Loading...",
		);
		expect(screen.queryByTestId("overview-name-row")).not.toBeInTheDocument();
		expect(screen.queryByTestId("detail-page-error")).not.toBeInTheDocument();
		expect(
			screen.queryByTestId("detail-page-not-found"),
		).not.toBeInTheDocument();
	});

	it("renders the error state via ErrorDisplay with getApiError", () => {
		renderDetailPage(
			<DetailPage
				{...defaultProps({
					error: axiosError(
						403,
						"ACCESS_DENIED",
						"You do not have access to this machine",
					),
				})}
			/>,
		);

		const alert = screen.getByTestId("detail-page-error");
		expect(alert).toHaveTextContent("ACCESS_DENIED");
		expect(alert).toHaveTextContent("You do not have access to this machine");
		expect(screen.queryByTestId("overview-name-row")).not.toBeInTheDocument();
	});

	it("renders the not-found state when the item no longer exists (404)", () => {
		renderDetailPage(
			<DetailPage
				{...defaultProps({
					notFound: true,
					error: axiosError(404, "CONTAINER_NOT_FOUND", "Container not found"),
				})}
			/>,
		);

		expect(screen.getByTestId("detail-page-not-found")).toHaveTextContent(
			"This item no longer exists on the machine.",
		);
		expect(screen.queryByTestId("detail-page-error")).not.toBeInTheDocument();
		expect(screen.queryByTestId("overview-name-row")).not.toBeInTheDocument();
	});

	it("renders the overview children once the data has loaded", () => {
		renderDetailPage(<DetailPage {...defaultProps()} />);

		expect(screen.getByTestId("overview-name-row")).toBeInTheDocument();
		expect(screen.queryByTestId("detail-page-loading")).not.toBeInTheDocument();
		expect(screen.queryByTestId("detail-page-error")).not.toBeInTheDocument();
		expect(
			screen.queryByTestId("detail-page-not-found"),
		).not.toBeInTheDocument();
	});

	// jsdom proves no real 360px layout; this pins the style props that
	// produce it (spec #393's testing decision, the LogsPage precedent):
	// an unbounded resource name in the title heading is the one overflow
	// risk on the page. The clamp engages only when the name overflows, so
	// desktop titles that fit are unchanged.
	it("clamps the title heading to one line so a long name cannot force page-level horizontal scroll", () => {
		renderDetailPage(
			<DetailPage
				{...defaultProps({
					title: "registry.example.com/some-team/a-very-long-image-name:latest",
				})}
			/>,
		);

		const heading = screen.getByTestId("detail-page-title");
		expect(heading).toHaveStyle("overflow: hidden");
		expect(heading).toHaveStyle("min-width: 0px");
		expect(
			window.getComputedStyle(heading).getPropertyValue("--chakra-line-clamp"),
		).toBe("1");
	});

	// The optional header actions slot (spec 412, ticket #414): a page
	// hands over the actions its header carries (the container detail
	// page's "Edit configuration" button). The slot is optional — the
	// pages that pass nothing render exactly the header they always did.
	it("renders the optional header actions beside the title when handed over", () => {
		renderDetailPage(
			<DetailPage
				{...defaultProps({
					headerActions: (
						<button type="button" data-testid="header-action-button">
							Edit configuration
						</button>
					),
				})}
			/>,
		);

		const action = screen.getByTestId("header-action-button");
		expect(action).toBeInTheDocument();
		// The action rides the header row, beside the back button and title.
		const header = screen.getByTestId("detail-page-title").parentElement;
		expect(header).toContainElement(action);
	});

	it("renders no header actions region when the slot is not used", () => {
		renderDetailPage(<DetailPage {...defaultProps()} />);

		expect(
			screen.queryByTestId("detail-page-header-actions"),
		).not.toBeInTheDocument();
	});

	it("keeps the inspect JSON hidden by default and toggles it on reveal", async () => {
		const user = userEvent.setup();
		renderDetailPage(<StatefulDetailPage {...defaultProps()} />);

		expect(screen.queryByTestId("inspect-json")).not.toBeInTheDocument();
		expect(screen.getByTestId("toggle-inspect-button")).toHaveTextContent(
			"Show inspect JSON",
		);

		await user.click(screen.getByTestId("toggle-inspect-button"));

		expect(screen.getByTestId("inspect-json")).toBeInTheDocument();
		expect(screen.getByTestId("toggle-inspect-button")).toHaveTextContent(
			"Hide inspect JSON",
		);

		await user.click(screen.getByTestId("toggle-inspect-button"));

		expect(screen.queryByTestId("inspect-json")).not.toBeInTheDocument();
		expect(screen.getByTestId("toggle-inspect-button")).toHaveTextContent(
			"Show inspect JSON",
		);
	});

	it("renders the pretty-printed inspect JSON with a 2-space indent when revealed", () => {
		const data = rawInspectFixture();
		renderDetailPage(
			<DetailPage
				{...defaultProps({
					revealed: true,
					inspectQuery: makeQuery({ data }),
				})}
			/>,
		);

		const pre = screen.getByTestId("inspect-json");
		expect(pre.tagName).toBe("PRE");
		expect(pre.textContent).toBe(JSON.stringify(data, null, 2));
	});

	it("copies the pretty-printed inspect JSON to the clipboard", async () => {
		const user = userEvent.setup();
		// userEvent.setup() installs its own clipboard stub, so the spy goes
		// on after it to be the one the button's click handler reads.
		const writeText = vi.fn().mockResolvedValue(undefined);
		Object.defineProperty(navigator, "clipboard", {
			value: { writeText },
			configurable: true,
		});
		const data = rawInspectFixture();
		renderDetailPage(
			<DetailPage
				{...defaultProps({
					revealed: true,
					inspectQuery: makeQuery({ data }),
				})}
			/>,
		);

		await user.click(screen.getByTestId("copy-inspect-button"));

		expect(writeText).toHaveBeenCalledTimes(1);
		expect(writeText).toHaveBeenCalledWith(JSON.stringify(data, null, 2));
	});

	it("renders the inspect JSON only after the lazy query delivers data on first reveal", async () => {
		let fetches = 0;
		const user = userEvent.setup();
		renderDetailPage(<LazyInspectDouble onFetch={() => fetches++} />);

		// The page mounts with the overview rendered and no inspect data:
		// the query is enabled on first reveal, not on mount.
		expect(await screen.findByTestId("overview-name-row")).toBeInTheDocument();
		expect(fetches).toBe(0);
		expect(screen.queryByTestId("inspect-json")).not.toBeInTheDocument();

		await user.click(screen.getByTestId("toggle-inspect-button"));

		expect((await screen.findByTestId("inspect-json")).textContent).toBe(
			JSON.stringify(rawInspectFixture(), null, 2),
		);
		expect(fetches).toBe(1);
	});
});
