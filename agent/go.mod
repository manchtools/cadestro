module github.com/manchtools/cadestro/agent

go 1.25.12

require (
	github.com/manchtools/cadestro/contract v0.0.0
	github.com/manchtools/cadestro/sdk v0.0.0
	github.com/oklog/ulid/v2 v2.1.2
	github.com/pressly/goose/v3 v3.27.3
	github.com/stretchr/testify v1.11.1
	google.golang.org/protobuf v1.36.12
	modernc.org/sqlite v1.55.0
)

replace github.com/manchtools/cadestro/contract => ../contract

replace github.com/manchtools/cadestro/sdk => ../sdk

require (
	buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go v1.36.11-20260709200747-435963d16310.1 // indirect
	buf.build/go/protovalidate v1.3.0 // indirect
	cel.dev/expr v0.25.1 // indirect
	connectrpc.com/connect v1.20.0 // indirect
	github.com/antlr4-go/antlr/v4 v4.13.1 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/go-cmd/cmd v1.4.3 // indirect
	github.com/google/cel-go v0.30.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/mattn/go-isatty v0.0.23 // indirect
	github.com/mfridman/interpolate v0.0.2 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/sethvargo/go-retry v0.4.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/exp v0.0.0-20260718201538-764159d718ef // indirect
	golang.org/x/net v0.58.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.41.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20250811230008-5f3141c8851a // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260720211330-0afa2a65878a // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	modernc.org/libc v1.74.3 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
)
