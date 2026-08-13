.PHONY: dev build preview check check-watch install generate clean

# Install npm dependencies (including SDK for type resolution)
install:
	npm install
	cd ../sdk && npm install

# Development server (all feature flags enabled).
# Points the marketplace iframe at the locally running marketplace
# web dev server. Default URL uses a "marketplace.localhost" hostname
# so the cookie-based publisher session sharing works in dev —
# *.localhost resolves to 127.0.0.1 natively in Chrome / Firefox /
# Safari (RFC 6761), and a Domain=.localhost cookie set by the
# marketplace tab is picked up by any *.localhost iframe.
#
# Visit this app via http://pm.localhost:5173 (pair the two
# *.localhost labels so the cookie's Domain=.localhost scope covers
# both).
#
# Override either var on the command line when running against a
# different marketplace instance, e.g.:
#   make dev PUBLIC_MARKETPLACE_URL=https://marketplace.example.com
#
# VITE_SKIP_AUTH defaults to 1 here (TEMPORARY: fake admin session so the
# UI starts without a control server; compile-time dev-only, see
# src/lib/dev/skip-auth.ts). Run `make dev VITE_SKIP_AUTH=0` for the real
# login flow.
dev:
	PUBLIC_SKIP_FEATURE_FLAGS=true \
	PUBLIC_ENABLE_MARKETPLACE=$${PUBLIC_ENABLE_MARKETPLACE:-true} \
	PUBLIC_MARKETPLACE_URL=$${PUBLIC_MARKETPLACE_URL:-http://marketplace.localhost:5180} \
	VITE_SKIP_AUTH=$${VITE_SKIP_AUTH:-1} \
	npm run dev

# Production build
build:
	npm run build

# Preview production build
preview:
	npm run preview

# Type check
check:
	npm run check

# Type check (watch mode)
check-watch:
	npm run check:watch

# Regenerate TypeScript types from protobuf definitions
generate:
	cd ../sdk && npm install && make generate-ts

# Clean build artifacts
clean:
	rm -rf .svelte-kit build
