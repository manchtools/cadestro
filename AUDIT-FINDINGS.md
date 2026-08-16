# Answers after your audit

Ten read-only investigations against the tree at `99ab489`, one per question
cluster plus a meta-pattern sweep over everything you did not list. Nothing was
implemented. Verdict vocabulary: **RIGHT** (your reading holds), **INVERTED**
(the defect is real but the mechanism is the opposite of your reading),
**MISREAD** (code refutes the premise; the citation shows why the misreading
was easy), **DOC** (editorial fix), **GAP** (real missing behavior),
**DECISION** (a coherent alternative that trades away a named guard — yours to
rule).

---

## 1. The headline: what your audit caught

### Launch blockers (corroborated independently by two investigators)

1. **The documented control-plane install command downloads the agent
   installer.** `README.md:39` and `docs/installation.md:36` fetch
   `releases/latest/download/install.sh`; the release workflow uploads only
   `agent/install.sh` under that name (`.github/workflows/release.yml:314`),
   and `/releases/latest/` skips `-rc` prereleases anyway. There is currently
   no published route to `server/deploy/install.sh` at all.
2. **The documented dev-auth activation path is dead.**
   `CADESTRO_DEV_AUTH`/`CADESTRO_DEV_AUTH_TOKEN`
   (`server/cmd/cadestro/devauth.go:36-37`) are absent from
   `configEnvironment` (`config.go:47-80`), and `config.go:189-190` exits on
   any undeclared `CADESTRO_` variable — setting the documented vars is a
   startup failure.

### Real design defects your items surfaced (mechanism sometimes inverted)

3. **Assignments are inert: the only thing that turns them into work is an RPC
   nothing calls** (your item 10). Creating an assignment does not make the
   device process it on sync — sync only refreshes already-compiled deliveries
   (`server/internal/agentsync/service.go:69`). The single compiler is
   `assignedManifests` (`server/internal/dispatch/assigned.go:15`), whose only
   caller is the `DispatchAssignedActions` handler (`handlers.go:455`) — and
   **no shipped client invokes that RPC**: outside generated code it appears
   only in the handler, its mount, its permission entry, one server test, an
   unused TS client method, and a permission-name fixture. The web UI's assign
   flow issues `dispatchActionSet` for a direct device target with schedule
   "now" (`web/src/routes/(app)/assign/assign-data.ts:160-182`) and dispatches
   nothing at all for group targets. A new device joining an assigned group
   gets nothing, ever.

   **Operator ruling: `DispatchAssignedActions` should not exist.** An
   assignment is declarative desired state; compiling it is the server's job on
   sync, not an operator-invoked verb. This is the same position as your item
   22 (control stores policy; the only control→agent push is instant actions
   and terminal). Consequences to design against, not objections: compilation
   moves into the sync path, so a manifest needs a *content-stable* identity
   rather than a per-dispatch ULID (the agent already rejects a same-id
   delivery whose bytes differ, `agent/internal/store/manifest.go:87-98`), and
   a per-device generation counter (bumped by assignment / membership /
   definition edits) keeps re-compilation from running on every sync for every
   device. With those, the hole closes structurally instead of by remembering
   to call an RPC.
4. **Maintenance windows implement the opposite of what the docs claim**
   (your item 26, INVERTED). The docs say an unwindowed group unrestricts the
   device; the code filters empty windows *before* the union
   (`queries/devices.sql:290,296`; `agentsync/service.go:115-130`), so the
   union's "any empty ⇒ always allowed" branch
   (`contract/maintenance/window.go:85-96`) is unreachable. Reality: one
   windowed group freezes work assigned through every other group, per-device,
   not per-path (`agent/internal/scheduler/scheduler.go:182,190`). Bonus
   defects: a user-group's window gates devices it assigns nothing to; all
   deferred manifests fire simultaneously when the window opens; deferred work
   leaves no server-visible trace.
5. **The compliance verdict is agent-trusted in exactly the way you feared**
   (your item 30, RIGHT — narrowly). The status enum is 100% server-computed
   (`server/internal/execution/compliance.go:111-120`), but the server's only
   check on the agent's `bool compliant` is `&& status == "success"`
   (`compliance.go:50`) — it stores the contradicting detection exit code in
   the same transaction and never reads it (zero `ExitCode` readers in
   server non-test code). Sharper still: the grace clock is agent-supplied
   with no skew clamp (`execution/result.go:291-299`), and grace expires only
   when a new report arrives — no sweep. Your fix (agent sends evidence,
   server derives) is small: the agent's derivation is already exactly
   `exit_code == 0` (`agent/internal/executor/executor.go:342-350`).
