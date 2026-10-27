import type { Machine } from "@shared/types/api";
import { QueryClient } from "@tanstack/react-query";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import { renderApp } from "@/test/auth";
import { errorResponse } from "@/test/errors";
import { containerFixture, machineFixture } from "@/test/factories";
import { breakpointRules } from "@/test/responsiveCss";
import { server } from "@/test/server";

function machineHandler(machine: Machine) {
	return http.get(`*/api/machines/${machine.id}`, () =>
		HttpResponse.json(machine),
	);
}

// Empty-list response resolvers: the module-level handlers below and the
// counting tab handlers both answer through these, so each endpoint's
// empty shape lives in exactly one place.
const emptyContainersResolver = () =>
	HttpResponse.json({
		containers: [],
		total: 0,
		timestamp: "2026-08-26T00:00:00Z",
	});

const emptyQuadletsResolver = () => HttpResponse.json([]);

const emptySecretsResolver = () => HttpResponse.json([]);

const emptyNetworksResolver = () => HttpResponse.json([]);

const emptyContainersHandler = http.get(
	"*/api/machines/m-1/containers",
	emptyContainersResolver,
);

const emptyQuadletsHandler = http.get(
	"*/api/machines/m-1/quadlets",
	emptyQuadletsResolver,
);

const emptySecretsHandler = http.get(
	"*/api/machines/m-1/secrets",
	emptySecretsResolver,
);

const emptyNetworksHandler = http.get(
	"*/api/machines/m-1/networks",
	emptyNetworksResolver,
);

function renderMachineDetailPage(id = "m-1") {
	return renderApp(`/machines/${id}`, {
		resolveInitialState: async () => ({ username: "alice", role: "user" }),
	});
}

// Per-endpoint request counts for the machine-scoped tab endpoints.
interface TabCounts {
	machine: number;
	containers: number;
	pods: number;
	volumes: number;
	images: number;
	networks: number;
	quadlets: number;
	secrets: number;
	access: number;
}

// The machine-scoped tab endpoints as MSW handlers. With `counts`, each
// handler records its hit; without, it only answers — for suites that
// need the endpoints answered but assert no request counts.
function machineTabHandlers(
	machine: Machine = machineFixture(),
	counts?: TabCounts,
) {
	const counted =
		<R,>(key: keyof TabCounts, respond: () => R) =>
		() => {
			if (counts) counts[key] += 1;
			return respond();
		};
	return [
		http.get(
			"*/api/machines/m-1",
			counted("machine", () => HttpResponse.json(machine)),
		),
		http.get(
			"*/api/machines/m-1/containers",
			counted("containers", emptyContainersResolver),
		),
		http.get(
			"*/api/machines/m-1/pods",
			counted("pods", () =>
				HttpResponse.json({
					pods: [],
					total: 0,
					timestamp: "2026-10-24T12:00:00Z",
				}),
			),
		),
		http.get(
			"*/api/machines/m-1/volumes",
			counted("volumes", () => HttpResponse.json([])),
		),
		http.get(
			"*/api/machines/m-1/images",
			counted("images", () => HttpResponse.json([])),
		),
		http.get(
			"*/api/machines/m-1/networks",
			counted("networks", emptyNetworksResolver),
		),
		http.get(
			"*/api/machines/m-1/quadlets",
			counted("quadlets", emptyQuadletsResolver),
		),
		http.get(
			"*/api/machines/m-1/secrets",
			counted("secrets", emptySecretsResolver),
		),
		http.get(
			"*/api/machines/m-1/access",
			counted("access", () => HttpResponse.json([])),
		),
	];
}

// MSW request counting shared by the tab-behavior suites (lazy
// mounting, refetch on activation, deep-linked tabs): every
// machine-scoped endpoint records how often it was hit, so a suite
// can assert which endpoints fired at which moment.
function countingTabHandlers(machine: Machine = machineFixture()) {
	const counts: TabCounts = {
		machine: 0,
		containers: 0,
		pods: 0,
		volumes: 0,
		images: 0,
		networks: 0,
		quadlets: 0,
		secrets: 0,
		access: 0,
	};
	server.use(...machineTabHandlers(machine, counts));
	return counts;
}

