import { ConnectDialog } from "@features/networks/components/ConnectDialog";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http, type JsonBodyType } from "msw";
import { describe, expect, it, vi } from "vitest";
import { errorJson } from "@/test/errors";
import { containerFixture, networkFixture } from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

// The bulk variant's seam (spec #486, ticket #491): the containers list's
// "Connect selected to network". One network is chosen once for the whole
// selection, the connects run one at a time in list order over the existing
// per-item endpoint, and the body becomes a per-item report as they settle.
// The two single-fixed-side flows are pinned in ConnectDialog.test.tsx and
// are untouched by this suite.

// web and db are both on app-net; queue is on app-net and other-net.
const web = containerFixture({
	id: "c-1",
	names: ["/web"],
	networks: ["app-net"],
});
const db = containerFixture({
	id: "c-2",
	names: ["/db"],
	networks: ["app-net"],
});
const queue = containerFixture({
	id: "c-4",
	names: ["/queue"],
	networks: ["app-net", "other-net"],
});

let connectRequests: { networkName: string; container: string }[] = [];
let inFlight = 0;
let maxInFlight = 0;
let networksRequests = 0;

function resetCounters() {
	connectRequests = [];
	inFlight = 0;
	maxInFlight = 0;
	networksRequests = 0;
}

function mockNetworks() {
	return http.get("*/api/hosts/m-1/networks", () => {
		networksRequests++;
		return HttpResponse.json([
			networkFixture(),
			networkFixture({ name: "other-net", subnets: ["10.89.0.0/16"] }),
			networkFixture({ name: "third-net", subnets: ["10.90.0.0/16"] }),
		]);
	});
}

// A connect handler that records every request and how many were in flight
// at once (so a parallel fire shows up as an overlap), answers the listed
// container ids with a failure envelope, and holds the listed ones open
// until the test releases them — which is what makes the report's
// fill-in-as-it-settles behavior observable rather than raced.
function connectHandler(
	options: {
		failures?: Record<string, { status: number; body: JsonBodyType }>;
		hold?: string[];
	} = {},
) {
	const { failures = {}, hold = [] } = options;
	let release: () => void = () => {};
	const gate = new Promise<void>((resolve) => {
		release = resolve;
	});
	const handler = http.post(
		"*/api/hosts/m-1/networks/:networkName/connect",
		async ({ request, params }) => {
			inFlight++;
			maxInFlight = Math.max(maxInFlight, inFlight);
			const body = (await request.json()) as { container: string };
			connectRequests.push({
				networkName: params.networkName as string,
				container: body.container,
			});
			if (hold.includes(body.container)) await gate;
			inFlight--;
			const failure = failures[body.container];
			if (failure)
				return HttpResponse.json(failure.body, { status: failure.status });
			return new HttpResponse(null, { status: 200 });
		},
	);
	return { handler, release: () => release() };
}

// The picker is a single-select: one click on the input, one on the option.
async function pickNetwork(
	user: ReturnType<typeof userEvent.setup>,
	networkName: string,
) {
	await user.click(screen.getByTestId("searchable-select-input"));
	await user.click(
		await screen.findByTestId(`searchable-select-option-${networkName}`),
	);
}

function reportRows() {
	return screen.getAllByTestId("connect-selected-report-row");
}

