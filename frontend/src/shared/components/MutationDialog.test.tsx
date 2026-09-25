import { Button, Text } from "@chakra-ui/react";
import {
	ActionDialog,
	ConfirmDialog,
	FormDialog,
	FormFooter,
	type FormStateSlice,
	FormValidityScope,
	MutationDialog,
	type MutationSlice,
} from "@shared/components/MutationDialog";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { axiosError } from "@/test/errors";
import { renderWithQuery } from "@/test/render";

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

function stubBody() {
	return <Text data-testid="stub-body">Stub body</Text>;
}

// The body side of the form-id association: a form carrying the id the
// footer's submit button associates to. The submit handler stands in for
// the react-hook-form handleSubmit the real bodies own; it calls mutate
// the same way.
function stubForm(formId: string, onSubmit: () => void) {
	return (
		<form
			id={formId}
			data-testid="stub-form"
			noValidate
			onSubmit={(event) => {
				event.preventDefault();
				onSubmit();
			}}
		>
			<input data-testid="stub-input" />
		</form>
	);
}

// Chakra compiles colorScheme into the emotion class's stylesheet rule;
// jsdom does not resolve the CSS variables onto the element itself, so the
// pin reads the palette out of the background rule the button's class
// carries (base, hover, and active all set it in the button's own palette).
function colorSchemeOf(button: HTMLElement): string | null {
	const token = button.className
		.split(/\s+/)
		.find((className) => className.startsWith("css-"));
	if (!token) return null;
	for (const sheet of Array.from(document.styleSheets)) {
		let rules: CSSRuleList;
		try {
			rules = sheet.cssRules;
		} catch {
			continue;
		}
		for (const rule of Array.from(rules)) {
			if (!(rule instanceof CSSStyleRule)) continue;
			const palette = rule.cssText.match(
				/background: var\(--chakra-colors-([a-z]+)-\d+\)/,
			);
			if (rule.selectorText?.includes(token) && palette) {
				return palette[1];
			}
		}
	}
	return null;
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
		const error = axiosError(409, "GADGET_IN_USE", "Gadget is in use", "req-9");
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
		const mutation = makeMutation({
			error: axiosError(409, "GADGET_IN_USE", "Gadget is in use", "req-9"),
		});

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
		const mutation = makeMutation({
			error: axiosError(409, "GADGET_IN_USE", "Gadget is in use", "req-9"),
		});

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
		const error = axiosError(409, "GADGET_IN_USE", "Gadget is in use", "req-9");
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

describe("ConfirmDialog confirm knobs", () => {
	it("keeps the destructive defaults when both knobs are absent: red confirm under confirm-delete-button labeled Delete", () => {
		renderWithQuery(
			<ConfirmDialog
				mutation={makeMutation()}
				title="Delete secret"
				confirm={() => "db-password"}
				onClose={vi.fn()}
				body={stubBody()}
			/>,
			{ chakra: true },
		);

		const confirmButton = screen.getByTestId("confirm-delete-button");
		expect(confirmButton).toHaveTextContent("Delete");
		expect(colorSchemeOf(confirmButton)).toBe("red");
	});

	it("routes confirmTestId and confirmColorScheme into the confirm button, keeping the confirmLabel seam", () => {
		renderWithQuery(
			<ConfirmDialog
				mutation={makeMutation()}
				title="Restart quadlet"
				confirm={() => "db-backup"}
				onClose={vi.fn()}
				body={stubBody()}
				confirmTestId="confirm-lifecycle-button"
				confirmColorScheme="gray"
				confirmLabel="Restart quadlet"
			/>,
			{ chakra: true },
		);

		const confirmButton = screen.getByTestId("confirm-lifecycle-button");
		expect(confirmButton).toHaveTextContent("Restart quadlet");
		expect(
			screen.queryByTestId("confirm-delete-button"),
		).not.toBeInTheDocument();
		expect(colorSchemeOf(confirmButton)).toBe("gray");
	});
});

describe("ActionDialog action footer", () => {
	it("renders the shell with ghost Cancel and the required confirm, and cancel closes without firing onConfirm", async () => {
		const user = userEvent.setup();
		const onConfirm = vi.fn();
		const onClose = vi.fn();

		render(
			<ActionDialog
				title="Update all quadlets"
				body={stubBody()}
				confirmLabel="Update all"
				confirmTestId="confirm-auto-update-button"
				onConfirm={onConfirm}
				onClose={onClose}
			/>,
		);

		expect(screen.getByTestId("modal")).toBeInTheDocument();
		expect(screen.getByText("Update all quadlets")).toBeInTheDocument();
		expect(screen.getByTestId("stub-body")).toBeInTheDocument();
		expect(screen.getByTestId("confirm-auto-update-button")).toHaveTextContent(
			"Update all",
		);

		await user.click(screen.getByRole("button", { name: "Cancel" }));

		expect(onClose).toHaveBeenCalledTimes(1);
		expect(onConfirm).not.toHaveBeenCalled();
	});

	it("fires onConfirm without closing, renders no error region, and carries confirmColorScheme into the confirm button", async () => {
		const user = userEvent.setup();
		const onConfirm = vi.fn();
		const onClose = vi.fn();

		renderWithQuery(
			<ActionDialog
				title="Update all quadlets"
				body={stubBody()}
				confirmLabel="Update all"
				confirmTestId="confirm-auto-update-button"
				confirmColorScheme="orange"
				onConfirm={onConfirm}
				onClose={onClose}
			/>,
			{ chakra: true },
		);

		await user.click(screen.getByTestId("confirm-auto-update-button"));

		expect(onConfirm).toHaveBeenCalledTimes(1);
		expect(onClose).not.toHaveBeenCalled();
		expect(screen.getByTestId("modal")).toBeInTheDocument();
		expect(screen.queryByRole("alert")).not.toBeInTheDocument();
		expect(
			colorSchemeOf(screen.getByTestId("confirm-auto-update-button")),
		).toBe("orange");
	});
});

describe("FormDialog form preset", () => {
	it("submits through the body's form via the native form-id association from the footer button", async () => {
		const user = userEvent.setup();
		const mutation = makeMutation();
		const onClose = vi.fn();
		const submitForm = vi.fn(() => mutation.mutate("gadget-values"));

		renderWithQuery(
			// The scope sits above the dialog: the form owner renders it, and
			// React context — not DOM nesting — is what reaches the footer
			// through the modal portal.
			<FormValidityScope formState={{ isValid: true, isSubmitting: false }}>
				<FormDialog
					mutation={mutation}
					title="Create gadget"
					onClose={onClose}
					formId="gadget-form"
					submitLabel="Create gadget"
					submitTestId="gadget-submit"
					body={stubForm("gadget-form", submitForm)}
				/>
			</FormValidityScope>,
			{ chakra: true },
		);

		const submit = screen.getByTestId("gadget-submit");
		expect(submit).toHaveTextContent("Create gadget");
		expect(submit).toBeEnabled();
		// The submit lives in the modal footer, outside the body's form:
		// association is by native form id, not nesting.
		expect(screen.getByTestId("stub-form").contains(submit)).toBe(false);
		expect(colorSchemeOf(submit)).toBe("blue");

		await user.click(submit);

		expect(submitForm).toHaveBeenCalledTimes(1);
		expect(mutation.mutate).toHaveBeenCalledWith("gadget-values");
		expect(onClose).not.toHaveBeenCalled();
	});

	it("submits on Enter in a field through the same association", async () => {
		const user = userEvent.setup();
		const mutation = makeMutation();
		const submitForm = vi.fn(() => mutation.mutate("gadget-values"));

		render(
			<FormValidityScope formState={{ isValid: true, isSubmitting: false }}>
				<FormDialog
					mutation={mutation}
					title="Create gadget"
					onClose={vi.fn()}
					formId="gadget-form"
					submitLabel="Create gadget"
					submitTestId="gadget-submit"
					body={stubForm("gadget-form", submitForm)}
				/>
			</FormValidityScope>,
		);

		await user.type(screen.getByTestId("stub-input"), "{enter}");

		expect(submitForm).toHaveBeenCalledTimes(1);
		expect(mutation.mutate).toHaveBeenCalledWith("gadget-values");
	});

	it("canceling closes without sending anything from the form footer", async () => {
		const user = userEvent.setup();
		const mutation = makeMutation();
		const onClose = vi.fn();
		const submitForm = vi.fn(() => mutation.mutate("gadget-values"));

		render(
			<FormValidityScope formState={{ isValid: true, isSubmitting: false }}>
				<FormDialog
					mutation={mutation}
					title="Create gadget"
					onClose={onClose}
					formId="gadget-form"
					submitLabel="Create gadget"
					submitTestId="gadget-submit"
					body={stubForm("gadget-form", submitForm)}
				/>
			</FormValidityScope>,
		);

		await user.click(screen.getByRole("button", { name: "Cancel" }));

		expect(onClose).toHaveBeenCalledTimes(1);
		expect(submitForm).not.toHaveBeenCalled();
		expect(mutation.mutate).not.toHaveBeenCalled();
	});

	it("disables submit while the published state is invalid or submitting or the mutation is pending, keeping cancel enabled", () => {
		const dialog = (props: {
			isPending: boolean;
			formState: FormStateSlice;
		}) => (
			<FormValidityScope formState={props.formState}>
				<FormDialog
					mutation={makeMutation({ isPending: props.isPending })}
					title="Create gadget"
					onClose={vi.fn()}
					formId="gadget-form"
					submitLabel="Create gadget"
					submitTestId="gadget-submit"
					body={stubForm("gadget-form", vi.fn())}
				/>
			</FormValidityScope>
		);
		const view = render(
			dialog({
				isPending: true,
				formState: { isValid: true, isSubmitting: false },
			}),
		);
		expect(screen.getByTestId("gadget-submit")).toBeDisabled();
		expect(screen.getByRole("button", { name: "Cancel" })).toBeEnabled();

		view.rerender(
			dialog({
				isPending: false,
				formState: { isValid: false, isSubmitting: false },
			}),
		);
		expect(screen.getByTestId("gadget-submit")).toBeDisabled();

		view.rerender(
			dialog({
				isPending: false,
				formState: { isValid: true, isSubmitting: true },
			}),
		);
		expect(screen.getByTestId("gadget-submit")).toBeDisabled();

		view.rerender(
			dialog({
				isPending: false,
				formState: { isValid: true, isSubmitting: false },
			}),
		);
		expect(screen.getByTestId("gadget-submit")).toBeEnabled();
	});
});

describe("FormFooter standalone", () => {
	it("renders the footer shape: submit fires the associated form, cancel fires onCancel", async () => {
		const user = userEvent.setup();
		const onSubmit = vi.fn();
		const onCancel = vi.fn();

		render(
			<>
				<form
					id="solo-form"
					noValidate
					onSubmit={(event) => {
						event.preventDefault();
						onSubmit();
					}}
				>
					<input data-testid="solo-input" />
				</form>
				<FormFooter
					formId="solo-form"
					submitLabel="Save machine"
					submitTestId="machine-form-save"
					pending={false}
					onCancel={onCancel}
				/>
			</>,
		);

		const submit = screen.getByTestId("machine-form-save");
		expect(submit).toHaveTextContent("Save machine");
		expect(submit).toBeEnabled();

		await user.click(submit);
		expect(onSubmit).toHaveBeenCalledTimes(1);

		await user.click(screen.getByRole("button", { name: "Cancel" }));
		expect(onCancel).toHaveBeenCalledTimes(1);
	});

	it("names the cancel button when given a cancelTestId", () => {
		const onCancel = vi.fn();
		render(
			<>
				<form id="solo-form" />
				<FormFooter
					formId="solo-form"
					submitLabel="Save machine"
					submitTestId="machine-form-save"
					pending={false}
					onCancel={onCancel}
					cancelTestId="machine-form-cancel"
				/>
			</>,
		);

		expect(screen.getByTestId("machine-form-cancel")).toBeEnabled();
	});

	it("applies the one submit policy and drops cancel without an onCancel", () => {
		const footer = (props: { pending: boolean; onCancel?: boolean }) => (
			<FormFooter
				formId="solo-form"
				submitLabel="Save machine"
				submitTestId="machine-form-save"
				pending={props.pending}
				onCancel={props.onCancel ? vi.fn() : undefined}
			/>
		);
		const view = render(
			<>
				<form id="solo-form" />
				{footer({ pending: true, onCancel: true })}
			</>,
		);
		expect(screen.getByTestId("machine-form-save")).toBeDisabled();
		expect(screen.getByRole("button", { name: "Cancel" })).toBeEnabled();

		view.rerender(
			<>
				<form id="solo-form" />
				{footer({ pending: false, onCancel: true })}
			</>,
		);
		expect(screen.getByTestId("machine-form-save")).toBeEnabled();

		view.rerender(
			<FormValidityScope formState={{ isValid: false, isSubmitting: false }}>
				<form id="solo-form" />
				{footer({ pending: false, onCancel: true })}
			</FormValidityScope>,
		);
		expect(screen.getByTestId("machine-form-save")).toBeDisabled();

		view.rerender(
			<FormValidityScope formState={{ isValid: true, isSubmitting: true }}>
				<form id="solo-form" />
				{footer({ pending: false, onCancel: true })}
			</FormValidityScope>,
		);
		expect(screen.getByTestId("machine-form-save")).toBeDisabled();

		view.rerender(
			<>
				<form id="solo-form" />
				{footer({ pending: false, onCancel: false })}
			</>,
		);
		expect(
			screen.queryByRole("button", { name: "Cancel" }),
		).not.toBeInTheDocument();
	});
});