describe("MachineDetailPage", () => {
	it("navigates from the machines list into the detail page via the card click", async () => {
		server.use(
			http.get("*/api/machines", () => HttpResponse.json([machineFixture()])),
		);
		server.use(
			machineHandler(machineFixture()),
			emptyContainersHandler,
			emptyQuadletsHandler,
		);
		renderApp("/machines", {
			resolveInitialState: async () => ({ username: "alice", role: "user" }),
		});

		await userEvent.click(await screen.findByTestId("machine-card"));

		expect(await screen.findByText("prod-web-01")).toBeInTheDocument();
		expect(screen.getByText("web01.example.com:22")).toBeInTheDocument();
	});
	it("shows a loading state before the machine arrives", async () => {
		server.use(
			http.get("*/api/machines/m-1", async () => {
				await new Promise((resolve) => setTimeout(resolve, 50));
				return HttpResponse.json(machineFixture());
			}),
		);
		renderMachineDetailPage();

		expect(await screen.findByTestId("machine-loading")).toBeInTheDocument();
	});

	it("renders machine identity with permission badge", async () => {
		server.use(
			machineHandler(machineFixture()),
			emptyContainersHandler,
			emptyQuadletsHandler,
		);
		renderMachineDetailPage();

		expect(await screen.findByText("prod-web-01")).toBeInTheDocument();
		expect(screen.getByTestId("machine-endpoint")).toHaveTextContent(
			"web01.example.com:22",
		);
		expect(screen.getByTestId("machine-ssh-user")).toHaveTextContent("deploy");
		expect(screen.getByTestId("machine-permission")).toHaveTextContent("admin");
	});

	it("switches between the Containers, Pods, Images, Networks, Quadlets, Secrets, and Settings tabs", async () => {
		server.use(
			machineHandler(machineFixture()),
			emptyContainersHandler,
			emptyQuadletsHandler,
			emptySecretsHandler,
			emptyNetworksHandler,
		);
		server.use(
			http.get("*/api/machines/m-1/images", () => HttpResponse.json([])),
			http.get("*/api/machines/m-1/pods", () =>
				HttpResponse.json({
					pods: [],
					total: 0,
					timestamp: "2026-10-24T12:00:00Z",
				}),
			),
		);
		renderMachineDetailPage();

		await screen.findByText("prod-web-01");
		// The Pods tab sits after Quadlets, before Secrets (spec 352).
		const tabNames = screen.getAllByRole("tab").map((tab) => tab.textContent);
		expect(tabNames[0]).toBe("Containers");
		expect(tabNames[5]).toBe("Pods");
		expect(tabNames[6]).toBe("Secrets");
		expect(await screen.findByTestId("containers-empty")).toBeInTheDocument();
		expect(
			screen.queryByTestId("tab-containers-placeholder"),
		).not.toBeInTheDocument();

		await userEvent.click(screen.getByRole("tab", { name: /pods/i }));
		expect(await screen.findByTestId("pods-empty")).toBeInTheDocument();

		await userEvent.click(screen.getByRole("tab", { name: /images/i }));
		expect(await screen.findByTestId("images-empty")).toBeInTheDocument();

		await userEvent.click(screen.getByRole("tab", { name: /networks/i }));
		expect(await screen.findByTestId("networks-empty")).toBeInTheDocument();

		await userEvent.click(screen.getByRole("tab", { name: /quadlets/i }));
		expect(await screen.findByTestId("quadlets-empty")).toBeInTheDocument();
		expect(
			screen.queryByTestId("tab-quadlets-placeholder"),
		).not.toBeInTheDocument();

		await userEvent.click(screen.getByRole("tab", { name: /secrets/i }));
		expect(await screen.findByTestId("secrets-empty")).toBeInTheDocument();

		await userEvent.click(screen.getByRole("tab", { name: /settings/i }));
		expect(screen.getByTestId("machine-form-submit")).toBeInTheDocument();
	});

	it("renders the container list inside the Containers tab", async () => {
		server.use(machineHandler(machineFixture()), emptyQuadletsHandler);
		server.use(
			http.get("*/api/machines/m-1/containers", () =>
				HttpResponse.json({
					containers: [
						{
							id: "abc123def4567890",
							names: ["/prod-web"],
							image: "docker.io/library/nginx:1.27",
							createdAt: "2026-08-20T10:00:00Z",
							state: "running",
							status: "Up 2 hours",
						},
					],
					total: 1,
					timestamp: "2026-08-26T00:00:00Z",
				}),
			),
		);
		renderMachineDetailPage();

		expect(await screen.findByText("prod-web")).toBeInTheDocument();
		expect(
			screen.getByText("docker.io/library/nginx:1.27"),
		).toBeInTheDocument();
		expect(screen.getByTestId("state-badge")).toHaveTextContent("running");
	});

	it("renders the image list inside the Images tab", async () => {
		server.use(
			machineHandler(machineFixture()),
			emptyContainersHandler,
			emptyQuadletsHandler,
		);
		server.use(
			http.get("*/api/machines/m-1/images", () =>
				HttpResponse.json([
					{
						id: "sha256:abc123def4567890abc123def4567890abc123def4567890abc123def4567890",
						repoTags: ["docker.io/library/nginx:1.27"],
						repoDigests: [],
						createdAt: "2026-08-18T10:00:00Z",
						size: 157286400,
					},
				]),
			),
		);
		renderMachineDetailPage();

		await userEvent.click(await screen.findByRole("tab", { name: /images/i }));
		expect(
			await screen.findByText("docker.io/library/nginx:1.27"),
		).toBeInTheDocument();
		expect(screen.getByText("157.3 MB")).toBeInTheDocument();
	});

	it("renders the network list inside the Networks tab", async () => {
		server.use(
			machineHandler(machineFixture()),
			emptyContainersHandler,
			emptyQuadletsHandler,
		);
		server.use(
			http.get("*/api/machines/m-1/networks", () =>
				HttpResponse.json([
					{
						name: "app-net",
						driver: "bridge",
						subnets: ["10.88.0.0/16"],
						createdAt: "2026-08-20T10:00:00Z",
					},
				]),
			),
		);
		renderMachineDetailPage();

		await userEvent.click(
			await screen.findByRole("tab", { name: /networks/i }),
		);
		expect(await screen.findByText("app-net")).toBeInTheDocument();
		expect(screen.getByText("bridge")).toBeInTheDocument();
		expect(screen.getByText("10.88.0.0/16")).toBeInTheDocument();
	});

	it("renders the quadlet list inside the Quadlets tab", async () => {
		server.use(machineHandler(machineFixture()), emptyContainersHandler);
		server.use(
			http.get("*/api/machines/m-1/quadlets", () =>
				HttpResponse.json([
					{
						id: "web",
						name: "web",
						path: "/etc/containers/systemd/web.container",
						unit: "web.service",
						resource: "container",
						state: "running",
					},
				]),
			),
		);
		renderMachineDetailPage();

		await userEvent.click(
			await screen.findByRole("tab", { name: /quadlets/i }),
		);
		expect(await screen.findByText("web")).toBeInTheDocument();
		expect(screen.getByTestId("state-badge")).toHaveTextContent("running");
	});

	it("renders the secret list inside the Secrets tab", async () => {
		server.use(
			machineHandler(machineFixture()),
			emptyContainersHandler,
			emptyQuadletsHandler,
		);
		server.use(
			http.get("*/api/machines/m-1/secrets", () =>
				HttpResponse.json([
					{
						id: "3f2e1d0c",
						name: "db-password",
						driver: "file",
						createdAt: "2026-08-20T10:00:00Z",
						updatedAt: "2026-08-20T10:00:00Z",
					},
				]),
			),
		);
		renderMachineDetailPage();

		await userEvent.click(await screen.findByRole("tab", { name: /secrets/i }));
		expect(await screen.findByText("db-password")).toBeInTheDocument();
		expect(screen.getByText("file")).toBeInTheDocument();
	});

	it("renders the error envelope for an unknown machine id", async () => {
		server.use(
			http.get("*/api/machines/m-missing", () =>
				errorResponse(404, "MACHINE_NOT_FOUND", "Machine not found", "req-9"),
			),
		);
		renderMachineDetailPage("m-missing");

		expect(await screen.findByRole("alert")).toBeInTheDocument();
		expect(screen.getByText("MACHINE_NOT_FOUND")).toBeInTheDocument();
		expect(screen.getByText("Machine not found")).toBeInTheDocument();
	});
});

