




.PHONY: sqlc-generate sqlc-check help







SQLC_IMAGE ?= sqlc/sqlc:1.31.1
DOCKER ?= docker

help:
	@echo "Targets:"
	@echo "  sqlc-generate  regenerate internal/store/generated/ via the pinned sqlc image"
	@echo "  sqlc-check     fail if generated code is stale (run sqlc-generate first)"












sqlc-generate:
	@set -eu; \
	stage=$$(mktemp -d internal/store/.sqlc.XXXXXX); \
	trap 'rm -rf "$$stage"' EXIT INT TERM; \
	cp -R internal/store/sqlc.yaml internal/store/migrations internal/store/queries "$$stage/"; \
	$(DOCKER) run --rm --memory=6g --cpus=4 --user "$$(id -u):$$(id -g)" \
		-v "$$(realpath "$$stage")":/src:Z -w /src $(SQLC_IMAGE) generate; \
	mv internal/store/generated "$$stage/previous"; \
	if mv "$$stage/generated" internal/store/generated; then \
		rm -rf "$$stage"; \
		trap - EXIT INT TERM; \
	else \
		mv "$$stage/previous" internal/store/generated; \
		exit 1; \
	fi





sqlc-check: sqlc-generate
	git diff --exit-code internal/store/generated
