import { usePullImage } from "@features/images/hooks/usePullImage";
import type { ApiError } from "@shared/types/api";
import { useEffect, useRef } from "react";

export interface PullThenRetry {
	isPulling: boolean;
	progress: ReturnType<typeof usePullImage>["progress"];
	error: ApiError | null;
	startPull: (image: string) => void;
}

// The pull-then-retry-once ritual of the create page (spec #371, ticket
// #370): the hook composes the images feature's pull hook and owns the
// retry decision, so the page keeps form ownership only. Requesting a
// pull arms a synchronous ref guard; a pull that settles without an
// error fires the caller's post-pull callback exactly once, and a pull
// that settles with an error fires nothing.
export function usePullThenRetry(
	hostId: string,
	onPulled: () => void,
): PullThenRetry {
	const pullImage = usePullImage(hostId);

	// A ref, not state: the retry decision must not depend on render
	// timing. The caller's re-render storm (its own mutation pending
	// state) must be able neither to re-fire the callback in a loop nor to
	// lose the fire — the ref flips synchronously, so the effect below
	// fires exactly once per requested pull.
	const pullRequestedRef = useRef(false);

	// The latest-callback ref: the callback's identity churn (the caller
	// re-renders with a fresh submit closure) must not re-trigger the
	// effect — the effect keys on the pull state only and reads whatever
	// callback is current when the pull settles.
	const onPulledRef = useRef(onPulled);
	onPulledRef.current = onPulled;

	const startPull = (image: string) => {
		pullRequestedRef.current = true;
		pullImage.pull(image);
	};

	useEffect(() => {
		if (!pullRequestedRef.current || pullImage.isPulling) return;
		pullRequestedRef.current = false;
		if (pullImage.error) return;
		onPulledRef.current();
	}, [pullImage.isPulling, pullImage.error]);

	return {
		isPulling: pullImage.isPulling,
		progress: pullImage.progress,
		error: pullImage.error,
		startPull,
	};
}
