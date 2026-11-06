import {
	openCreatePage,
	setupCreatePage,
	typeImage,
} from "@features/containers/components/createContainerTestHarness";
import { screen, waitFor, within } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import { beforeEach, describe, expect, it } from "vitest";
import { volumeFixture } from "@/test/factories";
import { server } from "@/test/server";

// Inline volume create on the create-container page (spec 341, ticket
// #347): the "Create volume" button next to the named-volume rows opens
// the existing create-volume dialog; success refreshes the volumes query
// (the create mutation invalidates the host's volume list) and lands
// the new volume's name in a volume row's dropdown. The seam is the
// rendered page: the button, the dialog, and the row dropdowns.

// The harness's option-list handler serves a fixed volume list; these
// overrides give the volumes endpoint a mutable list so the post-create
// refetch serves the new volume, the way the real host does.
let volumeListRequests = 0;
let volumeCreateRequests = 0;
let volumes = [volumeFixture(), volumeFixture({ name: "pgdata" })];

function resetVolumeState() {
	volumeListRequests = 0;
	volumeCreateRequests = 0;
	volumes = [volumeFixture(), volumeFixture({ name: "pgdata" })];
}

function volumeHandlers() {
	return [
		http.get("*/api/hosts/m-1/volumes", () => {
			volumeListRequests += 1;
			return HttpResponse.json(volumes);
		}),
		http.post("*/api/hosts/m-1/volumes", async ({ request }) => {
			volumeCreateRequests += 1;
			const body = (await request.json()) as { name?: string };
			const created = volumeFixture({
				name: body.name?.trim() || "generated-name",
			});
			volumes = [...volumes, created];
			return HttpResponse.json(created, { status: 201 });
		}),
	];
}

async function openInlineVolumeDialog() {
	const user = await openCreatePage();
	await user.click(screen.getByTestId("create-volume-inline"));
	const modal = await screen.findByTestId("modal");
	expect(within(modal).getByTestId("volume-form-submit")).toBeInTheDocument();
	return user;
}

// Every test here boots the full create-container page and drives a
// userEvent chain through it; under full-suite parallel load they outgrow
// vitest's default 5s test timeout, so the file sets its own.
const TEST_TIMEOUT = 20_000;

describe("CreateContainerPage inline volume create", () => {
	beforeEach(() => {
		resetVolumeState();
		setupCreatePage();
		server.use(...volumeHandlers());
	});

	it(
		"renders a Create volume button next to the named-volume rows that opens the create-volume dialog",
		async () => {
			const user = await openCreatePage();

			const addButton = screen.getByTestId("add-named-volume");
			const createButton = screen.getByTestId("create-volume-inline");
			expect(createButton).toHaveTextContent("Create volume");
			expect(createButton.closest("div")).toBe(addButton.closest("div"));

			await user.click(createButton);
			const modal = await screen.findByTestId("modal");
			expect(
				within(modal).getByTestId("volume-form-submit"),
			).toBeInTheDocument();
		},
		TEST_TIMEOUT,
	);

	it(
		"on success refreshes the volumes query and selects the new volume in an added row",
		async () => {
			const user = await openInlineVolumeDialog();
			const listRequestsBefore = volumeListRequests;

			await user.type(
				within(screen.getByTestId("modal")).getByLabelText(/^name/i),
				"fresh",
			);
			await user.click(screen.getByTestId("volume-form-submit"));

			// The create mutation invalidates the host's volume list, so
			// the page's volumes query refetches through MSW.
			await waitFor(() => expect(volumeCreateRequests).toBe(1));
			await waitFor(() =>
				expect(volumeListRequests).toBeGreaterThan(listRequestsBefore),
			);

			// No volume row existed, so one was added with the new volume
			// selected — and the refetched options carry it.
			const volumeSelect = await screen.findByLabelText("Volume name 1");
			expect(volumeSelect).toHaveValue("fresh");
			expect(within(volumeSelect).getByText("fresh")).toBeInTheDocument();
			expect(screen.queryByLabelText("Volume name 2")).not.toBeInTheDocument();
		},
		TEST_TIMEOUT,
	);

	it(
		"on success selects the new volume into an existing empty row without adding another",
		async () => {
			const user = await openCreatePage();
			await user.click(screen.getByTestId("add-named-volume"));
			expect(screen.getByLabelText("Volume name 1")).toBeInTheDocument();

			await user.click(screen.getByTestId("create-volume-inline"));
			const modal = await screen.findByTestId("modal");
			await user.type(within(modal).getByLabelText(/^name/i), "fresh");
			await user.click(screen.getByTestId("volume-form-submit"));

			await waitFor(() => expect(volumeCreateRequests).toBe(1));
			expect(await screen.findByLabelText("Volume name 1")).toHaveValue(
				"fresh",
			);
			expect(screen.queryByLabelText("Volume name 2")).not.toBeInTheDocument();
		},
		TEST_TIMEOUT,
	);

	it(
		"cancelling the volume dialog leaves the create form untouched",
		async () => {
			const user = await openCreatePage();
			typeImage();
			await user.click(screen.getByTestId("add-named-volume"));

			await user.click(screen.getByTestId("create-volume-inline"));
			const modal = await screen.findByTestId("modal");
			await user.type(within(modal).getByLabelText(/^name/i), "fresh");
			await user.click(within(modal).getByRole("button", { name: "Cancel" }));

			await waitFor(() =>
				expect(screen.queryByTestId("modal")).not.toBeInTheDocument(),
			);
			expect(screen.getByLabelText(/image/i)).toHaveValue(
				"docker.io/library/redis:7",
			);
			// The pre-existing empty row keeps its placeholder selection; no
			// volume was created and no row was added or filled.
			expect(screen.getByLabelText("Volume name 1")).toHaveValue("");
			expect(screen.queryByLabelText("Volume name 2")).not.toBeInTheDocument();
			expect(volumeCreateRequests).toBe(0);
		},
		TEST_TIMEOUT,
	);
});
