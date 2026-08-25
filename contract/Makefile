.PHONY: generate generate-go generate-ts clean install-tools

PROTO_DIR := proto
GEN_DIR := gen

MAKEFILE_DIR := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))
BUF := $(MAKEFILE_DIR)scripts/buf.sh

install-tools:
	go install google.golang.org/protobuf/cmd/protoc-gen-go@$$(go list -m -f '{{.Version}}' google.golang.org/protobuf)
	go install connectrpc.com/connect/cmd/protoc-gen-connect-go@$$(go list -m -f '{{.Version}}' connectrpc.com/connect)

generate:
	$(MAKE) generate-go
	$(MAKE) generate-ts

generate-go:
	@rm -f $(GEN_DIR)/go/cadestro/v1/*.pb.go $(GEN_DIR)/go/cadestro/v1/cadestrov1connect/*.connect.go
	@mkdir -p $(GEN_DIR)/go/cadestro/v1
	$(BUF) generate --template buf.gen.go.yaml

generate-ts:
	@rm -f $(GEN_DIR)/ts/cadestro/v1/*_pb.ts
	$(BUF) generate --include-imports --template buf.gen.ts.yaml

clean:
	rm -rf $(GEN_DIR)
