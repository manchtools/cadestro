.PHONY: generate generate-go generate-ts clean install-tools

PROTO_DIR := proto
GEN_DIR := gen
TOOLS_DIR := .tools/bin

MAKEFILE_DIR := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))
BUF := $(MAKEFILE_DIR)scripts/buf.sh

install-tools:
	GOBIN=$(MAKEFILE_DIR)$(TOOLS_DIR) go install google.golang.org/protobuf/cmd/protoc-gen-go@$$(go list -m -f '{{.Version}}' google.golang.org/protobuf)
	GOBIN=$(MAKEFILE_DIR)$(TOOLS_DIR) go install connectrpc.com/connect/cmd/protoc-gen-connect-go@$$(go list -m -f '{{.Version}}' connectrpc.com/connect)

generate:
	$(MAKE) generate-go
	$(MAKE) generate-ts

generate-go:
	@rm -rf $(GEN_DIR)
	$(BUF) generate --template buf.gen.go.yaml

generate-ts:
	$(BUF) generate --include-imports --template buf.gen.ts.yaml

clean:
	rm -rf $(GEN_DIR)
