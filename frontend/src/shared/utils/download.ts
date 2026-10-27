import {
	genericError,
	getApiErrorFromResponse,
	getBearerToken,
} from "@shared/utils/api";

// Extracts the download filename from a response's Content-Disposition
// header (backend handlers send `attachment; filename="name.tar"`).
function filenameFromDisposition(disposition: string | null): string | null {
	if (!disposition) return null;
	const match = disposition.match(/filename="?([^";]+)"?/);
	return match ? match[1] : null;
}

// Saves a blob via a temporary object URL anchored with the given filename.
// The one owner of the client-side download ritual; both download helpers
// (the fetch-based binary one and the text one) ride it.
function saveBlob(blob: Blob, filename: string): void {
	const objectUrl = URL.createObjectURL(blob);
	const anchor = document.createElement("a");
	anchor.href = objectUrl;
	anchor.download = filename;
	document.body.appendChild(anchor);
	anchor.click();
	document.body.removeChild(anchor);
	URL.revokeObjectURL(objectUrl);
}

// Makes a display name safe as a filename component: path separators,
// Windows-reserved punctuation (colon, asterisk, question mark, quotes,
// angle brackets, pipe), and whitespace become dashes; edge separators
// are trimmed.
export function sanitizeFilename(name: string): string {
	return name.replace(/[/\\:*?"<>|\s]/g, "-").replace(/^-+|-+$/g, "");
}

// Saves text already in hand (e.g. a fetched log history) as a file. The
// content arrived through an authenticated JSON call, so a direct link
// cannot work — the blob is built client-side.
export function downloadText(content: string, filename: string): void {
	saveBlob(new Blob([content], { type: "text/plain" }), filename);
}

// Downloads a binary endpoint (e.g. the volume export tar stream) with
// plain fetch — the shared axios client is JSON-oriented and would buffer,
// and this needs the same bearer-token handling as every other call. The
// response is saved via a temporary object URL anchored with the given
// filename, or the server's Content-Disposition name when omitted.
export async function downloadFile(
	url: string,
	filename?: string,
): Promise<void> {
	const token = getBearerToken();
	let response: Response;
	try {
		response = await fetch(url, {
			headers: token ? { Authorization: `Bearer ${token}` } : undefined,
		});
	} catch {
		throw { ...genericError };
	}

	if (!response.ok) {
		throw await getApiErrorFromResponse(response);
	}

	const blob = await response.blob();
	const name =
		filename ??
		filenameFromDisposition(response.headers.get("Content-Disposition"));
	if (!name) {
		throw { ...genericError };
	}

	saveBlob(blob, name);
}
