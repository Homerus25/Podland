import { rowScoped } from "@shared/queries/rowScopedMutation";
import { describe, expect, it } from "vitest";

// The projection is pure, so its suite needs no DOM and no QueryClient:
// each case hands it the structural slice a react-query mutation result
// carries (isPending, error, variables) and asserts the derived row state.
const identity = (variables: string) => variables;

describe("rowScoped", () => {
	it("scopes pending to the row whose key the in-flight variables carry", () => {
		const inFlight = {
			isPending: true,
			error: null,
			variables: "pod-1",
		};

		expect(rowScoped(inFlight, "pod-1", identity)).toEqual({
			isPending: true,
			error: null,
		});
		expect(rowScoped(inFlight, "pod-2", identity)).toEqual({
			isPending: false,
			error: null,
		});
	});

	it("drops pending for the matching row once the mutation settles", () => {
		const settled = {
			isPending: false,
			error: null,
			variables: "pod-1",
		};

		expect(rowScoped(settled, "pod-1", identity)).toEqual({
			isPending: false,
			error: null,
		});
	});

	it("scopes the error to the row whose key the variables carry, including after settle", () => {
		const failure = new Error("pod has containers with non-zero exit codes");
		// react-query keeps `variables` after the mutation settles, which is
		// what makes the error scoping work: the failed row still recognizes
		// the call as its own.
		const settled = {
			isPending: false,
			error: failure,
			variables: "pod-1",
		};

		expect(rowScoped(settled, "pod-1", identity)).toEqual({
			isPending: false,
			error: failure,
		});
		expect(rowScoped(settled, "pod-2", identity)).toEqual({
			isPending: false,
			error: null,
		});
	});

	it("extracts the row key from object variables through the caller's keyOf", () => {
		const inFlight = {
			isPending: true,
			error: null,
			variables: { quadletId: "web", timeout: 30 },
		};
		const quadletId = (variables: { quadletId: string }) => variables.quadletId;

		expect(rowScoped(inFlight, "web", quadletId)).toEqual({
			isPending: true,
			error: null,
		});
		expect(rowScoped(inFlight, "db", quadletId)).toEqual({
			isPending: false,
			error: null,
		});
	});

	it("reports false and null for every row while variables are undefined", () => {
		const untouched = {
			isPending: true,
			error: new Error("boom"),
			variables: undefined,
		};

		expect(rowScoped(untouched, "pod-1", identity)).toEqual({
			isPending: false,
			error: null,
		});
		expect(rowScoped(untouched, "pod-2", identity)).toEqual({
			isPending: false,
			error: null,
		});
	});
});
