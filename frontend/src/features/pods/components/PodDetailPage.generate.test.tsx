import { PodDetailPage } from "@features/pods/components/PodDetailPage";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { Route, Routes } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";
import { errorResponse } from "@/test/errors";
import { machineFixture, podDetailFixture } from "@/test/factories";
import { renderWithQuery } from "@/test/render";
import { server } from "@/test/server";

const generatedContent = "[Pod]\nName=app-pod\nPublishPort=8080:80\n";

// The page reads the machine's per-caller facts for the save gate; the
// generate and copy actions stay read-level.
function mockMachine(permission: "read" | "write" | "admin" = "admin") {
	server.use(
		http.get("*/api/machines/m-1", () =>
			HttpResponse.json(
				machineFixture({ permission, isOwner: permission === "admin" }),
			),
		),
	);
}

function mockPodDetail() {
	server.use(
		http.get("*/api/machines/m-1/pods/p-1", () =>
			HttpResponse.json(podDetailFixture()),
		),
		http.get("*/api/machines/m-1/pods/p-1/inspect", () =>
			HttpResponse.json({ Id: "p-1" }),
		),
	);
}

function renderPodDetailPage() {
	return renderWithQuery(
		<Routes>
			<Route
				path="/machines/:machineId/pods/:podId"
				element={<PodDetailPage />}
			/>
		</Routes>,
		{ router: { initialEntries: ["/machines/m-1/pods/p-1"] } },
	);
}

function generateHandler() {
	return http.post("*/api/machines/m-1/quadlets/p-1/generate", () =>
		HttpResponse.json({
			id: "app-pod",
			name: "app-pod",
			resource: "pod",
			state: "unknown",
			content: generatedContent,
		}),
	);
}

