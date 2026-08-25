.PHONY: dev build preview check check-watch install generate clean

install:
	npm install

dev:
	PUBLIC_SKIP_FEATURE_FLAGS=true \
	PUBLIC_ENABLE_MARKETPLACE=$${PUBLIC_ENABLE_MARKETPLACE:-true} \
	PUBLIC_MARKETPLACE_URL=$${PUBLIC_MARKETPLACE_URL:-http://marketplace.localhost:5180} \
	npm run dev

build:
	npm run build

preview:
	npm run preview

check:
	npm run check

check-watch:
	npm run check:watch

generate:
	$(MAKE) -C ../contract generate-ts

clean:
	rm -rf .svelte-kit build
