import { Button, Text } from "@chakra-ui/react";
import {
	ConfirmDialog,
	MutationDialog,
	type MutationSlice,
} from "@shared/components/MutationDialog";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { AxiosError, type AxiosResponse } from "axios";
import { describe, expect, it, vi } from "vitest";

// The module drives the mutation result the caller hands it, never
// react-query itself: the stub below is the whole interface under test.
function makeMutation(
	overrides: Partial<MutationSlice<string>> = {},
): MutationSlice<string> {
	return {
		mutate: vi.fn(),
		isPending: false,
		error: null,
		isSuccess: false,
		reset: vi.fn(),
		...overrides,
	};
}

function envelopeError() {
	return new AxiosError(
		"Request failed with status code 409",
		AxiosError.ERR_BAD_REQUEST,
		undefined,
		undefined,
		{
			status: 409,
			data: {
				requestId: "req-9",
				error: { code: "GADGET_IN_USE", message: "Gadget is in use" },
			},
		} as unknown as AxiosResponse,
	);
}

function stubBody() {
	return <Text data-testid="stub-body">Stub body</Text>;
}

describe("MutationDialog lifecycle", () => {
	it("renders the always-open shell with title and body, resetting the mutation on mount", () => {
		const mutation = makeMutation();
		const onClose = vi.fn();

		render(
			<MutationDialog
				mutation={mutation}
				title="Delete gadget"
				onClose={onClose}
				body={stubBody()}
			/>,
		);

		expect(screen.getByTestId("modal")).toBeInTheDocument();
		expect(screen.getByText("Delete gadget")).toBeInTheDocument();
		expect(screen.getByTestId("stub-body")).toBeInTheDocument();
		expect(mutation.reset).toHaveBeenCalledTimes(1);
		expect(mutation.reset).toHaveBeenCalledWith();
		expect(mutation.mutate).not.toHaveBeenCalled();
	});

	it("resets the mutation again on unmount", () => {
		const mutation = makeMutation();

		const view = render(
			<MutationDialog
				mutation={mutation}
				title="Delete gadget"
				onClose={vi.fn()}
				body={stubBody()}
			/>,
		);
		expect(mutation.reset).toHaveBeenCalledTimes(1);
		view.unmount();

		expect(mutation.reset).toHaveBeenCalledTimes(2);
	});
});

describe("MutationDialog close-on-success", () => {
	it("fires onSuccess then onClose exactly once when the mutation succeeds, staying closed on further renders", () => {
		const mutation = makeMutation();
		const calls: string[] = [];
		const onSuccess = vi.fn(() => calls.push("success"));
		const onClose = vi.fn(() => calls.push("close"));

		const view = render(
			<MutationDialog
				mutation={mutation}
				title="Delete gadget"
				onClose={onClose}
				body={stubBody()}
				onSuccess={onSuccess}
			/>,
		);
		view.rerender(
			<MutationDialog
				mutation={{ ...mutation, isSuccess: true }}
				title="Delete gadget"
				onClose={onClose}
				body={stubBody()}
				onSuccess={onSuccess}
			/>,
		);

		expect(calls).toEqual(["success", "close"]);
		expect(onSuccess).toHaveBeenCalledTimes(1);
		expect(onClose).toHaveBeenCalledTimes(1);

		view.rerender(
			<MutationDialog
				mutation={{ ...mutation, isSuccess: true }}
				title="Delete gadget"
				onClose={onClose}
				body={stubBody()}
				onSuccess={onSuccess}
			/>,
		);

		expect(onSuccess).toHaveBeenCalledTimes(1);
		expect(onClose).toHaveBeenCalledTimes(1);
	});

	it("closes on success even without an onSuccess seam", () => {
		const mutation = makeMutation();
		const onClose = vi.fn();

		const view = render(
			<MutationDialog
				mutation={mutation}
				title="Delete gadget"
				onClose={onClose}
				body={stubBody()}
			/>,
		);
		view.rerender(
			<MutationDialog
				mutation={{ ...mutation, isSuccess: true }}
				title="Delete gadget"
				onClose={onClose}
				body={stubBody()}
			/>,
		);

		expect(onClose).toHaveBeenCalledTimes(1);
	});
});

