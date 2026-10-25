// The memory-units pattern: one owner for the client-side check against
// podman's units syntax (a byte count with an optional unit suffix,
// e.g. "512m", "1g"). The create-pod dialog and the create-container
// page both import it for their memory fields' register validation —
// the check mirrors podman's units parser loosely, keeping obvious
// typos from a round trip; the backend's units parser stays the
// validator (issue #377, the cross-domain duplication ruling's second
// implementation).
export const MEMORY_UNITS_PATTERN = /^\d+(\.\d+)?\s*(b|k|m|g|t|kb|mb|gb|tb)?$/i;
