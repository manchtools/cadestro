import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

const control = process.env.VITE_DEV_CONTROL_URL || 'https://localhost:8081';

export default defineConfig({
	server: {
		host: '127.0.0.1',
		allowedHosts: ['.localhost'],
		proxy: {
			'/cadestro.v1.ControlService': { target: control, changeOrigin: true },
			'/health': { target: control, changeOrigin: true }
		}
	},
	plugins: [sveltekit()]
});
