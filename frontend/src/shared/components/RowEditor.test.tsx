import {
	FormSection,
	MountFlags,
	RowInput,
	RowListEditor,
	RowSelect,
} from "@shared/components/RowEditor";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useFieldArray, useForm } from "react-hook-form";
import { describe, expect, it, vi } from "vitest";
import { renderWithQuery } from "@/test/render";

// The structured row-editor module's suite (spec #371, ticket #366) pins
// the row ritual once — the control rendering, the add/remove/testid/aria
// derivation, the z/Z mutual exclusion, the add-area slot — the way
// KeyValueEditor.test.tsx pins the key/value ritual. The hosts own
// useForm/useFieldArray/watch/setValue and hand over the returns; the
// module never calls form hooks.

interface InputHostValues {
	path: string;
	count: string;
}

function RowInputHost() {
	const { register } = useForm<InputHostValues>({
		defaultValues: { path: "", count: "" },
	});
	return (
		<>
			<RowInput
				ariaLabel="Device host path 1"
				placeholder="Host path"
				mono
				registration={register("path")}
			/>
			<RowInput
				ariaLabel="Host port 1"
				type="number"
				registration={register("count")}
			/>
		</>
	);
}

describe("RowInput", () => {
	it("renders its aria label, placeholder, mono font, and type", () => {
		renderWithQuery(<RowInputHost />);

		const path = screen.getByLabelText("Device host path 1");
		expect(path).toHaveAttribute("placeholder", "Host path");
		expect(path).toHaveStyle({ fontFamily: "mono" });

		const port = screen.getByLabelText("Host port 1");
		expect(port).toHaveAttribute("type", "number");
		expect(port).not.toHaveStyle({ fontFamily: "mono" });
	});
});

interface SelectHostValues {
	protocol: string;
	volume: string;
}

function RowSelectHost() {
	const { register } = useForm<SelectHostValues>({
		defaultValues: { protocol: "tcp", volume: "" },
	});
	return (
		<>
			<RowSelect
				ariaLabel="Protocol 1"
				options={[
					{ value: "tcp", label: "tcp" },
					{ value: "udp", label: "udp" },
				]}
				registration={register("protocol")}
			/>
			<RowSelect
				ariaLabel="Volume name 1"
				options={[{ value: "", label: "Select a volume" }]}
				registration={register("volume")}
			/>
		</>
	);
}

describe("RowSelect", () => {
	it("renders its aria label and its options as select entries", () => {
		renderWithQuery(<RowSelectHost />);

		const protocol = screen.getByLabelText("Protocol 1");
		expect(protocol).toHaveValue("tcp");
		expect(screen.getByRole("option", { name: "udp" })).toBeInTheDocument();

		expect(screen.getByLabelText("Volume name 1")).toHaveValue("");
		expect(
			screen.getByRole("option", { name: "Select a volume" }),
		).toBeInTheDocument();
	});
});

describe("FormSection", () => {
	it("renders its title as a section heading over its children", () => {
		renderWithQuery(
			<FormSection title="Storage">
				<button type="button">Add bind mount</button>
			</FormSection>,
		);

		expect(
			screen.getByRole("heading", { name: "Storage" }),
		).toBeInTheDocument();
		expect(
			screen.getByRole("button", { name: "Add bind mount" }),
		).toBeInTheDocument();
	});
});

interface DeviceRow {
	hostPath: string;
	containerPath: string;
}

interface ListHostValues {
	devices: DeviceRow[];
}

// The list host mirrors the create-surface adapters: it owns the form and
// the field array, renders the row's controls through the render seam,
// and appends the row shape itself — the module never calls form hooks.
function ListHost({
	onSubmit,
	addArea,
}: {
	onSubmit: (values: ListHostValues) => void;
	addArea?: React.ReactNode;
}) {
	const { register, control, handleSubmit } = useForm<ListHostValues>({
		defaultValues: { devices: [] },
	});
	const rows = useFieldArray({ control, name: "devices" });
	return (
		<form onSubmit={handleSubmit(onSubmit)}>
			<RowListEditor
				rows={rows}
				noun="device"
				addLabel="Add device"
				removeAriaLabel={(rowNumber) => `Remove device ${rowNumber}`}
				renderRow={(index) => (
					<RowInput
						mono
						placeholder="Host path"
						ariaLabel={`Device host path ${index + 1}`}
						registration={register(`devices.${index}.hostPath` as const)}
					/>
				)}
				onAdd={() => rows.append({ hostPath: "", containerPath: "" })}
				addArea={addArea}
			/>
			<button type="submit" data-testid="host-submit">
				Submit
			</button>
		</form>
	);
}

