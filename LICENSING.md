# Licensing

This repository is **not** under a single license. Each module carries its own,
and the split is deliberate.

| Module | License | Why |
|---|---|---|
| `contract/` | MIT | The wire contract. Anything that must interoperate with Cadestro has to speak it, so it is permissive: implementing the protocol must never impose obligations. |
| `sdk/` | MIT | Reusable system capabilities. Intended to be embedded in other people's tools; copyleft here would defeat the purpose. |
| `agent/` | GPL-3.0 | Ships to managed devices. Modifications distributed to others must stay open. |
| `server/` | AGPL-3.0 | The control plane. Network use counts as distribution — running a modified control plane as a service carries the same obligation as shipping it. |
| `web/` | AGPL-3.0 | The administration UI is served operator-facing software, the same case as the server. |

Each module's `LICENSE` file is authoritative for that module. When in doubt
about a file, the governing license is the one in the nearest enclosing module
directory.

**Dependency direction and license flow.** `contract/` and `sdk/` are
independent Go leaves. `agent/` and `server/` depend on both; `web/` consumes
the generated TypeScript contract. This keeps permissive modules below the
copyleft consumers.
