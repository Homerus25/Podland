import { Box, Button } from "@chakra-ui/react";
import { ErrorDisplay } from "@shared/components/ErrorDisplay";
import { Modal } from "@shared/components/Modal";
import type { ApiError } from "@shared/types/api";
import { getApiError } from "@shared/utils/api";
import { type ReactNode, useEffect, useRef } from "react";

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
export function ConfirmDialog<TVariables>({
	mutation,
	title,
	onClose,
	body,
	confirm,
	confirmLabel = "Delete",
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
							colorScheme="red"
							onClick={onConfirm}
							isLoading={mutation.isPending}
							data-testid="confirm-delete-button"
						>
							{confirmLabel}
						</Button>
					)}
				</>
			}
		/>
	);
}