describe("page-level permission gating on the machine detail page", () => {
	it("gates from the detail response's own per-caller facts, never the machines list", async () => {
		// The list answers owner-admin for the same machine while the
		// detail answers read-grantee. Only the detail facts may drive
		// the page: the list query is not part of the detail page's data.
		server.use(
			http.get("*/api/machines", () =>
				HttpResponse.json([
					machineFixture({ permission: "admin", isOwner: true }),
				]),
			),
			http.get("*/api/machines/m-1", () =>
				HttpResponse.json(
					machineFixture({ permission: "read", isOwner: false }),
				),
			),
			http.get("*/api/machines/m-1/secrets", () => HttpResponse.json([])),
		);
		renderApp("/machines/m-1", {
			resolveInitialState: async () => ({ username: "alice", role: "user" }),
		});
		await screen.findByText(machineFixture().name);
		expect(screen.getByTestId("machine-permission")).toHaveTextContent("read");

		await userEvent.click(screen.getByRole("tab", { name: /secrets/i }));
		await screen.findByTestId("secrets-permission-hint");
		expect(
			screen.queryByTestId("create-secret-button"),
		).not.toBeInTheDocument();
	});
});

describe("lazy tab mounting", () => {
	it("issues only the machine query and the default tab's list when the page opens", async () => {
		const counts = countingTabHandlers();
		renderMachineDetailPage();

		expect(await screen.findByTestId("containers-empty")).toBeInTheDocument();
		expect(counts.machine).toBe(1);
		expect(counts.containers).toBe(1);
		expect(counts.volumes).toBe(0);
		expect(counts.images).toBe(0);
		expect(counts.networks).toBe(0);
		expect(counts.quadlets).toBe(0);
		expect(counts.secrets).toBe(0);
		expect(counts.access).toBe(0);
	});

	it("mounts a tab's panel and issues its endpoint on first activation", async () => {
		const counts = countingTabHandlers();
		renderMachineDetailPage();

		expect(await screen.findByTestId("containers-empty")).toBeInTheDocument();
		expect(counts.volumes).toBe(0);

		await userEvent.click(screen.getByRole("tab", { name: /volumes/i }));
		expect(await screen.findByTestId("volumes-empty")).toBeInTheDocument();
		expect(counts.volumes).toBe(1);
	});

	it("unmounts a tab's panel on leave and resets its sort state on re-entry", async () => {
		server.use(
			machineHandler(machineFixture()),
			http.get("*/api/machines/m-1/containers", () =>
				HttpResponse.json({
					containers: [
						containerFixture({ id: "c-zeta", names: ["/zeta"] }),
						containerFixture({ id: "c-alpha", names: ["/alpha"] }),
					],
					total: 2,
					timestamp: "2026-08-26T00:00:00Z",
				}),
			),
			http.get("*/api/machines/m-1/images", () => HttpResponse.json([])),
		);
		renderMachineDetailPage();

		// Server order: zeta first, no sort applied.
		let rows = await screen.findAllByTestId("container-row");
		expect(rows[0]).toHaveTextContent("zeta");
		const nameHeader = screen.getByTestId("sort-container-0");
		expect(nameHeader).not.toHaveAttribute("aria-sort");

		// First click on the Name header sorts ascending: alpha first.
		await userEvent.click(nameHeader);
		expect(nameHeader).toHaveAttribute("aria-sort", "ascending");
		rows = screen.getAllByTestId("container-row");
		expect(rows[0]).toHaveTextContent("alpha");

		// Leaving the tab unmounts the panel: the list leaves the DOM.
		await userEvent.click(screen.getByRole("tab", { name: /images/i }));
		expect(await screen.findByTestId("images-empty")).toBeInTheDocument();
		expect(screen.queryByTestId("containers-table")).not.toBeInTheDocument();

		// Re-entry remounts the panel with the sort state reset: no
		// aria-sort, the server's natural order restored.
		await userEvent.click(screen.getByRole("tab", { name: /containers/i }));
		rows = await screen.findAllByTestId("container-row");
		expect(rows[0]).toHaveTextContent("zeta");
		expect(screen.getByTestId("sort-container-0")).not.toHaveAttribute(
			"aria-sort",
		);
	});

	it("fires the access-grants request only for owners, only when the Settings tab is activated", async () => {
		const counts = countingTabHandlers();
		server.use(http.get("*/api/users", () => HttpResponse.json([])));
		renderMachineDetailPage();

		expect(await screen.findByTestId("containers-empty")).toBeInTheDocument();
		expect(counts.access).toBe(0);

		await userEvent.click(screen.getByRole("tab", { name: /settings/i }));
		expect(
			await screen.findByTestId("machine-form-submit"),
		).toBeInTheDocument();
		await waitFor(() => expect(counts.access).toBe(1));
	});

	it("gates the Settings tab to the permission hint for non-owners without firing access grants", async () => {
		const counts = countingTabHandlers(
			machineFixture({ permission: "read", isOwner: false }),
		);
		renderMachineDetailPage();

		expect(await screen.findByText("prod-web-01")).toBeInTheDocument();
		expect(counts.access).toBe(0);

		await userEvent.click(screen.getByRole("tab", { name: /settings/i }));
		expect(
			await screen.findByTestId("settings-permission-hint"),
		).toBeInTheDocument();
		expect(screen.queryByTestId("machine-form-submit")).not.toBeInTheDocument();
		expect(counts.access).toBe(0);
	});
});

