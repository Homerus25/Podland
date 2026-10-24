import { Box, Button, type ButtonProps, Text } from "@chakra-ui/react";
import { ErrorDisplay } from "@shared/components/ErrorDisplay";
import { Modal } from "@shared/components/Modal";
import type { ApiError } from "@shared/types/api";
import { getApiError } from "@shared/utils/api";
import {
	createContext,
	type ReactNode,
	useContext,
	useEffect,
	useRef,
} from "react";

// The structural slice of a react-query mutation result this module drives.
// Features keep their mutation hooks and hand over the result itself; the
// module never touches react-query.
export interface MutationSlice<TVariables = unknown> {
	mutate: (variables: TVariables) => void;
	isPending: boolean;
	error: unknown;
	isSuccess: boolean;
	reset: () => void;
}

// One owner for the machinery every mutation dialog hand-rolled: the modal
// shell, the reset invariant, close-on-success, the side-effect seams, and
// the in-modal error region. Consumers render the dialog conditionally, so
// mount is open and unmount is close; the module passes an always-open
// shell and runs reset() on both ends, which is what makes reopening start
// clean. Success closes through onClose after firing the caller's onSuccess
// seam exactly once (ref-guarded against double-fire); failure fires
// onError and stays open.
export function MutationDialog<TVariables>({
	mutation,
	title,
	onClose,
	body,
	footer,
	onSuccess,
	onError,
	renderError,
}: {
	mutation: MutationSlice<TVariables>;
	title: string;
	onClose: () => void;
	body: ReactNode;
	footer?: ReactNode;
	onSuccess?: () => void;
	onError?: (error: unknown) => void;
	renderError?: (apiError: ApiError) => ReactNode;
}) {
	const { reset } = mutation;
	useEffect(() => {
		reset();
		return () => reset();
	}, [reset]);

	const successHandledRef = useRef(false);
	useEffect(() => {
		if (!mutation.isSuccess || successHandledRef.current) return;
		successHandledRef.current = true;
		onSuccess?.();
		onClose();
	}, [mutation.isSuccess, onSuccess, onClose]);

	const errorHandledRef = useRef<unknown>(undefined);
	useEffect(() => {
		if (!mutation.error || errorHandledRef.current === mutation.error) return;
		errorHandledRef.current = mutation.error;
		onError?.(mutation.error);
	}, [mutation.error, onError]);

	return (
		<Modal isOpen onClose={onClose} title={title} footer={footer}>
			{mutation.error ? (
				<Box mt={2}>
					{renderError ? (
						renderError(getApiError(mutation.error))
					) : (
						<ErrorDisplay
							error={getApiError(mutation.error)}
							data-testid="mutation-dialog-error"
						/>
					)}
				</Box>
			) : null}
			{body}
		</Modal>
	);
}

// The danger preset over the core: ghost cancel and the red confirm
// carrying the confirm-delete-button testid, pending wired to the mutation.
// The caller's confirm handler returns the variables to mutate with; option
// state (checkboxes) lives in the caller's body and closes over it.
// renderConfirm replaces the default confirm button (e.g. a force-delete
// swap driven by the error) and still receives the confirm handler.
// confirmColorScheme and confirmTestId re-skin the default confirm for
// non-destructive confirms; both default to the destructive look.
export function ConfirmDialog<TVariables>({
	mutation,
	title,
	onClose,
	body,
	confirm,
	confirmLabel = "Delete",
	confirmColorScheme = "red",
	confirmTestId = "confirm-delete-button",
	onSuccess,
	onError,
	renderError,
	renderConfirm,
}: {
	mutation: MutationSlice<TVariables>;
	title: string;
	onClose: () => void;
	body: ReactNode;
	confirm: () => TVariables;
	confirmLabel?: string;
	confirmColorScheme?: ButtonProps["colorScheme"];
	confirmTestId?: string;
	onSuccess?: () => void;
	onError?: (error: unknown) => void;
	renderError?: (apiError: ApiError) => ReactNode;
	renderConfirm?: (api: {
		error: unknown;
		isPending: boolean;
		onConfirm: () => void;
	}) => ReactNode;
}) {
	const onConfirm = () => mutation.mutate(confirm());

	return (
		<MutationDialog
			mutation={mutation}
			title={title}
			onClose={onClose}
			body={body}
			onSuccess={onSuccess}
			onError={onError}
			renderError={renderError}
			footer={
				<>
					<Button variant="ghost" onClick={onClose}>
						Cancel
					</Button>
					{renderConfirm ? (
						renderConfirm({
							error: mutation.error,
							isPending: mutation.isPending,
							onConfirm,
						})
					) : (
						<Button
							colorScheme={confirmColorScheme}
							onClick={onConfirm}
							isLoading={mutation.isPending}
							data-testid={confirmTestId}
						>
							{confirmLabel}
						</Button>
					)}
				</>
			}
		/>
	);
}

