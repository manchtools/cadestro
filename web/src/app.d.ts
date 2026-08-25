

declare global {
	const __APP_VERSION__: string;
	const __MARKETPLACE_URL__: string;
	const __BASE_PATH__: string;

	namespace App {

		interface PageState {
			actionSheet?: string;
			compliancePolicySheet?: string;
		}

	}
}

export {};
