import { paraglideVitePlugin } from '@inlang/paraglide-js';
import { sveltekit } from '@sveltejs/kit/vite';
import tailwindcss from '@tailwindcss/vite';
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
	plugins: [
		paraglideVitePlugin({ project: './project.inlang', outdir: './src/lib/paraglide' }),
		tailwindcss(),
		sveltekit()
	]
});