// The force-refusal preset over ConfirmDialog: the confirm-to-force ritual
// a destructive delete hand-rolled per feature until now. The preset owns
// the ritual through ConfirmDialog's existing replacement seams — the
// refusal-code hint replaces the error region (renderError, orange text,
// with the shared ErrorDisplay for every other error), and the footer
// swaps the confirm for the outline force button (renderConfirm) — plus
// the force flag keyed off the current error: the first confirm sends
// the plain request, and only after the refusal does the retry carry
// force=true. The adapter keeps its domain only: the refusal code, the
// hint copy and its testid, the labels, the force button's testid, and
// the payload builder taking the force flag. Settled 2026-10-24
// (2026-10-24 architecture review, ticket 358).
export function ForceConfirmDialog<TVariables>({
	mutation,
	title,
	onClose,
	body,
	refusalCode,
	hint,
	hintTestId,
	confirmLabel,
	forceLabel,
	forceTestId,
	confirm,
	onSuccess,
	onError,
}: {
	mutation: MutationSlice<TVariables>;
	title: string;
	onClose: () => void;
	body: ReactNode;
	refusalCode: string;
	hint: string;
	hintTestId: string;
	confirmLabel: string;
	forceLabel: string;
	forceTestId: string;
	confirm: (force: boolean) => TVariables;
	onSuccess?: () => void;
	onError?: (error: unknown) => void;
}) {
	const refused = (error: unknown) => getApiError(error).code === refusalCode;

	return (
		<ConfirmDialog
			mutation={mutation}
			title={title}
			onClose={onClose}
			body={body}
			confirm={() => confirm(refused(mutation.error))}
			confirmLabel={confirmLabel}
			onSuccess={onSuccess}
			onError={onError}
			renderError={(apiError) =>
				apiError.code === refusalCode ? (
					<Text mt={2} color="orange.500" data-testid={hintTestId}>
						{hint}
					</Text>
				) : (
					<ErrorDisplay error={apiError} />
				)
			}
			renderConfirm={({ error, isPending, onConfirm }) =>
				refused(error) ? (
					<Button
						colorScheme="red"
						variant="outline"
						onClick={onConfirm}
						isLoading={isPending}
						data-testid={forceTestId}
					>
						{forceLabel}
					</Button>
				) : (
					<Button
						colorScheme="red"
						onClick={onConfirm}
						isLoading={isPending}
						data-testid="confirm-delete-button"
					>
						{confirmLabel}
					</Button>
				)
			}
		/>
	);
}

// The mutation-less confirm preset: composes the plain modal shell, not the
// mutation core, for confirms that perform a caller-owned action instead of
// a mutation — gating a stream, opening a view. No pending wiring, no error
// region, no reset machinery, and no auto-close: the caller closes in
// onConfirm. Label and testid are required, so new adapters state the
// intent; without a color scheme the button keeps Chakra's neutral look.
export function ActionDialog({
	title,
	body,
	confirmLabel,
	confirmTestId,
	onConfirm,
	onClose,
	confirmColorScheme,
}: {
	title: string;
	body: ReactNode;
	confirmLabel: string;
	confirmTestId: string;
	onConfirm: () => void;
	onClose: () => void;
	confirmColorScheme?: ButtonProps["colorScheme"];
}) {
	return (
		<Modal
			isOpen
			onClose={onClose}
			title={title}
			footer={
				<>
					<Button variant="ghost" onClick={onClose}>
						Cancel
					</Button>
					<Button
						colorScheme={confirmColorScheme}
						onClick={onConfirm}
						data-testid={confirmTestId}
					>
						{confirmLabel}
					</Button>
				</>
			}
		>
			{body}
		</Modal>
	);
}