6. **Sibling containers with conflicting desired state both compile** (your
   items 12/14 — the "contradiction" is two true statements about different
   layers). Mode precedence per source is real and pinned
   (`assignment/handlers.go:311-350`); container absorption is real and pinned
   (`dispatch/assigned.go:38-85`). What has no rule: two *unrelated* sources
   carrying the same action — both emit, the device flaps between PRESENT and
   ABSENT forever, untested. And the preview lies: `GetDeviceAssignments`
   dedupes across siblings (`handlers.go:367-381`) while dispatch does not,
   so the UI can show one ABSENT row while the device receives both.
7. **Ordered definition execution is unsupported and the contract promises
   it** (your item 16, RIGHT). `control.proto:906-909` says sets are walked
   in order when "the definition's schedule fires" — no such event exists;
   the compiler emits N independent manifests (`manifest/compiler.go:145-155`)
   with no sequence field, and observed order is a ULID-tiebreak accident.
8. **One empty assigned set aborts the device's entire dispatch** (your item
   18, RIGHT). `ErrEmptyManifest` fails the whole `DispatchAssignedActions`
   (`dispatch/handlers.go:455-466`), the web maps it to "Please check your
   input and try again" with the request ID suppressed
   (`web/src/lib/errors.ts:46,64-70`). The codebase itself already treats
   zero-work dispatch as audited success (`handlers.go:469-478`) — skipping
   with an audit effect is consistent with its own logic. The refusal is
   pinned by no test.

### Your identity proposals (items 36) — measured against pinned attacks

- Multi-SCIM equality: **already today's design** — no authority concept
  exists anywhere; the erase decision is a bare link count
  (`scim/users_write.go:728-734`).
- **Link-governed lifecycle** — your model, stated correctly: any SCIM provider
  that provisions a user takes management by *appending* an identity link;
  deprovisioning removes that provider's link; the last link removed deletes
  the user. (An earlier draft of this report mis-stated this as "delete the
  provenance condition and bind freely" and raised objections that do not
  apply to what you actually proposed. Corrected below.)

  In code it is **one clause**. `deleteUser` today ends with
  `if remaining > 0 || before.ProvisioningSource != SCIM { return nil }`
  (`scim/users_write.go:728-734`); your model drops the second half. Nothing
  else needs inventing — `bindExistingSubject` (`users_write.go:184-200`)
  already appends a link to an existing local account, and the
  `(user_id, provider_id)` uniqueness already makes "one link per provider"
  structural.

  **The takeover guard is untouched by this, and I was wrong to couple them.**
  `mayBindByAddress` (`users_write.go:166-182`) governs *who may adopt*, not
  what unlinking means. Under your model it becomes *more* load-bearing:
  binding now carries the power to delete on last unlink. Note the
  consequence — adopting an already-bound subject (an OIDC-JIT user, i.e. the
  exact "SCIM should take over the OIDC users" case you describe) requires
  that provider to have `trust_email_assertions` set. That flag's name and
  docs undersell what it grants once lifecycle follows links; it deserves
  renaming or an explicit warning in the provider form.

  **What the model fixes structurally:** the zero-link zombie becomes
  impossible; `provisioning_source` stops governing lifecycle and reverts to
  what it honestly is (a record of how the subject first appeared); and no
  precedence concept is needed — which is already true today, so your
  "providers are equal" point costs nothing to adopt.

  **What it leaves open:** a subject whose only link is OIDC and which no SCIM
  directory ever adopts still has no removal path — self-service
  `UnlinkIdentity` refuses the last link (`identity/links.go:76-83`) — so
  `EraseJITUser` survives as the manual erase for that case rather than as a
  lifecycle rule.

  **What would change under it:** the deleteUser doc comment
  (`users_write.go:700-704`), `TestUsers_DeleteOfJITUsersLastSCIMBindingOnlyUnbinds`
  (`scim/users_test.go:727`) and `TestEraseJITUser_RejectsSCIMSubjectWithoutMutation`
  all encode the old rule and would invert. The erasure itself already
  crypto-shreds the subject's DEK (`store/user_erasure.go:45-51`) — that is
  the intended meaning of "deprovisioned", but it is irreversible, so the
  provider form should say so where the admin turns adoption on.

### Security items (items 38, 42, 44, 46, 50)