// The generate-quadlet flow (spec #402 ticket #406): the detail page's
// button opens a feature-owned modal that runs the quadlet generate
// endpoint for this pod and shows the generated .pod text, with copy and
// save-to-machine actions. The modal's phase machinery follows the
// ImportComposeModal precedent; this suite keeps the pod-domain assertions.
describe("PodDetailPage generate quadlet", () => {
	it("opens the modal on the Generate quadlet button and shows the generated .pod text", async () => {
		const generateBodies: unknown[] = [];
		mockPodDetail();
		server.use(
			http.post(
				"*/api/machines/m-1/quadlets/p-1/generate",
				async ({ request }) => {
					generateBodies.push(await request.json());
					return HttpResponse.json({
						id: "app-pod",
						name: "app-pod",
						resource: "pod",
						state: "unknown",
						content: generatedContent,
					});
				},
			),
		);
		renderPodDetailPage();
		const user = userEvent.setup();

		await screen.findByTestId("pod-detail-name");
		await user.click(screen.getByTestId("generate-quadlet-button"));

		expect(await screen.findByTestId("modal")).toBeInTheDocument();
		const content = await screen.findByTestId("generated-quadlet-content");
		expect(content).toHaveTextContent("[Pod]");
		expect(content).toHaveTextContent("PublishPort=8080:80");
		expect(generateBodies).toEqual([
			{ resourceId: "p-1", resourceType: "pod" },
		]);
	});

	it("copies the generated .pod text to the clipboard", async () => {
		mockPodDetail();
		server.use(generateHandler());
		renderPodDetailPage();
		const user = userEvent.setup();
		// userEvent.setup() installs its own clipboard stub, so the spy goes
		// on after it to be the one the button's click handler reads.
		const writeText = vi.fn().mockResolvedValue(undefined);
		Object.defineProperty(navigator, "clipboard", {
			value: { writeText },
			configurable: true,
		});

		await screen.findByTestId("pod-detail-name");
		await user.click(screen.getByTestId("generate-quadlet-button"));
		await screen.findByTestId("generated-quadlet-content");

		await user.click(screen.getByTestId("copy-generated-quadlet-button"));

		expect(writeText).toHaveBeenCalledTimes(1);
		expect(writeText).toHaveBeenCalledWith(generatedContent);
	});

	it("saves the generated quadlet to the machine through the quadlet create endpoint", async () => {
		const createBodies: unknown[] = [];
		mockMachine();
		mockPodDetail();
		server.use(
			generateHandler(),
			http.post("*/api/machines/m-1/quadlets", async ({ request }) => {
				createBodies.push(await request.json());
				return HttpResponse.json(
					{
						id: "app-pod",
						name: "app-pod",
						resource: "pod",
						state: "unknown",
						content: generatedContent,
					},
					{ status: 201 },
				);
			}),
		);
		renderPodDetailPage();
		const user = userEvent.setup();

		await screen.findByTestId("pod-detail-name");
		await user.click(screen.getByTestId("generate-quadlet-button"));
		await screen.findByTestId("generated-quadlet-content");
		await user.click(screen.getByTestId("save-generated-quadlet-button"));

		expect(
			await screen.findByTestId("generated-quadlet-saved"),
		).toBeInTheDocument();
		await waitFor(() => expect(createBodies).toHaveLength(1));
		// The generate response forwards to the create endpoint verbatim:
		// the pod's name, its resource type, and the generated content.
		expect(createBodies[0]).toEqual({
			name: "app-pod",
			resource: "pod",
			content: generatedContent,
		});
	});

	it("renders a generate failure in the modal with no generated text", async () => {
		mockPodDetail();
		server.use(
			http.post("*/api/machines/m-1/quadlets/p-1/generate", () =>
				errorResponse(404, "POD_NOT_FOUND", "pod not found"),
			),
		);
		renderPodDetailPage();
		const user = userEvent.setup();

		await screen.findByTestId("pod-detail-name");
		await user.click(screen.getByTestId("generate-quadlet-button"));

		const error = await screen.findByTestId("generate-quadlet-error");
		expect(error).toHaveTextContent("POD_NOT_FOUND");
		expect(error).toHaveTextContent("pod not found");
		expect(
			screen.queryByTestId("generated-quadlet-content"),
		).not.toBeInTheDocument();
	});

	it("renders a save failure in the modal and keeps the generated text for a retry", async () => {
		mockMachine();
		mockPodDetail();
		server.use(
			generateHandler(),
			http.post("*/api/machines/m-1/quadlets", () =>
				errorResponse(409, "QUADLET_ALREADY_EXISTS", "Quadlet already exists"),
			),
		);
		renderPodDetailPage();
		const user = userEvent.setup();

		await screen.findByTestId("pod-detail-name");
		await user.click(screen.getByTestId("generate-quadlet-button"));
		await screen.findByTestId("generated-quadlet-content");
		await user.click(screen.getByTestId("save-generated-quadlet-button"));

		const error = await screen.findByTestId("generate-quadlet-save-error");
		expect(error).toHaveTextContent("QUADLET_ALREADY_EXISTS");
		expect(error).toHaveTextContent("Quadlet already exists");
		// Still in the text phase: the content renders and save can retry.
		expect(screen.getByTestId("generated-quadlet-content")).toBeInTheDocument();
		expect(screen.getByTestId("save-generated-quadlet-button")).toBeEnabled();
	});

	it("hides the Save to machine action below the manage-quadlets capability, keeping Generate and Copy", async () => {
		mockMachine("read");
		mockPodDetail();
		server.use(generateHandler());
		renderPodDetailPage();
		const user = userEvent.setup();

		await screen.findByTestId("pod-detail-name");
		await user.click(screen.getByTestId("generate-quadlet-button"));
		await screen.findByTestId("generated-quadlet-content");

		// Generate is read-level, so the text and the copy action stay;
		// only the write-gated save hides (never a disabled button).
		expect(
			screen.getByTestId("copy-generated-quadlet-button"),
		).toBeInTheDocument();
		expect(
			screen.queryByTestId("save-generated-quadlet-button"),
		).not.toBeInTheDocument();
	});
});