// The slice of the form library's state the submit policy reads. The
// module stays form-library-blind: react-hook-form's formState satisfies
// it structurally, so bodies pass formState straight from useForm.
export interface FormStateSlice {
	isValid: boolean;
	isSubmitting: boolean;
}

// Defaults to always-valid so a footer rendered outside any scope (the
// non-dialog form site) gates on pending alone.
const FormValidityContext = createContext<FormStateSlice>({
	isValid: true,
	isSubmitting: false,
});

// The form owner (the component calling useForm) wraps its FormDialog — or,
// for the standalone footer, its form and FormFooter — in this to publish
// its validity state to the footer. React context is the channel: it flows
// through the modal portal only to React descendants, so the scope must sit
// above the dialog, not inside its body. Reading the two fields off the
// form library's state here is what subscribes the form's owner to exactly
// those values; the plain non-library role form needs no scope — the
// default already publishes always-valid.
export function FormValidityScope({
	formState,
	children,
}: {
	formState: FormStateSlice;
	children: ReactNode;
}) {
	return (
		<FormValidityContext.Provider
			value={{
				isValid: formState.isValid,
				isSubmitting: formState.isSubmitting,
			}}
		>
			{children}
		</FormValidityContext.Provider>
	);
}

// The composable form footer behind FormDialog: an optional ghost Cancel
// (rendered only when the caller wants one — never a dead button) and the
// blue submit associated to the form by native form id, so it triggers the
// form's submit handler with zero props crossing the body/footer boundary.
// One submit policy: disabled while the published form state is invalid or
// submitting, or while the mutation is pending; cancel stays enabled.
// cancelTestId is opt-in: dialog cancels stay unnamed (suites select them
// by role), the non-dialog site names its cancel for its revert assertion.
export function FormFooter({
	formId,
	submitLabel,
	submitTestId,
	pending,
	onCancel,
	cancelTestId,
}: {
	formId: string;
	submitLabel: string;
	submitTestId: string;
	pending: boolean;
	onCancel?: () => void;
	cancelTestId?: string;
}) {
	const { isValid, isSubmitting } = useContext(FormValidityContext);
	const busy = isSubmitting || pending;

	return (
		<>
			{onCancel ? (
				<Button variant="ghost" onClick={onCancel} data-testid={cancelTestId}>
					Cancel
				</Button>
			) : null}
			<Button
				type="submit"
				form={formId}
				colorScheme="blue"
				isDisabled={!isValid || busy}
				isLoading={busy}
				data-testid={submitTestId}
			>
				{submitLabel}
			</Button>
		</>
	);
}

// The form preset over the core. The body shrinks to fields: its form
// carries the id and its submit handler calls mutate — this preset owns
// the footer (ghost Cancel closing without sending anything, submit
// associated by native form id). The form owner publishes validity by
// wrapping this dialog in FormValidityScope; without a scope the footer
// gates on pending alone. Submit label and testid are required; every
// site names its button. Success/error seams and the error region are the
// core's, unchanged.
export function FormDialog<TVariables>({
	mutation,
	title,
	onClose,
	body,
	formId,
	submitLabel,
	submitTestId,
	onSuccess,
	onError,
	renderError,
}: {
	mutation: MutationSlice<TVariables>;
	title: string;
	onClose: () => void;
	body: ReactNode;
	formId: string;
	submitLabel: string;
	submitTestId: string;
	onSuccess?: () => void;
	onError?: (error: unknown) => void;
	renderError?: (apiError: ApiError) => ReactNode;
}) {
	return (
		<MutationDialog
			mutation={mutation}
			title={title}
			onClose={onClose}
			body={body}
			onSuccess={onSuccess}
			onError={onError}
			renderError={renderError}
			footer={
				<FormFooter
					formId={formId}
					submitLabel={submitLabel}
					submitTestId={submitTestId}
					pending={mutation.isPending}
					onCancel={onClose}
				/>
			}
		/>
	);
}
