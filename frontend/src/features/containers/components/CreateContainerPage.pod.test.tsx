import {
	createRequests,
	listRequests,
	openCreatePage,
	optionRequests,
	renderCreatePage,
	setupCreatePage,
	submitForm,
	typeImage,
	type User,
} from "@features/containers/components/createContainerTestHarness";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import { errorJson } from "@/test/errors";

// The create-container page's pod selector (spec 356): a single-select
// fed by the pods list query, the network section stepping aside while a
// pod is chosen, the pod riding the create request's pod field, the
// POD_NOT_FOUND envelope surfacing in the form's error region, and the
// pods list refetching after a create that joins a pod. The payload
// mapping contract itself is pinned without a DOM in
// services/containersApi.create.test.ts.

describe("CreateContainerPage pod selector", () => {
	it("offers the machine's pods in a single-select with No pod as the default", async () => {
		setupCreatePage();
		renderCreatePage();

		const select = await screen.findByLabelText("Pod");
		expect(select).toHaveValue("");
		expect(within(select).getByText("No pod")).toBeInTheDocument();
		expect(await within(select).findByText("app-pod")).toBeInTheDocument();
		expect(within(select).getByText("db-pod")).toBeInTheDocument();
	});

	it("disables the network checkboxes with the pod-owns-networking hint while a pod is selected", async () => {
		setupCreatePage();
		renderCreatePage();
		const user = userEvent.setup();
		await screen.findByLabelText("Pod");

		// No pod: the network checkboxes are live and no hint shows.
		expect(await screen.findByLabelText("app-net")).toBeEnabled();
		expect(
			screen.queryByTestId("container-pod-network-hint"),
		).not.toBeInTheDocument();

		await user.selectOptions(screen.getByLabelText("Pod"), "app-pod");

		expect(screen.getByTestId("container-pod-network-hint")).toHaveTextContent(
			/pod owns networking/i,
		);
		expect(screen.getByLabelText("app-net")).toBeDisabled();
		expect(screen.getByLabelText("other-net")).toBeDisabled();

		// Back to No pod: the section steps back in.
		await user.selectOptions(screen.getByLabelText("Pod"), "");
		expect(
			screen.queryByTestId("container-pod-network-hint"),
		).not.toBeInTheDocument();
		expect(screen.getByLabelText("app-net")).toBeEnabled();
	});

	it("sends the chosen pod in the create request and drops the network selection", async () => {
		setupCreatePage();
		const user: User = await openCreatePage();

		// A network checked before the pod is chosen still must not
		// travel: the pod owns networking.
		await user.click(await screen.findByLabelText("app-net"));
		await user.selectOptions(screen.getByLabelText("Pod"), "app-pod");
		await typeImage(user);
		await submitForm(user);

		expect(createRequests.body).toEqual({
			image: "docker.io/library/redis:7",
			pod: "app-pod",
		});
	});

	it("surfaces a POD_NOT_FOUND create failure in the form's error region", async () => {
		setupCreatePage(
			http.post("*/api/machines/m-1/containers", () =>
				HttpResponse.json(errorJson("POD_NOT_FOUND", "pod not found"), {
					status: 404,
				}),
			),
		);
		renderCreatePage();
		const user = userEvent.setup();

		await typeImage(user);
		await user.selectOptions(screen.getByLabelText("Pod"), "app-pod");
		await user.click(screen.getByTestId("container-form-submit"));

		const errorRegion = await screen.findByTestId("container-form-error");
		await waitFor(() => expect(errorRegion).toHaveTextContent("POD_NOT_FOUND"));
		expect(errorRegion).toHaveTextContent("pod not found");
	});

	it("refetches the pods list after a create that joins a pod", async () => {
		setupCreatePage();
		renderCreatePage();

		// The selector's own fetch on mount.
		await waitFor(() => expect(optionRequests.pods).toBe(1));
		const user = userEvent.setup();
		await typeImage(user);
		await user.selectOptions(screen.getByLabelText("Pod"), "app-pod");
		await submitForm(user);

		// The create joined a pod, so the pods list invalidates and
		// refetches alongside the containers list.
		await waitFor(() => expect(optionRequests.pods).toBe(2));
	});

	it("leaves the pods list alone when the create joins no pod", async () => {
		setupCreatePage();
		renderCreatePage();

		await waitFor(() => expect(optionRequests.pods).toBe(1));
		const user = userEvent.setup();
		await typeImage(user);
		await submitForm(user);

		// The navigate-back lands on the containers tab, whose list
		// refetches; the pods list, untouched by a podless create, does
		// not refetch again.
		await waitFor(() => expect(listRequests.count).toBe(1));
		expect(optionRequests.pods).toBe(1);
	});
});
