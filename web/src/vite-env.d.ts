/// <reference types="vite/client" />

declare module 'virtual:pwa-register' {
	export interface RegisterSWOptions {
		immediate?: boolean;
		onRegisteredSW?: (swUrl: string, r?: ServiceWorkerRegistration) => void;
		onRegisterError?: (error: Error) => void;
		onOfflineReady?: () => void;
		onNeedRefresh?: () => void;
	}

	export function registerSW(options?: RegisterSWOptions): (reloadPage?: boolean) => Promise<void>;
}

declare module 'virtual:pwa-info' {
	export const pwaInfo:
		| {
				webManifest: {
					href: string;
					useCredentials: boolean;
					linkTag: string;
				};
		  }
		| undefined;
}