describe("refetch on tab activation", () => {
	// The app-wide stale time is five minutes (main.tsx). The client here
	// mirrors it, so these tests pin the refetch inside the cache window:
	// with the harness default (staleTime 0) a remount refetches anyway and
	// the assertions could not fail.
	function renderMachineDetailPageWithAppStaleTime() {
		return renderApp("/machines/m-1", {
			resolveInitialState: async () => ({ username: "alice", role: "user" }),
			queryClient: new QueryClient({
				defaultOptions: {
					queries: { retry: false, staleTime: 5 * 60 * 1000 },
				},
			}),
		});
	}

	it("refetches the default tab's list when returning to it", async () => {
		const counts = countingTabHandlers();
		renderMachineDetailPageWithAppStaleTime();

		expect(await screen.findByTestId("containers-empty")).toBeInTheDocument();
		expect(counts.containers).toBe(1);

		await userEvent.click(screen.getByRole("tab", { name: /images/i }));
		expect(await screen.findByTestId("images-empty")).toBeInTheDocument();

		await userEvent.click(screen.getByRole("tab", { name: /containers/i }));
		expect(await screen.findByTestId("containers-empty")).toBeInTheDocument();
		await waitFor(() => expect(counts.containers).toBe(2));
	});

	it.each(["volumes", "images", "networks", "quadlets", "secrets"] as const)(
		"issues the %s endpoint request on every activation of its tab",
		async (endpoint) => {
			const counts = countingTabHandlers();
			renderMachineDetailPageWithAppStaleTime();

			expect(await screen.findByTestId("containers-empty")).toBeInTheDocument();

			await userEvent.click(
				screen.getByRole("tab", { name: new RegExp(endpoint, "i") }),
			);
			expect(
				await screen.findByTestId(`${endpoint}-empty`),
			).toBeInTheDocument();
			expect(counts[endpoint]).toBe(1);

			// Leaving unmounts the panel; re-entry remounts it and the
			// endpoint fires again inside the five-minute cache window.
			await userEvent.click(screen.getByRole("tab", { name: /containers/i }));
			expect(await screen.findByTestId("containers-empty")).toBeInTheDocument();

			await userEvent.click(
				screen.getByRole("tab", { name: new RegExp(endpoint, "i") }),
			);
			expect(
				await screen.findByTestId(`${endpoint}-empty`),
			).toBeInTheDocument();
			await waitFor(() => expect(counts[endpoint]).toBe(2));
		},
	);

	it("refetches the settings tab's access grants on every activation in the owner view", async () => {
		const counts = countingTabHandlers();
		server.use(http.get("*/api/users", () => HttpResponse.json([])));
		renderMachineDetailPageWithAppStaleTime();

		expect(await screen.findByTestId("containers-empty")).toBeInTheDocument();

		await userEvent.click(screen.getByRole("tab", { name: /settings/i }));
		expect(
			await screen.findByTestId("machine-form-submit"),
		).toBeInTheDocument();
		await waitFor(() => expect(counts.access).toBe(1));

		await userEvent.click(screen.getByRole("tab", { name: /containers/i }));
		expect(await screen.findByTestId("containers-empty")).toBeInTheDocument();

		await userEvent.click(screen.getByRole("tab", { name: /settings/i }));
		expect(
			await screen.findByTestId("machine-form-submit"),
		).toBeInTheDocument();
		await waitFor(() => expect(counts.access).toBe(2));
	});
});

