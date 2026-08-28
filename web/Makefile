.PHONY: dev build preview check install generate clean

install:
	npm install

dev:
	npm run dev

build:
	npm run build

preview:
	npm run preview

check:
	npm run check

generate:
	$(MAKE) -C ../contract generate-ts

clean:
	rm -rf .svelte-kit build