describe("RowListEditor", () => {
	it("derives the add/remove testids and the remove aria label from its props", async () => {
		renderWithQuery(<ListHost onSubmit={vi.fn()} />);
		const user = userEvent.setup();

		const addButton = screen.getByTestId("add-device");
		expect(addButton).toHaveTextContent("Add device");

		await user.click(addButton);

		expect(screen.getByTestId("remove-device-0")).toHaveAttribute(
			"aria-label",
			"Remove device 1",
		);
	});

	it("adds rows and removes them by index, re-numbering the remaining rows", async () => {
		const onSubmit = vi.fn();
		renderWithQuery(<ListHost onSubmit={onSubmit} />);
		const user = userEvent.setup();

		await user.click(screen.getByTestId("add-device"));
		await user.click(screen.getByTestId("add-device"));
		await user.type(screen.getByLabelText("Device host path 1"), "first");
		await user.type(screen.getByLabelText("Device host path 2"), "second");

		await user.click(screen.getByTestId("remove-device-0"));

		expect(
			screen.queryByLabelText("Device host path 2"),
		).not.toBeInTheDocument();
		expect(screen.getByLabelText("Device host path 1")).toHaveValue("second");
		expect(screen.getByTestId("remove-device-0")).toBeInTheDocument();
		expect(screen.queryByTestId("remove-device-1")).not.toBeInTheDocument();
	});

	it("renders the add-area slot beside the add button for sibling actions", () => {
		renderWithQuery(
			<ListHost
				onSubmit={vi.fn()}
				addArea={
					<button type="button" data-testid="sibling-action">
						Create volume
					</button>
				}
			/>,
		);

		const addButton = screen.getByTestId("add-device");
		const sibling = screen.getByTestId("sibling-action");
		expect(sibling).toBeInTheDocument();
		// Beside: the slot shares the add button's row.
		expect(sibling.parentElement).toBe(addButton.parentElement);
	});
});

interface MountRowValues {
	source: string;
	selinuxShared: boolean;
	selinuxPrivate: boolean;
	chownUser: boolean;
}

interface FlagsHostValues {
	mounts: MountRowValues[];
}

// The flags host mirrors the create-surface adapters: it owns the form,
// watches the rows, and hands over setValue — the flags block is generic
// over the surface's form values and never calls form hooks itself.
function FlagsHost() {
	const { control, watch, setValue } = useForm<FlagsHostValues>({
		defaultValues: {
			mounts: [
				{
					source: "",
					selinuxShared: false,
					selinuxPrivate: false,
					chownUser: false,
				},
			],
		},
	});
	const rows = useFieldArray({ control, name: "mounts" });
	const mountValues = watch("mounts");
	return (
		<>
			{rows.fields.map((field, index) => (
				<MountFlags
					key={field.id}
					name="mounts"
					index={index}
					row={mountValues?.[index]}
					setValue={setValue}
				/>
			))}
		</>
	);
}

describe("MountFlags", () => {
	it("renders the z/Z/U checkboxes with their aria labels", () => {
		renderWithQuery(<FlagsHost />);

		expect(
			screen.getByRole("checkbox", { name: "Bind mount 1 SELinux shared (z)" }),
		).not.toBeChecked();
		expect(
			screen.getByRole("checkbox", {
				name: "Bind mount 1 SELinux private (Z)",
			}),
		).not.toBeChecked();
		expect(
			screen.getByRole("checkbox", {
				name: "Bind mount 1 chown to user (U)",
			}),
		).not.toBeChecked();
	});

	it("checking z unchecks Z and checking Z unchecks z, while U stays independent", async () => {
		renderWithQuery(<FlagsHost />);
		const user = userEvent.setup();

		const z = screen.getByRole("checkbox", {
			name: "Bind mount 1 SELinux shared (z)",
		});
		const bigZ = screen.getByRole("checkbox", {
			name: "Bind mount 1 SELinux private (Z)",
		});
		const u = screen.getByRole("checkbox", {
			name: "Bind mount 1 chown to user (U)",
		});

		await user.click(z);
		expect(z).toBeChecked();
		expect(bigZ).not.toBeChecked();

		await user.click(bigZ);
		expect(bigZ).toBeChecked();
		expect(z).not.toBeChecked();

		await user.click(u);
		expect(u).toBeChecked();
		expect(bigZ).toBeChecked();
		expect(z).not.toBeChecked();
	});
});