- **HttpOnly cookies: GAP, worth doing, cheaper than the docs imply.** Bearer
  is browser-only (agents are mTLS, SCIM has its own bearer, bootstrap is a
  distinct scheme), same-origin already holds in the deploy, so this is a
  replacement, not a second path. CSRF story: `SameSite=Strict` + exact-origin
  CORS + `connect.WithRequireConnectProtocolHeader()` (currently unused).
  Friction: client-side refresh scheduling reads `expiresAt` — keep it in the
  body (it is not secret) or move to 401-driven refresh.
- **Crypto-shred: MISREAD — it exists and is live.** Per-subject DEK destroyed
  in the erasure transaction (`store/user_erasure.go:45-51`,
  `schema.sql:53-59`), justified by the off-host audit archive that DELETE
  cannot reach. The docs describe it (`docs/security-model.md:545-551`) but
  never print the word "crypto-shred" — that is the fix.
- **Enrollment socket: MISREAD caused by six stale texts.** The socket is
  0600 + exact-uid SO_PEERCRED, fail-closed
  (`agent/internal/deviceauth/enroll_peercred.go:14-16`,
  `enroll_server.go:71-74`). But `agent/install.sh:473,428,600`,
  `agent/README.md:175,204`, `enroll.go:265,55`, and
  `device_auth.proto:12,17,25` all still say "any user, no sudo" — two of
  those print at install time, and the proto ships the inverted claim to
  third-party client authors. The docs also omit the sharper truth: any
  *root* process can enroll the host (authorization by OS identity, not
  human intent — the code's own comment, `enroll_server.go:17-24`).
- **CA pin: MISREAD.** It is `hex(sha256(CA certificate DER))` — never a
  private key (`sdk/crypto/cert.go:19-26`); the docs pre-empt the question at
  `docs/enrollment.md:179-186`. Cert-DER over SPKI is deliberate (the pin is
  one-shot at enrollment; rotation is handled by `VerifyCAContinuity`). A
  docs-side `openssl` verification recipe would have prevented the question.
- **Server installer verification: GAP, cheap.** The agent installer's whole
  signing scheme (manifest + Ed25519 + stamped key + runtime-assembled
  sentinels) exists and is reusable; the deploy tarball just needs to become
  a signed release asset — which also fixes launch blocker #1. The mutable
  branch-ref fallback (`server/deploy/install.sh:130-132`) should die with it.

### Proto/field items — the one-line verdicts

| Your item | Verdict |
|---|---|
| instant_action enum (56) | Valid, narrower: keep `Action.type`; add a request-level enum. Real defect: the instant set is hardcoded three times with no shared source |
| AssignmentSource/TargetType (58) | MISREAD — load-bearing everywhere (`assignment/state.go`, resolution SQL) |
| ErrorDetail typed codes (60) | Wrong fix, real problem: live 3-way drift — 14 TS codes no Go emits, 9 Go codes with no i18n, one dead path pointer |
| Monthly windows (62) | Weekly-only confirmed; but monthly *execution* exists via cron `0 3 1 * *` — the window model needs the union fixed first |
| cron validator (64) | Valid; stock `cron` accepts descriptors/6-7 fields — needs a custom `cron5` beside `ulid`. Today invalid cron fails OPEN to 8h drift |
| invert run_on_assign / skip_if_unchanged (66) | Your instinct right, both flags broken deeper: `run_on_assign` only gates the cron branch (interval actions run on assign regardless, `store.go:165-167`); `skip_if_unchanged` suppresses the *report*, not execution, and its hash keeps `Output` so varying stdout never dedups. Rename+invert (`skip_on_assign`, `report_unchanged`), never flip polarity under the same name |
| per-manager version (68) | Valid and worse than cosmetic: one version field breaks mixed fleets — perpetual per-run failure, no NOT_APPLICABLE path (`action_package.go:64-102`) |
| ShellParams map (70) | Correct as-is: map = uniqueness; deterministic marshal already sorts keys; real gap is web has no editor for it at all |
| dirpath/filepath (72) | Do NOT: they stat the *control server's* filesystem for device-side paths, and panic on odd errors |
| security_only inversion (74) | Do NOT invert the bool: zero-value would no-op on pacman/flatpak/apt-sans-unattended (`not_applicable.go:36-40`). Make it a UI default or 3-state enum |
| flatpak url validator (76) | app_id is reverse-DNS, not a URL; real gap: no authoring-boundary grammar check (`--system` accepted until device) |
| flatpak remote action (78) | Confirmed gap; SDK already has AddRemote/RemoveRemote with zero callers |
| apt arch max 32 (80) | Debian arch tuples exceed 10 (`kfreebsd-amd64`); the regex allows comma lists whose deb822 rendering is suspect and untested — verify against apt before touching |
| dnf/pacman URL validation (82/84) | Valid, authoring-time only (SDK already enforces https downstream); `https_url`, not `url` |
| create_home default (86) | Do NOT invert — it would regress the pinned `cadestro-tty` bug (`actionparams.go:27-37`); the stale proto comment is the fix |
| primary_group vs gid (88) | Both do different jobs (name-ensure vs numeric); real gap: set both to different groups and primary_group is silently discarded |
| system_group+gid (90) | No error today: `-g` wins, `-r` becomes no-op — same silent-precedence class as above; fix both together or neither |
| ActionResult metadata comment (92) | Comment lies in both directions: server hard-rejects non-empty metadata; no agent path populates it; field has no structural redaction coverage — the comment is what would tempt someone to use it |
| https_url on agent update (94) | Already triple-gated and stronger than `https_url`; one cosmetic edge (`https://#x`) closes with the tag swap |
| on_failure always present (96) | Cannot be `required` (CONTINUE is the zero value); the real hole is out-of-range values failing OPEN to CONTINUE — needs `oneof=0 1`, the idiom the encryption enums already carry |
| default_on_failure duplication (98) | Write-only field: compiler writes it, nothing reads it, its stated purpose (agent logs naming the policy) was never implemented. Four options laid out in the sub-report; occurrence-level is the live one |

### Doc items — one-liners

- "Three containers behind Traefik" (3): the README double-counts — Traefik
  *is* the ingress; QUICKSTART and installation.md have it right; two
  docref-anchored blocks over the same region disagree.
- SQL upgrade files (4): recorded as your NEW post-RC direction, reversing
  the clean-break rule for released versions. Eight-point inventory in the
  sub-report; the sharpest constraint: audit-evidence tables carry
  UPDATE-refusing triggers, so upgrade SQL touching them is refused by the
  database itself.
- "One subtlety" phrasing (6): editorial, your framing adopted.
- "devices or groups" vs four targets (8): the first sentence is the error;
  USER/USER_GROUP are fully implemented targets.
- one-shot vs instant naming (28): they are two different concepts (every
  instant action is one-shot, not vice versa); renaming the heading would
  make the docs wrong — the real fix is giving instant actions their own
  section with a cross-reference.
- RELEASE_TAG=latest (48): mechanically impossible as-is (the tag feeds a
  source-archive URL; no `latest` ref exists) and the resolution step it
  would need reintroduces the recorded prerelease-skipping failure. Becomes
  workable only after signed deploy assets exist.
- IMAGE_TAG=latest contradiction (54): confirmed — `.env.example` contradicts
  itself two lines apart; installation.md praises pinning for the one image
  the project doesn't build.
- Backup → health (52): wiring it into `/ready` would fail the *first
  install* by construction (fresh installs have no backup; every `--wait`
  gate consumes `/ready`). The safe carrier is `/health` — currently an
  unconditional 200 that nothing consumes — or the existing
  `cadestro backup-status` + webhook, which already alert.
- Description beyond "audit log" (40): editorial; `README.md:67-68` is the
  sentence to rebalance; the GitHub About blurb lives in repo settings.

---

## 2. What the same care found beyond your list (meta-sweep, full-coverage)

Coverage: 1,301 proto fields (all), 719 gotags (all), 115 bools (all), 11
prose files (all), consumers traced. Highlights — the full tables are in the
sweep report:

**Severe:**
- `GetSSOLoginURLRequest.redirect_url` has **no allow-list anywhere**
  (`identity/sso.go:85,126`) — the IdP's registered-redirect check is the
  sole defense against auth-code interception.
- `UpdateIdentityProviderRequest` is full-replace over four security bools:
  an update omitting `trust_email_assertions` silently flips the takeover
  guard; omitting `enabled` disables the IdP. The `optional` idiom exists in
  the same file — applied to secrets only.
- Repository actions default to **unsigned-package trust**: omitted
  `gpgcheck` renders `gpgcheck=0` and drops the `gpgkey=` line
  (`sdk/sys/repo/dnf.go:36-43`); docs mention repository GPG nowhere.
- `SshParams` with both auth bools omitted writes `PubkeyAuthentication no`
  + `PasswordAuthentication no` — an access-granting action locks the group
  out, `sshd -t` passes it.
- Privileged `CreateToken{name}` mints a reusable, unlimited, never-expiring
  enrollment token — the low-privilege `:self` path is the guarded one
  (`registrationtoken/handlers.go:139-163`).
- `SecurityAlert.message` (required on the wire) is **discarded on arrival**
  — only the type string is persisted (`agentstream/handler.go:502-516`).
- `AddUserSshKeyRequest.public_key` parses but then stores the raw line —
  `command=`/`environment=` options survive into authorized_keys
  (`identity/users.go:558,568`).
- Structural: **the contract is systematically weaker than the executors** —
  ten field families where a working validator exists one layer down, so
  rejection lands as N failed device actions instead of a 400 (table in the
  sweep report). Unauthenticated endpoints (`RegisterRequest.csr`, SSO
  callback `code`/`state`, refresh tokens) carry no length caps.

**Dead surface** (beyond your anchors): `Heartbeat.cpu_percent`/`disk_percent`
never read; `OSQuery.where` + `LogQuery.source` structurally unreachable (the
control-side requests lack the fields); `doas` support exists only in proto
comments (zero code, and the env accepts a `root` value the enum can't
express); 6 of 13 documented osquery tables unimplemented while 6 implemented
ones are undocumented; the LUKS `cli_command` comment documents the exact
insecure `--token` form a security fix eliminated.

**Bool-default law (verified over all 115):** every documented default on a
non-`optional` proto3 bool drifts; every string/int default holds (they have
sentinels, bools don't). The web client already *tried* to compensate with
seven `?? true` fallbacks — all unreachable, protobuf-es bools are never
nullish.

**Where the floor holds** (calibration): the SDK's 12 anti-injection
validators, the sshd CR/LF + `sshd -t` guards, authorized_keys newline guard,
GECOS guard, osquery credential-table deny-list, debug_redact sink guards,
scope-kind fail-closed mapping — all real, all correct. The weak layer is the
contract, not the executors.

---

## 3. The patterns you told me to extract

1. **Proto3 bool + documented default = lie.** No sentinel, no code
   compensates, and `optional` was applied only where secrets forced it.
   Mechanizable: a contract test that fails any bool whose comment says
   "Default: true".
2. **Comment/behavior drift concentrates where behavior moved after the
   comment was written** — removed features (doas, CLI token form), added
   guards (metadata rejection), renamed fields. Docref catches anchored
   *prose*; proto comments and README strings have no anchor discipline —
   that is where all six "any user, no sudo" texts and both dead-capability
   comments lived.
3. **Validate-at-the-edge eroded into validate-at-the-executor.** Every new
   param field got a length cap and trusted the SDK to be strict later. The
   project's own rule ("validate at the transport boundary and handler")
   inverted silently, one field at a time.
4. **Two sources of truth drift; three drift worse.** Error codes
   (Go/TS/i18n), instant-action sets (×3), permission fixtures, env-var
   registries — every unshared enumeration in the repo has drifted. The
   fixes that lasted are the self-discovering ones.
5. **Docs written from code inherit the code's blind spots.** The
   docref-anchored corpus stayed factually correct *about the lines it
   anchored* and still got the windows union wrong — because the anchored
   function's semantics are decided by its callers. Anchoring must follow
   the data flow, not the function boundary.
6. **Fail-open lives in the un-taken branch**: invalid cron → drift
   cadence; out-of-range on_failure → CONTINUE; unknown search date-field →
   silent zero results; unvalidated enum → switch default. The codebase's own
   best idiom (`oneof=` range tags, discovered-set tests) exists and is
   applied to exactly two enums.

---

## 4. Corrections to the audit (gently, with proof)

Item 30's premise ("devices report their own compliance state") — the enum
never crosses the wire; item 42 (crypto-shred) — implemented, only the term
missing; item 44 (socket) — gated, the *texts* were stale; item 46 (CA pin) —
certificate DER, docs already correct; item 70 (env map) — map is the right
type; item 72/74/86 — each inversion would break named, pinned behavior. The
score otherwise is heavily in your favor: of ~45 items, roughly two-thirds
identified a real defect, doc lie, or unruled ambiguity, and several
(assignment execution, windows, compliance trust, empty-set poisoning) are
significant.

---

## 5. Report provenance

Sub-reports (full evidence, file:line for every claim) are in the session
records; this file is the curated synthesis. Verified against tree `99ab489`
(50 local commits, full sequential gate green). Three claims one verification
stream could not confirm are excluded, not asserted. A second scan wave over
the areas this round did not reach (server domain internals, remaining agent
executors, SDK sys design, web forms/validation drift, deploy/CI posture) is
running; its findings will be appended as `AUDIT-FINDINGS-WAVE2.md`.
