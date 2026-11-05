import type {
	ImageCopyInput,
	ImageCopyResult,
} from "@features/images/services/imagesApi";
import { imageCopyStreamUrl } from "@features/images/services/imagesApi";
import {
	type MoveStreamConfig,
	useMoveStream,
} from "@shared/transfers/transferHooks";
import { useMemo } from "react";

// The image copy's live progress state, folded from the stream's progress
// events (backend/internal/domain/images/copy.go ImageCopyEvent): one
// image, the latest phase winning. The backend emits a "bytes" event per
// reporting increment, so the entry updates in place instead of
// accumulating.
export interface ImageCopyProgress {
	phase: "start" | "bytes" | "done";
	bytes?: number;
}

// Event shape mirror of the backend's ImageCopyEvent (issue 375): progress
// events for the one image, then exactly one terminal event — the transfer
// envelope on "complete", the mapped code/message on "error".
interface ImageCopyStreamEvent {
	operationId: string;
	type: "image" | "complete" | "error";
	hostId: string;
	image?: string;
	phase?: ImageCopyProgress["phase"];
	bytes?: number;
	result?: ImageCopyResult;
	error?: { code: string; message: string };
}

function isImageCopyStreamEvent(value: unknown): value is ImageCopyStreamEvent {
	if (typeof value !== "object" || value === null) return false;
	const event = value as Record<string, unknown>;
	return (
		typeof event.operationId === "string" &&
		(event.type === "image" ||
			event.type === "complete" ||
			event.type === "error")
	);
}

// The single-value fold: the latest phase wins, and the bytes ride along
// when the event carries them.
function foldProgress(event: ImageCopyStreamEvent): ImageCopyProgress | null {
	return {
		phase: event.phase ?? "start",
		...(event.bytes !== undefined ? { bytes: event.bytes } : {}),
	};
}

export interface UseImageCopyStreamResult {
	isRunning: boolean;
	// The archive stream's latest state. Null until the first progress
	// event arrives and once the transfer has settled.
	progress: ImageCopyProgress | null;
	// The transfer envelope from the terminal complete event. Null until
	// then and on any failure.
	result: ImageCopyResult | null;
	// A terminal error event (e.g. IMAGE_TRANSFER_TOO_LARGE) or a
	// transport failure — never an abort.
	error: { code: string; message: string } | null;
	run: (imageId: string, input: ImageCopyInput) => void;
	cancel: () => void;
}

// The cross-host image copy (issue 376 over issue 375's endpoints): the
// endpoint always answers NDJSON, so the dialog drives it through the
// stream transport. The run/cancel lifecycle, reset-on-run, the
// malformed-event and complete-without-envelope skips, the terminal
// handling, and the both-hosts invalidation are the shared
// transfer-hooks module's (issue 273); this adapter keeps the images
// domain — the event vocabulary, shape guard, and single-value progress
// fold — and supplies the URL builder.
export function useImageCopyStream(hostId: string): UseImageCopyStreamResult {
	const config = useMemo<
		MoveStreamConfig<ImageCopyStreamEvent, ImageCopyProgress | null>
	>(
		() => ({
			resource: "images",
			url: (imageId) => imageCopyStreamUrl(hostId, imageId),
			isEvent: isImageCopyStreamEvent,
			foldProgress,
			initialProgress: null,
		}),
		[hostId],
	);
	return useMoveStream<
		ImageCopyStreamEvent,
		ImageCopyProgress | null,
		ImageCopyResult
	>(config);
}
