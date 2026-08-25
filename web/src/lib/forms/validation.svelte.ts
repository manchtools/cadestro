import { type z } from 'zod';

export type FieldErrors<T> = Partial<Record<keyof T, string>>;

export interface FormValidation<T> {
	readonly errors: FieldErrors<T>;
	readonly hasErrors: boolean;
	validate(data: T): boolean;
	validateField(field: keyof T, value: unknown, data?: Partial<T>): boolean;
	clearErrors(): void;
	clearFieldError(field: keyof T): void;
	handleSubmit(data: T, handler: (data: T) => Promise<void>): Promise<void>;
}

export function createFormValidation<T extends z.ZodTypeAny>(
	schema: T
): FormValidation<z.infer<T>> {
	let errors = $state<FieldErrors<z.infer<T>>>({});

	const hasErrors = $derived(Object.keys(errors).length > 0);

	function validate(data: z.infer<T>): boolean {
		const result = schema.safeParse(data);
		if (result.success) {
			errors = {};
			return true;
		}

		const fieldErrors: FieldErrors<z.infer<T>> = {};
		const flattened = result.error.flatten();

		for (const [key, messages] of Object.entries(flattened.fieldErrors)) {
			const msgs = messages as string[] | undefined;
			if (msgs && msgs.length > 0) {
				(fieldErrors as Record<string, string>)[key] = msgs[0];
			}
		}

		for (const issue of result.error.issues) {
			if (issue.path.length > 0) {
				const key = String(issue.path[0]);
				if (!(key in fieldErrors)) {
					(fieldErrors as Record<string, string>)[key] = issue.message;
				}
			}
		}

		errors = fieldErrors;
		return false;
	}

	function validateField(
		field: keyof z.infer<T>,
		value: unknown,
		_data?: Partial<z.infer<T>>
	): boolean {

		const shape = getSchemaShape(schema);
		if (shape && field in shape) {
			const fieldSchema = shape[field as string];
			const result = fieldSchema.safeParse(value);
			if (result.success) {
				const next = { ...errors };
				delete next[field];
				errors = next;
				return true;
			} else {
				errors = { ...errors, [field]: result.error.issues[0]?.message ?? 'Invalid' };
				return false;
			}
		}

		return true;
	}

	function clearErrors(): void {
		errors = {};
	}

	function clearFieldError(field: keyof z.infer<T>): void {
		const next = { ...errors };
		delete next[field];
		errors = next;
	}

	async function handleSubmit(
		data: z.infer<T>,
		handler: (data: z.infer<T>) => Promise<void>
	): Promise<void> {
		if (validate(data)) {
			await handler(data);
		}
	}

	return {
		get errors() {
			return errors;
		},
		get hasErrors() {
			return hasErrors;
		},
		validate,
		validateField,
		clearErrors,
		clearFieldError,
		handleSubmit
	};
}

function getSchemaShape(
	schema: z.ZodTypeAny
): Record<string, z.ZodTypeAny> | null {
	if ('shape' in schema && typeof schema.shape === 'object') {
		return schema.shape as Record<string, z.ZodTypeAny>;
	}

	if ('_def' in schema) {
		const def = schema._def as unknown as Record<string, unknown>;
		if ('schema' in def) {
			const inner = def.schema;
			if (inner && typeof inner === 'object' && 'shape' in inner) {
				return (inner as Record<string, unknown>).shape as Record<string, z.ZodTypeAny>;
			}
		}
	}
	return null;
}
