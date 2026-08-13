# Web e2e suite — behavioural / interaction tests

Exercises real user interactions (typing, clicking sort headers, ticking
filters, paging, opening dialogs, confirming deletes) and asserts the outcome
**deterministically** — by the exact RPC the interaction emits and the resulting
DOM state, never by pixels.

The whole Control API is mocked (Connect-RPC route interception), an admin
session is pre-seeded into `localStorage`, and the browser clock is frozen, so
the suite is hermetic (no backend) and reproducible.

> Local-only — not wired into CI.

## Running

```bash
npm run test:e2e       # run all behavioural tests
npm run test:e2e:ui    # Playwright UI mode (pick / inspect / step through)
npx playwright test tests/e2e/list-interactions.spec.ts   # one file
```

## How interactions are asserted deterministically

The key idea: **a click is verified by the request it produces.** `recordRpc`
captures the decoded body of every call to one RPC, then defers to the existing
mock — so capturing never changes the response. Then poll the captured calls:

```ts
const search = recordRpc(page, 'Search');
await page.getByText('No assigned actions').click();
await expect.poll(() => search.at(-1)?.tagFilters?.member_count).toBe('0');
```

No timing guesses, no pixel diffs — the assertion is the concrete wire effect of
the click. DOM-state outcomes (navigation, toasts, gated nav links) are asserted
directly (`toHaveURL`, an error/success toast, link presence).

## What it covers

| File | Covers |
|------|--------|
| `list-interactions.spec.ts` | search → query · sort header → `sortField`/`sortDirection` (asc↔desc) · **empty-relation filters (#325)** → `member_count`/`rule_count = 0` · "not in action set" → `assigned=false` · pagination → page token · row → detail nav |
| `dialog-flows.spec.ts` | delete (row menu → confirm → `DeleteDevice` + success toast) · create (dialog → fill → `CreateRole`) |
| `error-handling.spec.ts` | a mutation RPC forced to fail → the page surfaces an **error toast** (inventory refresh, rebuild index, delete-with-confirm) |
| `rbac.spec.ts` | a non-admin session with a restricted permission set → nav exposes only permitted sections |

## Adding cases

- **An interaction asserted by its RPC** — in `list-interactions.spec.ts`, use
  `recordRpc(page, '<Method>')` then `expect.poll` the captured body.
- **A mutation-failure case** — append to `CASES` in `error-handling.spec.ts`:
  `{ name, path, waitFor, failRpc, trigger }`. `trigger` is the click(s) that fire
  the mutation (single button, or the open→confirm steps for a dialog).
- **A dialog flow** — `clickUntil(trigger, appears)` opens bits-ui menus/dialogs
  robustly (it retries the first click, which Svelte 5 + Playwright can drop).
- **An RBAC check** — `preparePageAs(page, theme, ['ListX', ...])` seeds a
  non-admin session with exactly those permissions.

## Determinism

- **Frozen clock** — `page.clock.setFixedTime(REFERENCE_NOW_MS)` pins relative
  timestamps; `setFixedTime` keeps timers running so the 300ms search debounce
  still fires.
- **Mocked RPCs** with fixed fixtures.
- **No reliance on pixels** — so restyling never breaks these tests.

## Files

| File | Role |
|------|------|
| `fixtures.ts` | `preparePage`/`preparePageAs`, `gotoAndSettle`, `recordRpc`, `failRpc`, `clickUntil`, toast asserts |
| `../showcase/bootstrap.ts` | seeds the session (admin or custom-permission) into `localStorage` |
| `../showcase/mocks.ts` | Connect-RPC mock handlers |
| `../showcase/dummy.ts` | fixture data + `REFERENCE_NOW_MS` |

> `tests/showcase/` also holds the **documentation / marketing screenshot**
> generators (`*.spec.ts` using raw `page.screenshot()`). Those are unrelated to
> regression testing — run them with `npx playwright test tests/showcase`.
