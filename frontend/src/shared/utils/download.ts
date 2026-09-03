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

	const objectUrl = URL.createObjectURL(blob);
	const anchor = document.createElement("a");
	anchor.href = objectUrl;
	anchor.download = name;
	document.body.appendChild(anchor);
	anchor.click();
	document.body.removeChild(anchor);
	URL.revokeObjectURL(objectUrl);
}