describe("deep-linked tab via ?tab=", () => {
	function renderMachineDetailPageWithTab(tab?: string) {
		return renderApp(`/machines/m-1${tab === undefined ? "" : `?tab=${tab}`}`, {
			resolveInitialState: async () => ({ username: "alice", role: "user" }),
		});
	}

	it.each([
		["containers", "Containers", "containers-empty"],
		["pods", "Pods", "pods-empty"],
		["volumes", "Volumes", "volumes-empty"],
		["images", "Images", "images-empty"],
		["networks", "Networks", "networks-empty"],
		["quadlets", "Quadlets", "quadlets-empty"],
		["secrets", "Secrets", "secrets-empty"],
		["settings", "Settings", "machine-form-submit"],
	] as const)(
		"selects the %s tab and renders its panel when opened with ?tab=%s",
		async (key, tabName, panelTestId) => {
			server.use(...machineTabHandlers());
			server.use(http.get("*/api/users", () => HttpResponse.json([])));
			renderMachineDetailPageWithTab(key);

			expect(await screen.findByTestId(panelTestId)).toBeInTheDocument();
			expect(screen.getByRole("tab", { name: tabName })).toHaveAttribute(
				"aria-selected",
				"true",
			);
		},
	);

	it("fires only the deep-linked tab's endpoint on arrival", async () => {
		const counts = countingTabHandlers();
		renderMachineDetailPageWithTab("images");

		expect(await screen.findByTestId("images-empty")).toBeInTheDocument();
		expect(screen.queryByTestId("containers-empty")).not.toBeInTheDocument();
		expect(counts.machine).toBe(1);
		expect(counts.images).toBe(1);
		expect(counts.containers).toBe(0);
		expect(counts.pods).toBe(0);
		expect(counts.volumes).toBe(0);
		expect(counts.networks).toBe(0);
		expect(counts.quadlets).toBe(0);
		expect(counts.secrets).toBe(0);
		expect(counts.access).toBe(0);
	});

	it("falls back to the Containers tab for an unknown tab value", async () => {
		const counts = countingTabHandlers();
		renderMachineDetailPageWithTab("bogus");

		expect(await screen.findByTestId("containers-empty")).toBeInTheDocument();
		expect(screen.getByRole("tab", { name: "Containers" })).toHaveAttribute(
			"aria-selected",
			"true",
		);
		expect(counts.containers).toBe(1);
		expect(counts.images).toBe(0);
	});

	it("selects the Containers tab when the parameter is missing", async () => {
		server.use(...machineTabHandlers());
		renderMachineDetailPageWithTab();

		expect(await screen.findByTestId("containers-empty")).toBeInTheDocument();
		expect(screen.getByRole("tab", { name: "Containers" })).toHaveAttribute(
			"aria-selected",
			"true",
		);
	});
});

