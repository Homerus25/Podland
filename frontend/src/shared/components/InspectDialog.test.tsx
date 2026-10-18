import {
	InspectDialog,
	type InspectQuerySlice,
} from "@shared/components/InspectDialog";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { axiosError } from "@/test/errors";

// The module renders the query result the caller hands it, never
// react-query itself: the stub below is the whole interface under test.
function makeQuery(
	overrides: Partial<InspectQuerySlice> = {},
): InspectQuerySlice {
	return {
		isPending: false,
		error: null,
		data: undefined,
		...overrides,
	};
}

function renderDialog(query = makeQuery(), onClose = vi.fn()) {
	render(
		<InspectDialog
			title="Inspect my-container"
			query={query}
			onClose={onClose}
		/>,
	);
	return onClose;
}

describe("InspectDialog", () => {
	it("renders the loading state while the query is pending", () => {
		renderDialog(makeQuery({ isPending: true }));

		expect(screen.getByTestId("inspect-dialog-loading")).toBeInTheDocument();
		expect(screen.queryByTestId("inspect-dialog-json")).not.toBeInTheDocument();
		expect(
			screen.queryByTestId("inspect-dialog-error"),
		).not.toBeInTheDocument();
	});

	it("renders the error region from the envelope", () => {
		renderDialog(
			makeQuery({
				error: axiosError(
					404,
					"CONTAINER_NOT_FOUND",
					"Container not found",
					"req-7",
				),
			}),
		);

		const alert = screen.getByTestId("inspect-dialog-error");
		expect(alert).toHaveTextContent("CONTAINER_NOT_FOUND");
		expect(alert).toHaveTextContent("Container not found");
		expect(screen.queryByTestId("inspect-dialog-json")).not.toBeInTheDocument();
	});

	it("renders the error region when the machine is unreachable (502)", () => {
		renderDialog(
			makeQuery({
				error: axiosError(
					502,
					"CONNECTION_FAILED",
					"Machine unreachable",
					"req-8",
				),
			}),
		);

		// getApiError collapses 5xx envelopes to the generic INTERNAL_ERROR
		// message; the pin is that the error region renders and the JSON
		// region does not, closing the 502 half of spec story 10.
		const alert = screen.getByTestId("inspect-dialog-error");
		expect(alert).toHaveTextContent("INTERNAL_ERROR");
		expect(screen.queryByTestId("inspect-dialog-json")).not.toBeInTheDocument();
	});

	it("renders the pretty-printed JSON with a 2-space indent", () => {
		const data = { Id: "c-1", State: { Running: true, Pid: 42 } };
		renderDialog(makeQuery({ data }));

		expect(screen.getByTestId("inspect-dialog-json").textContent).toBe(
			JSON.stringify(data, null, 2),
		);
		expect(
			screen.queryByTestId("inspect-dialog-loading"),
		).not.toBeInTheDocument();
		expect(
			screen.queryByTestId("inspect-dialog-error"),
		).not.toBeInTheDocument();
	});

	it("copies the pretty-printed JSON to the clipboard", async () => {
		const user = userEvent.setup();
		// userEvent.setup() installs its own clipboard stub, so the spy goes
		// on after it to be the one the dialog's click handler reads.
		const writeText = vi.fn().mockResolvedValue(undefined);
		Object.defineProperty(navigator, "clipboard", {
			value: { writeText },
			configurable: true,
		});
		const data = { Id: "c-1", State: { Running: true, Pid: 42 } };
		renderDialog(makeQuery({ data }));

		await user.click(screen.getByTestId("copy-inspect-button"));

		expect(writeText).toHaveBeenCalledTimes(1);
		expect(writeText).toHaveBeenCalledWith(JSON.stringify(data, null, 2));
	});

	it("fires onClose from the modal close button", async () => {
		const user = userEvent.setup();
		const onClose = renderDialog(makeQuery({ data: { Id: "c-1" } }));

		await user.click(screen.getByRole("button", { name: "Close dialog" }));

		expect(onClose).toHaveBeenCalledTimes(1);
	});

	it("fires onClose on Escape", async () => {
		const user = userEvent.setup();
		const onClose = renderDialog(makeQuery({ data: { Id: "c-1" } }));

		await user.keyboard("{Escape}");

		expect(onClose).toHaveBeenCalledTimes(1);
	});

	it("fires onClose on overlay click", async () => {
		const user = userEvent.setup();
		const onClose = renderDialog(makeQuery({ data: { Id: "c-1" } }));

		// Chakra wires the backdrop click on the full-viewport content
		// container (z-index modal, above the decorative overlay element),
		// so that is the surface a real backdrop click lands on.
		const backdrop = document.querySelector<HTMLElement>(
			".chakra-modal__content-container",
		)!;
		await user.click(backdrop);

		expect(onClose).toHaveBeenCalledTimes(1);
	});
});
