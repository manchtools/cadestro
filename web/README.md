# Cadestro Web

The web frontend for Cadestro, built with SvelteKit 2, Svelte 5, and TailwindCSS 4. Communicates with the Control Server via Connect-RPC.

The web is an RPC consumer of the contract in `../contract/` and creates no
server history, rollout, or analytics API of its own.

## Tech Stack

- **Framework**: SvelteKit 2 with Svelte 5
- **Styling**: TailwindCSS 4 with tailwind-variants
- **UI Components**: bits-ui
- **API Client**: @connectrpc/connect-web
- **i18n**: @inlang/paraglide-js (English, German)
- **Icons**: Lucide
- **PWA**: vite-plugin-pwa
- **Runtime**: Bun (CI and Docker build); npm + Node for local development

## Development

```bash
# Install dependencies
npm install

# Start development server
npm run dev

# Type check
npm run check

# Build for production
npm run build

# Preview production build
npm run preview
```

## Features

- **Fleet home** — the devices route is a semantic-zoom fleet surface, not a device table: one snapshot rendered as status tiles, group bubbles or worst-first rows, with the zoom level carried in the URL. Status is never colour alone — every tone also carries a shape and a word
- **Device management** — labels, multi-user assignment, device groups (static + dynamic), sync intervals
- **Device detail view** — groups tab, compliance tab with policy inheritance, remote log queries (journalctl)
- **Action management** — 16 action types (packages, shell, systemd, files, users, SSH, sudo, LPS, LUKS, etc.)
- **Action sets and definitions** — ordered collections with drag-to-reorder
- **Assignment policies** — source-to-target with REQUIRED/AVAILABLE/EXCLUDED modes, batch support
- **Assign surface** — one page for two targeting modes: a *carried* selection handed over from the fleet (these devices, now) and a *rule* that compiles to a dynamic device group and keeps applying (whatever matches, now and later — gated by an explicit confirm)
- **Operations feed** — executions cluster into operation cards instead of reading as a log; the same Search RPC still owns query, filters, sorting and paging underneath
- **Compliance policies** — rules with grace periods, per-device evaluation status, policy inheritance through device groups
- **Dynamic RBAC** — custom roles, user groups with additive permissions, per-permission granularity. The role editor is a permission matrix whose groups are discovered from `ListPermissions`, and it commits from the pill rather than from a Save button
- **OIDC login** — identity providers, identity linking, and auto-created users; MFA stays at the IdP. There is no manual user creation; JIT-created users can be erased locally, SCIM-managed users are deprovisioned by the identity provider
- **SCIM v2 provisioning** — enable/disable, token rotation, group mapping
- **Full-text search** — the stable Search RPC, backed by SQLite FTS5 on the control server
- **Audit log viewer** — operation/effect evidence without exposing secrets
- **Skeleton loading states** — all searchable pages use skeleton tables during load
- **Row-list grammar** — entity lists are dense headerless rows, not tables: an icon or status tile, the name over its ULID, chips for state, a right-aligned stamp, and the former column headers moved into a sort bar
- **Onboarding** — a first-run welcome per (server, user), a coach-mark tour bound to anchors that really exist on the page (a missing anchor drops its step instead of pointing at nothing), and a getting-started checklist on an empty fleet
- **Paginated, filterable tables** — one shared list architecture on every entity page: server-side search, filtering (including date ranges), sorting, and pagination through the Search RPC's entity facets; the non-search-backed admin lists (roles, tokens, identity providers) use the same list UI over client-side state
- **LPS/LUKS secret access** — metadata lists with per-entry reveal; every reveal is an audited sensitive read on the server, and plaintext never appears in list responses
- **i18n** — English, German (error codes, UI labels, action types)
- **Error codes** — localized error messages from Connect-RPC error details

The settled shell has no persistent left navigation. A top-centre pill shows
exactly one of four modes, derived from its own state rather than stored
separately: **nav** at rest, **search** on Command-K, **selection** while a
multi-select is live, and **context** while a surface holds uncommitted state.
A context pill carries the commit (Save), Cancel and a third exit — Stash,
which parks the unfinished edit on the stage rail and restores it with its
buffers intact — so editors such as the role matrix and the identity-provider
form carry no Save button of their own. Under the pill a caption strip reports
the active mode's own subtext: a validation rollup, a compiled rule's live
count, or a selection's implications. Movable/minimizable work surfaces stay
alive across navigation, and terminal windows persist for their session.

## Project Structure

```
src/
├── lib/
│   ├── components/      UI components (ItemTablePicker, ActionDetailSheet, etc.)
│   ├── sdk/             Svelte 5 wrapper around the contract's plain-TS client
│   ├── errors.ts        Error code → i18n key mapping
│   ├── paraglide/       Paraglide compiler output (generated, gitignored)
│   └── ...
├── routes/              SvelteKit routes
└── app.html             HTML template

messages/                i18n message files at repo root (en.json, de.json)
../contract/gen/ts/      Generated protobuf types, resolved via the $contract alias
../contract/ts/          The contract's plain-TS client, resolved via $contractClient
```

## Regenerating Types

The protobuf definitions live in the contract module, and its Makefile owns
generation — nothing here regenerates them:

```bash
make -C ../contract generate-ts
```

That writes `../contract/gen/ts/`, which this app imports through the
`$contract` alias. The contract's hand-written client (`../contract/ts/`) is
imported through `$contractClient`. Both aliases are declared in
`svelte.config.js`; there is no npm dependency on the contract, so no registry
package has to exist for a build to resolve it.

## Environment Variables

| Variable | Description |
|----------|-------------|
| `PUBLIC_CONTROL_URL` | Origin of the control server this UI talks to, read at runtime by the server that serves the app. Set, it preconfigures the browser so a fresh install never stops at `/setup`; unset, the app asks for the URL there as before. The reference deployment renders it from `CONTROL_DOMAIN` and serves the UI on that same origin. |
| `BASE_PATH` | Base path prefix for non-root deployments (default: `/`). Affects SvelteKit `base`, the PWA scope, and the version-pinning cookie path. |
| `CADESTRO_DEV_AUTH_TOKEN` | Server-side-only token shared with a control `devauth` build. Use at least 32 random bytes; the loopback-only Vite proxy injects it into `/dev/session` and forwards the original client address without exposing the token to browser code. |
| `VITE_DEV_CONTROL_URL` | Control target for the local Vite proxy (default: `https://127.0.0.1:8081`). |
| `VITE_SKIP_AUTH` | **Temporary, dev-only.** `1` seeds a fake admin session so the UI starts without a control server (RPCs still fail; pages show their empty/error states). `make dev` sets it by default; use `make dev VITE_SKIP_AUTH=0` (or plain `npm run dev`) for the real login flow. Compile-time-guarded via `import.meta.env.DEV` — it has no effect in production builds. Close the tab or log out to drop the fake session. |

The control server URL is a runtime value, never a build-time one. `PUBLIC_CONTROL_URL`
seeds it once, before the first route guard runs; the operator can still change
it from Settings, which returns to `/setup`, and the choice is persisted to
`localStorage` from then on. Without that variable the app behaves as it always
did and starts at `/setup`.

## Building

```bash
npm run build
```

`svelte-adapter-bun` writes a server into `build/`, started with
`bun run build/index.js` (`PORT` defaults to 3000).

The released image is built from the repository root, because the app compiles
against `../contract`:

```bash
docker build -f web/Containerfile --build-arg VERSION=v2026.08 -t cadestro-web .
```

It is published as `ghcr.io/manchtools/cadestro-web` under the same tag as the
control image, and the reference deployment in `../server/deploy` runs the two
side by side on one hostname.