// jsdom cannot prove real 360px layout (spec 393's testing decision),
// so the small-viewport behavior is pinned through the responsive
// props Chakra compiles into the element's emotion CSS — see
// breakpointRules over cssRulesFor. Chakra's lg breakpoint (992px) is
// the spec's "desktop unchanged" line.
describe("small-viewport layout (spec 393)", () => {
	it("scrolls the tab strip horizontally below the breakpoint and restores the desktop strip at lg", async () => {
		server.use(machineHandler(machineFixture()), emptyContainersHandler);
		renderMachineDetailPage();
		await screen.findByText("prod-web-01");

		const { base, lg } = breakpointRules(screen.getByRole("tablist"));
		// Base: the eight-tab strip scrolls sideways instead of
		// overflowing the viewport.
		expect(base).toContain("overflow-x:auto");
		expect(base).toContain("overflow-y:hidden");
		// lg and up: the strip renders as on desktop today.
		expect(lg).toContain("overflow-x:visible");
		expect(lg).toContain("overflow-y:visible");
	});

	it("wraps the machine header rows at base and keeps them inline at lg", async () => {
		server.use(machineHandler(machineFixture()), emptyContainersHandler);
		renderMachineDetailPage();
		await screen.findByText("prod-web-01");

		const nameRow = breakpointRules(
			screen.getByTestId("machine-permission").parentElement!,
		);
		expect(nameRow.base).toContain("flex-wrap:wrap");
		expect(nameRow.lg).toContain("flex-wrap:nowrap");

		const endpointRow = breakpointRules(
			screen.getByTestId("machine-endpoint").parentElement!,
		);
		expect(endpointRow.base).toContain("flex-wrap:wrap");
		expect(endpointRow.lg).toContain("flex-wrap:nowrap");

		// The mono endpoint is a single unbreakable token; at base it
		// may break mid-token instead of clipping.
		const endpoint = breakpointRules(screen.getByTestId("machine-endpoint"));
		expect(endpoint.base).toContain("overflow-wrap:anywhere");
		expect(endpoint.lg).toContain("overflow-wrap:normal");
	});
});
