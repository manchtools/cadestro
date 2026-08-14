# Cadestro wire contract

The protocol Cadestro speaks: protobuf sources, the generated Go and
TypeScript clients, the agent stream client, and the request validation that
sits on the transport boundary.

MIT, and a leaf module — it imports nothing else in this repository. Anything
that has to interoperate with Cadestro speaks this contract, so implementing
the protocol must never impose an obligation or drag in the rest of the system.
See [`../LICENSING.md`](../LICENSING.md).

## Runtime contract

- Protobuf sources live in `proto/cadestro/v1/` under package
  `cadestro.v1`.
- Generated Go and TypeScript packages live in `gen/go/cadestro/v1/` and
  `gen/ts/cadestro/v1/`.
- `AgentService` exposes one bidirectional `Stream`. Handshake, synchronization,
  heartbeats, manifest delivery and receipts, results, secret operations, and
  terminal traffic are frames on that stream.
- Agent certificates authenticate the direct mTLS connection. Application
  frames are not separately signed.
- Fields classified as secret use the versioned X25519 `SealedValue` envelope
  with context-bound associated data.
- Human authentication is OIDC-based; the contract has no local password or
  TOTP RPCs.
- The exact current RPC set is pinned by `testdata/rpc_golden.json`. The separate
  predecessor golden exists only to prove the approved deletion sets; it is not
  a compatibility surface.

## Layout

| Path | Purpose |
|------|---------|
| `proto/cadestro/v1/` | Contract source |
| `gen/go/cadestro/v1/` | Generated protobuf and Connect Go packages |
| `gen/ts/cadestro/v1/` | Generated TypeScript messages |
| `client.go` | Agent-side stream client and correlated stream operations |
| `validate/` | Request validation used at the transport boundary |
| `maintenance/` | Maintenance-window semantics over the contract types |
| `ts/` | Browser client, auth storage, errors, logging, and exports |
| `testdata/` | The RPC surface goldens |

## Generate

Install the lockfile-pinned JavaScript tools, then regenerate both languages:

```bash
npm ci
make generate
```

`make generate` runs protobuf generation, injects Go validation tags, formats
the Go output, and generates TypeScript with the same pinned Buf tool used by
CI. The code-generating plugins are pinned to the versions `go.mod` already
records, so bumping the module moves the plugin with it. Generated files are
committed; never hand-edit `gen/`.

## Verify

```bash
./scripts/verify.sh
```

It checks formatting, build, vet, static analysis, Go tests, Buf lint and
format, generated-code drift, TypeScript typechecking, and TypeScript tests.
`GOWORK=off` is intentional so the result matches how a consumer outside this
repository resolves the module.