describe("ConnectDialog with the containers fixed", () => {
	it("names the selection and offers the networks not every selected container is already on", async () => {
		resetCounters();
		server.use(mockNetworks(), connectHandler().handler);
		renderWithQuery(
			<ConnectDialog
				hostId="m-1"
				containers={[web, db]}
				onClose={() => {}}
				onFinished={() => {}}
			/>,
		);
		const user = userEvent.setup();

		expect(
			screen.getByText("Connect 2 containers to a network"),
		).toBeInTheDocument();

		await user.click(screen.getByTestId("searchable-select-input"));
		expect(
			await screen.findByTestId("searchable-select-option-other-net"),
		).toBeInTheDocument();
		expect(
			screen.getByTestId("searchable-select-option-third-net"),
		).toBeInTheDocument();
		// Both selected containers are already on app-net, so connecting
		// them to it could only produce an error: an option that is bait
		// never appears.
		expect(
			screen.queryByTestId("searchable-select-option-app-net"),
		).not.toBeInTheDocument();
	});

	it("keeps a network every selected container is not on, even when one of them is attached", async () => {
		resetCounters();
		server.use(mockNetworks(), connectHandler().handler);
		renderWithQuery(
			<ConnectDialog
				hostId="m-1"
				containers={[web, queue]}
				onClose={() => {}}
				onFinished={() => {}}
			/>,
		);
		const user = userEvent.setup();

		await user.click(screen.getByTestId("searchable-select-input"));
		// queue is on other-net but web is not, so the pair can succeed.
		expect(
			await screen.findByTestId("searchable-select-option-other-net"),
		).toBeInTheDocument();
		expect(
			screen.queryByTestId("searchable-select-option-app-net"),
		).not.toBeInTheDocument();
	});

	it("chooses the network once for the whole selection and keeps submit disabled until it is chosen", async () => {
		resetCounters();
		server.use(mockNetworks(), connectHandler().handler);
		renderWithQuery(
			<ConnectDialog
				hostId="m-1"
				containers={[web, db]}
				onClose={() => {}}
				onFinished={() => {}}
			/>,
		);
		const user = userEvent.setup();

		expect(screen.getByTestId("connect-selected-submit")).toBeDisabled();
		await pickNetwork(user, "third-net");
		await waitFor(() =>
			expect(screen.getByTestId("connect-selected-submit")).toBeEnabled(),
		);

		await user.click(screen.getByTestId("connect-selected-submit"));
		await waitFor(() => expect(connectRequests).toHaveLength(2));
		// One choice for the whole selection: every request carries it, and
		// no per-container confirmation ever appears.
		expect(connectRequests.map((request) => request.networkName)).toEqual([
			"third-net",
			"third-net",
		]);
		expect(
			screen.queryByTestId("connect-selected-form"),
		).not.toBeInTheDocument();
	});

	it("connects the selection one at a time in list order and fills the report in as each item settles", async () => {
		resetCounters();
		const { handler, release } = connectHandler({ hold: ["c-2"] });
		server.use(mockNetworks(), handler);
		renderWithQuery(
			<ConnectDialog
				hostId="m-1"
				containers={[web, db]}
				onClose={() => {}}
				onFinished={() => {}}
			/>,
		);
		const user = userEvent.setup();

		await pickNetwork(user, "third-net");
		await user.click(screen.getByTestId("connect-selected-submit"));

		// The body becomes the report and the footer swaps to Close.
		expect(
			await screen.findByTestId("connect-selected-report"),
		).toBeInTheDocument();
		expect(
			screen.getByTestId("close-connect-selected-report-button"),
		).toBeInTheDocument();
		expect(
			screen.queryByTestId("connect-selected-submit"),
		).not.toBeInTheDocument();
		// One row per selected container, in list order.
		expect(reportRows()).toHaveLength(2);

		// The first item settles while the second is still in flight.
		await waitFor(() =>
			expect(
				within(reportRows()[0]).getByText("Connected"),
			).toBeInTheDocument(),
		);
		expect(
			within(reportRows()[1]).getByText("Connecting…"),
		).toBeInTheDocument();

		release();
		await waitFor(() =>
			expect(
				within(reportRows()[1]).getByText("Connected"),
			).toBeInTheDocument(),
		);
		// List order, one request at a time.
		expect(connectRequests.map((request) => request.container)).toEqual([
			"c-1",
			"c-2",
		]);
		expect(maxInFlight).toBe(1);
	});

	it("reports the failure per item and hands onFinished only the succeeded ids", async () => {
		resetCounters();
		const onFinished = vi.fn();
		const { handler } = connectHandler({
			failures: {
				"c-2": {
					status: 404,
					body: errorJson(
						"CONTAINER_NOT_FOUND",
						"container not found",
						"req-31",
					),
				},
			},
		});
		server.use(mockNetworks(), handler);
		renderWithQuery(
			<ConnectDialog
				hostId="m-1"
				containers={[web, db]}
				onClose={() => {}}
				onFinished={onFinished}
			/>,
		);
		const user = userEvent.setup();

		await pickNetwork(user, "third-net");
		await user.click(screen.getByTestId("connect-selected-submit"));

		await waitFor(() => expect(onFinished).toHaveBeenCalledTimes(1));
		// Only the succeeded container leaves the caller's selection.
		expect(onFinished).toHaveBeenCalledWith(["c-1"]);
		// The failure does not stop the batch: both requests fired.
		expect(connectRequests.map((request) => request.container)).toEqual([
			"c-1",
			"c-2",
		]);
		const rows = reportRows();
		expect(within(rows[0]).getByText("Connected")).toBeInTheDocument();
		expect(
			within(rows[1]).getByText("container not found"),
		).toBeInTheDocument();
		// The report stays open: the failed containers are the retry list.
		expect(screen.getByTestId("modal")).toBeInTheDocument();
	});

	it("aborts the in-flight connect on close — the remaining items never start and the abandoned run reports nothing", async () => {
		resetCounters();
		const onFinished = vi.fn();
		const onClose = vi.fn();
		const { handler } = connectHandler({ hold: ["c-2"] });
		server.use(mockNetworks(), handler);
		renderWithQuery(
			<ConnectDialog
				hostId="m-1"
				containers={[web, db, queue]}
				onClose={onClose}
				onFinished={onFinished}
			/>,
		);
		const user = userEvent.setup();

		await pickNetwork(user, "third-net");
		await user.click(screen.getByTestId("connect-selected-submit"));
		// c-1 settled, c-2 is held in flight, c-3 has not started.
		await waitFor(() =>
			expect(connectRequests.map((request) => request.container)).toEqual([
				"c-1",
				"c-2",
			]),
		);

		await user.click(
			screen.getByTestId("close-connect-selected-report-button"),
		);

		expect(onClose).toHaveBeenCalledTimes(1);
		// The aborted run settles — the in-flight request rejects, the loop
		// stops, the invalidation fires — and reports nothing back: the
		// batch transfer dialog's abandoned-run rule (spec 486 review).
		await waitFor(() => expect(networksRequests).toBeGreaterThanOrEqual(2));
		expect(connectRequests.map((request) => request.container)).toEqual([
			"c-1",
			"c-2",
		]);
		expect(onFinished).not.toHaveBeenCalled();
	});
});