describe("MutationDialog error region", () => {
	it("fires onError exactly once on failure and stays open", () => {
		const mutation = makeMutation();
		const onError = vi.fn();
		const onClose = vi.fn();

		const view = render(
			<MutationDialog
				mutation={mutation}
				title="Delete gadget"
				onClose={onClose}
				body={stubBody()}
				onError={onError}
			/>,
		);
		const error = envelopeError();
		view.rerender(
			<MutationDialog
				mutation={{ ...mutation, error }}
				title="Delete gadget"
				onClose={onClose}
				body={stubBody()}
				onError={onError}
			/>,
		);

		expect(onError).toHaveBeenCalledTimes(1);
		expect(onError).toHaveBeenCalledWith(error);
		expect(onClose).not.toHaveBeenCalled();
		expect(screen.getByTestId("modal")).toBeInTheDocument();
	});

	it("renders the default error region from the envelope, above the body", () => {
		const mutation = makeMutation({ error: envelopeError() });

		render(
			<MutationDialog
				mutation={mutation}
				title="Delete gadget"
				onClose={vi.fn()}
				body={stubBody()}
			/>,
		);

		const alert = screen.getByRole("alert");
		expect(alert).toHaveTextContent("GADGET_IN_USE");
		expect(alert).toHaveTextContent("Gadget is in use");
		const body = screen.getByTestId("stub-body");
		expect(
			alert.compareDocumentPosition(body) & Node.DOCUMENT_POSITION_FOLLOWING,
		).toBeTruthy();
	});

	it("renders no error region while the mutation is untouched", () => {
		const mutation = makeMutation();

		render(
			<MutationDialog
				mutation={mutation}
				title="Delete gadget"
				onClose={vi.fn()}
				body={stubBody()}
			/>,
		);

		expect(screen.queryByRole("alert")).not.toBeInTheDocument();
	});

	it("replaces the whole region with renderError, handing over the parsed envelope", () => {
		const renderError = vi.fn((apiError: { code: string; message: string }) => (
			<Text data-testid="custom-error">{`${apiError.code}: ${apiError.message}`}</Text>
		));
		const mutation = makeMutation({ error: envelopeError() });

		render(
			<MutationDialog
				mutation={mutation}
				title="Delete gadget"
				onClose={vi.fn()}
				body={stubBody()}
				renderError={renderError}
			/>,
		);

		expect(renderError).toHaveBeenCalledTimes(1);
		expect(renderError).toHaveBeenCalledWith({
			code: "GADGET_IN_USE",
			message: "Gadget is in use",
		});
		expect(screen.getByTestId("custom-error")).toHaveTextContent(
			"GADGET_IN_USE: Gadget is in use",
		);
		expect(screen.queryByRole("alert")).not.toBeInTheDocument();
	});
});

describe("ConfirmDialog danger footer", () => {
	it("confirms by mutating with the variables the caller's handler returns", async () => {
		const user = userEvent.setup();
		const mutation = makeMutation();
		const onClose = vi.fn();
		const confirm = vi.fn(() => "db-password");

		render(
			<ConfirmDialog
				mutation={mutation}
				title="Delete secret"
				confirm={confirm}
				onClose={onClose}
				body={stubBody()}
			/>,
		);

		const confirmButton = screen.getByTestId("confirm-delete-button");
		expect(confirmButton).toHaveTextContent("Delete");
		await user.click(confirmButton);

		expect(confirm).toHaveBeenCalledTimes(1);
		expect(mutation.mutate).toHaveBeenCalledTimes(1);
		expect(mutation.mutate).toHaveBeenCalledWith("db-password");
		expect(onClose).not.toHaveBeenCalled();
	});

	it("canceling closes without sending anything", async () => {
		const user = userEvent.setup();
		const mutation = makeMutation();
		const onClose = vi.fn();

		render(
			<ConfirmDialog
				mutation={mutation}
				title="Delete secret"
				confirm={() => "db-password"}
				onClose={onClose}
				body={stubBody()}
			/>,
		);

		await user.click(screen.getByRole("button", { name: "Cancel" }));

		expect(onClose).toHaveBeenCalledTimes(1);
		expect(mutation.mutate).not.toHaveBeenCalled();
	});

	it("wires the pending state into the confirm button only", () => {
		const mutation = makeMutation({ isPending: true });

		render(
			<ConfirmDialog
				mutation={mutation}
				title="Delete secret"
				confirm={() => "db-password"}
				onClose={vi.fn()}
				body={stubBody()}
			/>,
		);

		expect(screen.getByTestId("confirm-delete-button")).toBeDisabled();
		expect(screen.getByRole("button", { name: "Cancel" })).toBeEnabled();
	});

	it("replaces the default confirm button with renderConfirm, receiving error, pending, and the confirm handler", async () => {
		const user = userEvent.setup();
		const error = envelopeError();
		const mutation = makeMutation({ error });
		const confirm = vi.fn(() => "db-password");
		const renderConfirm = vi.fn(
			({
				error: renderError,
				isPending,
				onConfirm,
			}: {
				error: unknown;
				isPending: boolean;
				onConfirm: () => void;
			}) => (
				<Button data-testid="custom-confirm" onClick={onConfirm}>
					{isPending ? "pending" : "idle"}
					{renderError ? "!" : ""}
				</Button>
			),
		);

		render(
			<ConfirmDialog
				mutation={mutation}
				title="Delete secret"
				confirm={confirm}
				onClose={vi.fn()}
				body={stubBody()}
				renderConfirm={renderConfirm}
			/>,
		);

		expect(screen.getByTestId("custom-confirm")).toHaveTextContent("idle!");
		expect(
			screen.queryByTestId("confirm-delete-button"),
		).not.toBeInTheDocument();
		expect(screen.getByRole("button", { name: "Cancel" })).toBeInTheDocument();
		await user.click(screen.getByTestId("custom-confirm"));

		expect(confirm).toHaveBeenCalledTimes(1);
		expect(mutation.mutate).toHaveBeenCalledWith("db-password");
		const latestCall =
			renderConfirm.mock.calls[renderConfirm.mock.calls.length - 1][0];
		expect(latestCall.error).toBe(error);
		expect(latestCall.isPending).toBe(false);
	});
});
