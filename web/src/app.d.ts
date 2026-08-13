// See https://svelte.dev/docs/kit/types#app.d.ts
// for information about these interfaces
declare global {
	const __APP_VERSION__: string;
	const __MARKETPLACE_URL__: string;
	const __BASE_PATH__: string;

	namespace App {
		// interface Error {}
		// interface Locals {}
		// interface PageData {}
		interface PageState {
			actionSheet?: string;
			compliancePolicySheet?: string;
		}
		// interface Platform {}
	}
}

export {};
