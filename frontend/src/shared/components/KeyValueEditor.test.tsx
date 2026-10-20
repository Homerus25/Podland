import { KeyValueEditor } from "@shared/components/KeyValueEditor";
import {
	type KeyValueRow,
	keyValueRowsToRecord,
} from "@shared/utils/keyValueRows";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useFieldArray, useForm } from "react-hook-form";
import { describe, expect, it, vi } from "vitest";
import { renderWithQuery } from "@/test/render";

interface HostFormValues {
	entries: KeyValueRow[];
}

// The module never touches the form: the host owns useForm/useFieldArray
// and hands over the field-array return, exactly like the dialog adapters.
function Host({
	requireKeyWithValue = false,
	onSubmit,
}: {
	requireKeyWithValue?: boolean;
	onSubmit: (values: HostFormValues) => void;
}) {
	const {
		register,
		control,
		handleSubmit,
		formState: { errors },
	} = useForm<HostFormValues>({
		defaultValues: { entries: [] },
		mode: "onChange",
	});
	const rows = useFieldArray({ control, name: "entries" });
	return (
		<form onSubmit={handleSubmit(onSubmit)}>
			<KeyValueEditor
				name="entries"
				rows={rows}
				register={register}
				keyErrorMessage={(index) => errors.entries?.[index]?.key?.message}
				noun="entry"
				addLabel="Add entry"
				keyAriaLabel={(rowNumber) => `entry key ${rowNumber}`}
				valueAriaLabel={(rowNumber) => `entry value ${rowNumber}`}
				removeAriaLabel={(rowNumber) => `Remove entry ${rowNumber}`}
				requireKeyWithValue={requireKeyWithValue}
			/>
			<button type="submit" data-testid="host-submit">
				Submit
			</button>
		</form>
	);
}

describe("KeyValueEditor", () => {
	it("derives the testids, aria labels, placeholders, and add-button copy from its props", async () => {
		renderWithQuery(<Host onSubmit={vi.fn()} />);
		const user = userEvent.setup();

		const addButton = screen.getByTestId("add-entry");
		expect(addButton).toHaveTextContent("Add entry");

		await user.click(addButton);

		expect(screen.getByLabelText("entry key 1")).toHaveAttribute(
			"placeholder",
			"key",
		);
		expect(screen.getByLabelText("entry value 1")).toHaveAttribute(
			"placeholder",
			"value",
		);
		expect(screen.getByTestId("remove-entry-0")).toHaveAttribute(
			"aria-label",
			"Remove entry 1",
		);
	});

	it("adds rows and removes them by index, re-numbering the remaining rows", async () => {
		renderWithQuery(<Host onSubmit={vi.fn()} />);
		const user = userEvent.setup();

		await user.click(screen.getByTestId("add-entry"));
		await user.click(screen.getByTestId("add-entry"));
		await user.type(screen.getByLabelText("entry key 1"), "first");
		await user.type(screen.getByLabelText("entry key 2"), "second");

		await user.click(screen.getByTestId("remove-entry-0"));

		expect(screen.queryByLabelText("entry key 2")).not.toBeInTheDocument();
		expect(screen.getByLabelText("entry key 1")).toHaveValue("second");
		expect(screen.getByTestId("remove-entry-0")).toBeInTheDocument();
	});

	it("validates nothing per-row by default: a value without a key submits", async () => {
		const onSubmit = vi.fn();
		renderWithQuery(<Host onSubmit={onSubmit} />);
		const user = userEvent.setup();

		await user.click(screen.getByTestId("add-entry"));
		await user.type(screen.getByLabelText("entry value 1"), "orphan");

		await user.click(screen.getByTestId("host-submit"));

		await waitFor(() => expect(onSubmit).toHaveBeenCalledTimes(1));
		expect(onSubmit).toHaveBeenCalledWith(
			{ entries: [{ key: "", value: "orphan" }] },
			expect.anything(),
		);
		expect(
			screen.queryByText("Key is required when a value is set"),
		).not.toBeInTheDocument();
	});

	it("requires a key when a value is set, gating submit until the key is filled", async () => {
		const onSubmit = vi.fn();
		renderWithQuery(<Host requireKeyWithValue onSubmit={onSubmit} />);
		const user = userEvent.setup();

		await user.click(screen.getByTestId("add-entry"));
		await user.type(screen.getByLabelText("entry value 1"), "orphan");

		await user.click(screen.getByTestId("host-submit"));

		expect(
			await screen.findByText("Key is required when a value is set"),
		).toBeInTheDocument();
		expect(onSubmit).not.toHaveBeenCalled();

		await user.type(screen.getByLabelText("entry key 1"), "team");
		await waitFor(() =>
			expect(
				screen.queryByText("Key is required when a value is set"),
			).not.toBeInTheDocument(),
		);

		await user.click(screen.getByTestId("host-submit"));
		await waitFor(() => expect(onSubmit).toHaveBeenCalledTimes(1));
		expect(onSubmit).toHaveBeenCalledWith(
			{ entries: [{ key: "team", value: "orphan" }] },
			expect.anything(),
		);
	});
});

describe("keyValueRowsToRecord", () => {
	it("returns undefined when no row is filled", () => {
		expect(keyValueRowsToRecord([])).toBeUndefined();
		expect(
			keyValueRowsToRecord([
				{ key: "", value: "x" },
				{ key: "   ", value: "" },
			]),
		).toBeUndefined();
	});

	it("skips rows with an empty key and trims the kept keys", () => {
		expect(
			keyValueRowsToRecord([
				{ key: " FOO ", value: "bar" },
				{ key: "", value: "ignored" },
				{ key: "MODE", value: "prod" },
			]),
		).toEqual({ FOO: "bar", MODE: "prod" });
	});

	it("collapses duplicate keys to the last value", () => {
		expect(
			keyValueRowsToRecord([
				{ key: "FOO", value: "first" },
				{ key: "FOO", value: "last" },
			]),
		).toEqual({ FOO: "last" });
	});
});
