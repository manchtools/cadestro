# Security model

Cadestro's security posture is what the code enforces, not what this page
asserts. Every claim below is anchored to the source that proves it, and where
the code proves something narrower than you might expect, this page says so
rather than rounding up. The [limits section](#limits-stated-plainly) at the end
is not an afterthought — it is the part worth reading twice.

---

## 1. Trust boundaries

The control process opens **exactly two** listeners, and they authenticate
differently because they carry different things.

| | Browser / API | Agents |
|---|---|---|
| Default address | `:8081` | `:8082` |
| Who reaches it | Traefik, from the internet | Traefik, TCP passthrough |
| Authentication | per-request bearer session, SCIM token, or bootstrap token | client certificate + peer class + revocation |
| TLS | terminated at Traefik, re-originated to control | **not** terminated by Traefik — end to end to control |

<!-- docref: begin src=server/cmd/cadestro/httpserver.go#buildAgentServer:ccd04d34 -->
The agent listener requires and verifies a client certificate against the
deployment CA, at TLS 1.3. It is not an authenticated-optional endpoint with a
check bolted on afterwards: a connection without an acceptable certificate never
completes a handshake.
<!-- docref: end -->

<!-- docref: begin src=server/cmd/cadestro/httpserver.go#serveAgent:0543d07f -->
That listener additionally accepts **only PROXY protocol v2**, and only from the
configured proxy sources. Because Traefik passes agent traffic through without
terminating it, the real client address would otherwise be lost; the PROXY
header carries it, and a connection that does not present a v2 header is
rejected before TLS.
<!-- docref: end -->

<!-- docref: begin src=server/internal/architecture/deploy_test.go#TestDeploymentIsTheThreeServiceTarget:38c51965 -->
The shape of the deployment is a test, not a convention. It asserts exactly
three services, forbids mounting the Docker socket, forbids Docker-provider
autodiscovery, requires the agent network to be internal-only, and requires
every HTTP backend to be reached over TLS unless it is explicitly excused. One
backend is excused: the static web container on the internal bridge.
<!-- docref: end -->

<!-- docref: begin src=server/internal/auth/interceptor.go#resolveClientIP:c0b35abd -->
Forwarded client addresses are not taken on faith. `X-Forwarded-For` is honoured
**only when the direct peer is itself a configured trusted proxy**; the chain is
walked from the right, skipping trusted hops, and falls back to the direct peer
on anything malformed. With no trusted proxies configured, proxy headers are
ignored entirely — so a client cannot forge its own origin by sending the header
itself.
<!-- docref: end -->

<!-- docref: begin src=server/internal/auth/interceptor.go#Fingerprint:7d496407 -->
The resolved address never reaches an audit row in the clear. Audit records
carry a SHA-256 fingerprint of the origin, which is enough to correlate events
from one source without storing an address log.
<!-- docref: end -->

### The development bypass

Cadestro ships a development authentication bypass. It is worth describing
precisely, because "there is a bypass" is the kind of sentence that deserves
detail rather than reassurance.

<!-- docref: begin src=server/cmd/cadestro/devauth_stub.go#wrapDevAuth:93667767,server/cmd/cadestro/devauth_stub.go#archiveIsolationRelaxed:8de98d35 -->
**It is not in the production binary.** The bypass lives behind a `devauth`
build tag. The default build compiles a stub whose wrapper returns the handler
unchanged and whose archive-isolation relaxation returns false with no way to be
told otherwise. There is deliberately no configuration variable for either —
an option to skip a verification is the first thing an attacker looks for.
<!-- docref: end -->

<!-- docref: begin src=server/cmd/cadestro/devauth.go#devAuthEnabled:4a7b3325 -->
Even in a build that contains it, it stays off unless an environment variable is
set *and* a token of at least 32 characters is configured. Compiling it in is
not enabling it.
<!-- docref: end -->

<!-- docref: begin src=server/cmd/cadestro/devauth.go#devRequestIsLocal:76043d91 -->
And when enabled, it authorizes the **original client**, not the connection it
sees. A development proxy terminates the browser's connection, so a loopback
peer address proves only that the proxy is local. The check therefore requires a
forwarded-for header to be present (its absence is a refusal), refuses a request
carrying an RFC 7239 `Forwarded` header, and requires every address in the chain
— plus any real-IP header — to be loopback. A remotely reachable proxy cannot
turn its own local backend connection into evidence of a local caller. The
request must also carry the token, compared in constant time, and any origin
must be one of four fixed development origins.
<!-- docref: end -->

---

## 2. Identity

There are no local accounts. Every human identity comes from a directory you
already run.

### SCIM

<!-- docref: begin src=server/internal/scim/handler.go#Handler.Mount:4a7df22e -->
SCIM 2.0 provides the lifecycle: users and groups, each with list, get, create,
replace, patch, and delete, plus the discovery endpoints. Every route is
classified as a mutation, a sensitive read, or discovery — and the discovery
endpoints are authenticated too.
<!-- docref: end -->

<!-- docref: begin src=server/internal/scim/auth.go#Handler.withAuth:33fa16d3,server/internal/scim/auth.go#absentTokenDigest:aebfb116 -->
The bearer token is compared in constant time against a stored digest. The
interesting part is the failure path: an unknown provider, a disabled provider,
SCIM not enabled, and no token configured all compare against a sentinel digest
that no real token can produce. The work performed and the response returned are
identical in every case, so the endpoint cannot be used to enumerate which
provider slugs exist. Only the server-side reason differs. Rate limits apply per
provider and per source address, and every rejection is audited.
<!-- docref: end -->

<!-- docref: begin src=server/internal/scim/users_write.go#Handler.mayBindByAddress:f2d93a25 -->
Account takeover by email assertion is guarded explicitly: an account already
bound to another directory cannot be claimed by a second one unless that
provider is configured to be trusted for email assertions. Directory A cannot
absorb Directory B's users by asserting their addresses.
<!-- docref: end -->

<!-- docref: begin src=server/internal/scim/users_write.go#Handler.deleteUser:cbdec2e9 -->
Deprovisioning removes the identity link. The subject itself is erased only when
its **last** binding is removed and it was SCIM-provisioned in the first place —
so unlinking one directory from a user who is also in another does not delete
the person.
<!-- docref: end -->

### OIDC just-in-time provisioning

<!-- docref: begin src=server/internal/idp/linker.go#Linker.LinkOrCreate:1422b5b6 -->
For deployments without SCIM, a provider can be configured to create users on
first login. Resolution is ordered: an existing binding for that provider and
subject; then auto-link by email, **only** if the provider allows it and — where
the account already has a binding — only if the provider is trusted for email
assertions; then auto-create, only if the provider allows it and the assertion
carries an email. Everything else returns one indistinguishable "no matching
account" error, so a login attempt cannot probe which of the three conditions
failed.
<!-- docref: end -->

<!-- docref: begin src=server/internal/identity/users.go#Handlers.EraseJITUser:6cc8f91a -->
Erasure is correspondingly narrow. The erase RPC refuses any subject that was
not created by OIDC just-in-time provisioning — a SCIM-managed user is erased
through SCIM, and the RPC says so rather than deleting a record the directory
still believes it owns.
<!-- docref: end -->

### What deliberately does not exist

<!-- docref: begin src=server/internal/identity/guards_test.go#TestRegistry_HasNoLocalCredentialPermissions:10ba901e -->
There are no local passwords, no TOTP, and no WebAuthn. That is enforced, not
merely unimplemented: a test fails the build if a permission named for password
updates, TOTP setup or verification, backup codes, or login appears in the
permission registry.
<!-- docref: end -->

<!-- docref: begin src=server/internal/store/schema_test.go#TestSchema_HoldsNoLocalAuthenticationSecrets:21081d34 -->
The database is checked the same way — against the live catalog, not a
hand-maintained list. Any column whose name resembles a password hash, TOTP
secret, backup code, MFA or WebAuthn field fails the test, as does the existence
of a TOTP or WebAuthn table.
<!-- docref: end -->

<!-- docref: begin src=server/internal/identity/guards_test.go#TestPublicProcedures_AreExactlyTheUnauthenticatedSurface:03f0bc7a -->
The unauthenticated surface is pinned as an exact set — not a maximum, an exact
set, so adding an unauthenticated endpoint fails the build until someone
consciously adds it to that list. It is additionally asserted to contain nothing
resembling a login or TOTP procedure, and procedures that have been retired from
the contract are named individually so the entry cannot come back on its own.
<!-- docref: end -->

### Bootstrap

A fresh install has no identity provider, and therefore no way to log in. The
bootstrap token is the one exception, and it is scoped to exactly that problem.

<!-- docref: begin src=server/internal/identity/bootstrap.go#Bootstrapper.Issue:38580545,server/internal/identity/bootstrap.go#DefaultBootstrapTokenTTL:77b258d2 -->
It is minted only by a subcommand run on the host — you must already have
control of the machine. It is 32 bytes of cryptographic randomness, valid for 15
minutes, and only its digest is stored. Issuing one retires any predecessor in
the same transaction, so at most one is ever presentable.
<!-- docref: end -->

Spending it is a single conditional database update covering liveness, expiry
and use count, so two concurrent presentations cannot both succeed, and every
rejection returns one indistinguishable error.

<!-- docref: begin src=server/internal/identity/bootstrap.go#BootstrapPermissions:a87f5966 -->
Its authority is a fixed, minimal list: create and read identity providers,
create and read roles, list permissions, read users, and assign a role to a
user. That is precisely enough to configure the first identity provider and
grant the first admin — and nothing else. It cannot dispatch an action, read a
secret, or touch a device.
<!-- docref: end -->

<!-- docref: begin src=server/internal/auth/context.go#UserContext.CanOwnResources:411da86b -->
It also cannot *be* a user. The bootstrap principal's identifier is deliberately
not a ULID, and the check that decides whether a principal may own resources
requires both a user-kind principal and a well-formed ULID. So every
`:self`-scoped permission is unsatisfiable for it by construction, rather than
by a list of exceptions someone has to maintain.
<!-- docref: end -->

### Authorization

<!-- docref: begin src=server/internal/auth/permissions.go#registryPermissions:a3ba0530 -->
Permissions live in one registry — roughly 165 entries, each declaring its key,
its UI grouping, its description, and the kind of target it acts on. That target
kind is what decides whether a permission can be scoped to a group, and the zero
value is **not scopable**, so a new permission added without thinking lands on
the safe side.
<!-- docref: end -->

<!-- docref: begin src=server/internal/auth/permissions.go#IsPrivilegeGranting:86ba703f -->
Permissions that can widen someone else's privilege are listed once and are
global-only — they can never be handed out scoped to a group. The classifier
treats an **unknown** key as privilege-granting, so a permission that is somehow
not in the registry fails closed rather than being silently treated as harmless.
<!-- docref: end -->

<!-- docref: begin src=server/internal/auth/authorizer.go#Authorize:727c77ee -->
A check resolves in four tiers: the exact permission; the `:self` variant when
no resource is named; the `:self` variant when the named resource *is* the
actor; and an explicitly classified `:assigned` alternative for the handful of
device reads where "a device assigned to me" is a meaningful grant.
<!-- docref: end -->

<!-- docref: begin src=server/internal/auth/scope.go#EnforceDeviceScope:de709cda -->
Coarse authorization at the interceptor is not the end of it. Handlers
re-enforce scope against the actual resource, and a scoped grant of the wrong
kind — a device-group scope on a user permission — always fails closed rather
than being ignored.
<!-- docref: end -->

<!-- docref: begin src=server/internal/identity/users.go#Handlers.resolveUserTarget:9918fbf0,server/internal/device/handlers.go#Handlers.readDevice:9eca5df2 -->
**Out-of-scope access returns NotFound, not Forbidden.** A caller confined to a
subset of the fleet must not be able to learn that a device or user exists by
observing which error it gets. Unauthenticated still returns unauthenticated —
that distinction is not secret — but "exists, and you may not have it" and "does
not exist" are deliberately indistinguishable.
<!-- docref: end -->

### Browser sessions

<!-- docref: begin src=server/internal/auth/jwt.go#SigningAlgorithm:72fb2ebe,server/internal/auth/jwt.go#DefaultAccessTokenExpiry:b58dd1b5 -->
Sessions are Ed25519-signed JWTs. Validation pins the signing method — it does
not accept whatever the token's header claims — requires the issuer, requires an
expiry, and rejects an access token presented where a refresh token belongs. An
access token lives five minutes; a refresh token seven days.
<!-- docref: end -->

<!-- docref: begin src=server/internal/identity/session.go#Handlers.RefreshToken:27ffccfc -->
Permissions ride only on the short-lived access token; refresh re-reads
authority from the database. So revoking a role takes effect within five
minutes rather than at the end of a seven-day session. Refresh also rejects a
token whose user has been deleted, disabled, or had its session version bumped,
and rotation revokes the old token identifier **before** minting the
replacement.
<!-- docref: end -->

<!-- docref: begin src=contract/ts/auth.ts#saveAuth:2e47d5b2 -->
Session tokens are held in browser Web Storage — `localStorage` when the user
asked to stay signed in, `sessionStorage` otherwise. They are **not** in
`HttpOnly` cookies, and this page will not pretend otherwise: a script running
on the origin can read them. What that buys is that there is no ambient cookie
credential, so there is no cross-site request forgery surface to defend.
<!-- docref: end -->

<!-- docref: begin src=server/internal/middleware/security.go#SecurityHeaders:8aca50d7,server/internal/middleware/cors.go#CORS:89535eab -->
The mitigations that actually apply are a content security policy restricting
scripts to the origin and forbidding framing, and a CORS policy that sends
credentials only to an explicitly allow-listed origin. There is one qualifier
worth knowing: an operator can set an allow-all CORS variable, which reflects any
origin — deliberately without credentials — and logs a warning.
<!-- docref: end -->

---

## 3. Device PKI

<!-- docref: begin src=server/deploy/setup.sh#ensure_ca:51675c50 -->
The deployment generates a single self-signed Ed25519 CA — subject
`CN=Cadestro Internal CA, O=Cadestro` — marked critical as a CA and restricted
to certificate and CRL signing. There is no intermediate.
<!-- docref: end -->

<!-- docref: begin src=server/internal/ca/ca.go#New:ceb15152 -->
The CA key must be Ed25519 and its certificate's public key must match it; a
mismatch is a startup failure. The key file must also not be group- or
world-accessible — control refuses to start rather than using a CA key anyone
on the box can read.
<!-- docref: end -->

<!-- docref: begin src=server/internal/ca/ca.go#CA.SetTrustBundle:3b932aea -->
Rotation is by trust bundle rather than by hierarchy. A bundle may carry several
CAs, each of which must be capable of signing certificates, and the bundle must
contain the currently active CA — so you cannot rotate yourself out of trusting
the certificates already deployed to your fleet.
<!-- docref: end -->

### Peer classes

<!-- docref: begin src=server/internal/mtls/peer_class.go#PeerClassFromCert:df329135 -->
Certificates carry a SPIFFE-style URI SAN naming their class:
`spiffe://cadestro/agent` or `spiffe://cadestro/control`. Those two are the only
classes. Classification is strict — a certificate carrying two different class
URIs is a hard error rather than a first-match win, one carrying none is an
error, and an unrecognised class is refused.
<!-- docref: end -->

<!-- docref: begin src=server/internal/ca/ca_test.go#TestIssueCertificateFromCSR_StampsExactlyAgentPeerClass:8a47be06 -->
The class is stamped by the server and is unforgeable from the CSR, which may
not request URI SANs at all (see [enrollment](enrollment.md#4-the-key-and-the-csr)).
A test pins that a device certificate carries exactly one URI SAN, that it is
the agent class, and that agent certificates are client-auth only — so a device
certificate cannot be used to impersonate a server.
<!-- docref: end -->

**Where class checking applies is one-directional, and this page states it
plainly.** Control checks the class of every agent that connects. The agent
verifies control by ordinary X.509 against the CA it enrolled with — it does not
read control's peer class. The protection against a rogue "control" is CA
pinning, not class checking.

---

## 4. Audit guarantees

This is the part the product is named for. A *cadastre* is an authoritative land
register: the record that settles who holds what, kept so that it can be relied
on later. The audit log is Cadestro's equivalent, and it is built to be relied
on rather than merely written.

### One operation, its effects, one transaction

<!-- docref: begin src=server/internal/store/audit.go#AuditOperation:42ce04d3,server/internal/store/audit.go#AuditEffect:4a8afbb5 -->
Every audited event is one **operation** row plus one or more **effect** rows.
The operation records who acted, from where, what they invoked, whether
authorization allowed it, and the outcome. Each effect records one resource that
changed, the action taken, the outcome, and which fields changed.

There is deliberately **no free-form value field** on an effect. References must
be ULIDs, changed-field names must be lowercase identifiers, and evidence is a
digest with a named kind. That is not stylistic: it is what makes it
structurally impossible for a credential to end up in the audit log.
<!-- docref: end -->

<!-- docref: begin src=server/internal/store/audit.go#Store.WithAudit:e6d3a5d5,server/internal/store/store.go#Store.withTx:f0b64994 -->
The mutation and its audit rows are written in **one** database transaction. The
audited-write primitive opens a transaction, runs the mutation, then locks the
chain head, appends the operation and every effect at consecutive positions, and
advances the head — all before a single commit.
<!-- docref: end -->

<!-- docref: begin src=server/internal/store/audit_test.go#TestWithAudit_AuditWriteFailureRollsBackTheMutation:c4d3f8a3 -->
The consequence is the guarantee that matters: **if the audit write fails, the
mutation is rolled back with it.** A test proves this against a real database
with a real failure — an effect the schema refuses — and then asserts the
device, the operation row, and the effect rows are all absent. There is no
configuration in which a change happens un-audited, because there is no code
path in which it can.
<!-- docref: end -->

<!-- docref: begin src=server/internal/store/api_shape_test.go#TestStoreAPI_OnlyAuditedPrimitivesCanMutate:5e3ae40c -->
And that primitive is the only door. A test reflects over every exported method
on the store and requires each to be classified as either mutation-capable —
a short, enumerated list — or non-mutating. An **unclassified** method fails the
build, and a separate list names the shapes that would hand out a generic
escape hatch (a raw query, a transaction handle, the connection pool) so they
cannot be added at all.
<!-- docref: end -->

<!-- docref: begin src=server/internal/identity/credentials_test.go#TestProcedureClassification_MatchesTheMountedSurface:3136a55a,server/internal/controlrpc/mount_test.go#TestMountIsExactControlServiceDescriptorSet:ca4c8461 -->
Coverage cannot drift either. Every mounted procedure must be classified as a
mutation, a read, or a sensitive read, and every classified procedure must be
mounted — so a new RPC fails the build until it is classified. The mounted set
is separately pinned to be exactly the contract's declared set: nothing missing,
nothing extra, no duplicates.
<!-- docref: end -->

### Reads that are audited

<!-- docref: begin src=server/internal/device/mount.go#SensitiveReadProcedures:cfc7b0a6 -->
Some reads are evidence too. Device inventory, query and log results, compliance
status, execution history, the credential lists, the credential reveals, and
active terminal sessions are all recorded as sensitive-read operations.
<!-- docref: end -->

<!-- docref: begin src=server/internal/device/secrets.go#Handlers.recordSecretReveal:09f0a4fb -->
Revealing a stored credential is the strongest case. It records three effects —
against the credential, the device, and the action — **before any plaintext is
returned**. If the evidence cannot be written, the secret is not revealed.
<!-- docref: end -->

### Append-only

<!-- docref: begin src=server/internal/store/schema_test.go#TestSchema_EveryAuditEvidenceTableHasAppendOnlyGuards:d92964da -->
Append-only is enforced by database triggers, not by application discipline —
an `UPDATE` on any audit evidence table is refused outright, and a `DELETE` is
refused unless a transaction-scoped retention guard covers exactly that row's
position. A structural test queries the live catalog and requires every one of
the four evidence tables to carry both guards, so a table added later cannot
quietly ship without them.
<!-- docref: end -->

### The hash chain

<!-- docref: begin src=server/internal/store/audit.go#chainHash:c251ec0b -->
Every operation and effect row is one position in a single SHA-256 chain, over a
length-prefixed canonical encoding with a distinct domain tag per row type. The
length prefixing is what stops two different records from encoding to the same
bytes.
<!-- docref: end -->

<!-- docref: begin src=server/internal/store/audit_chain.go#Store.VerifyAuditChain:ce744de4 -->
Verification detects a changed field, a reordered or inserted row, an unexplained
gap, an edited head, **and a removed tail** — the last being the case a naive
chain check misses, because truncating the end of a chain leaves the remainder
internally consistent.
<!-- docref: end -->

**The chain is hashed, not signed.** There is no signature over audit rows. A
local hash chain proves internal consistency; on its own it cannot prove that
the whole chain was not rewritten. That is what anchors are for.

<!-- docref: begin src=server/internal/store/audit_chain.go#Store.RecordPublishedAuditAnchor:a9dee619 -->
An anchor records a position and its hash — but only **after** the anchor object
has been written off-host, and only if the local chain reproduces the value
being recorded. An anchor with no external reference is refused, so the table
cannot fill with anchors that were never actually published anywhere.
<!-- docref: end -->

<!-- docref: begin src=server/internal/maintenance/service.go#Service.VerifyAudit:f887e06a -->
Hourly verification then fails closed against those anchors: once an anchor
exists, a missing archive object is an error, and an archived anchor that
disagrees with the recorded row — behind it, or a different hash at the same
position — is treated as a rollback or rewrite. An anchor running *ahead* of the
row is accepted, because publishing the object before recording the row means a
crash between the two legitimately leaves a newer object.
<!-- docref: end -->

### Retention and archive

<!-- docref: begin src=server/internal/maintenance/service.go#Service.RetainAudit:8584f810 -->
Retention deletes evidence, so it earns the right first, in a strict order:
anchor the current head; **re-hash every prefix already archived** against its
recorded digest; find the retention boundary; stream the prefix into the
archive; verify what was written against the digest computed while streaming;
and only then prune. Any failure at any step leaves every live row in place.
<!-- docref: end -->

<!-- docref: begin src=server/internal/store/audit_chain.go#Store.PruneAuditPrefix:963fd119 -->
The prune itself refuses to run without an archive digest, an archive reference,
and an archive timestamp, and does the guard-arming, closed-prefix re-check,
deletion, checkpoint write and disarm in one transaction — so a crash cannot
leave the deletion permission behind.
<!-- docref: end -->

<!-- docref: begin src=server/internal/archive/fs.go#Verify:e4234a66 -->
Archive verification compares against a digest the caller holds from **outside**
the archive — the one in the append-only checkpoint table in the database — and
deliberately ignores the checksum sidecar sitting next to the artifact. Reading
the expected value from the same directory as the artifact would make the
comparison self-referential: anyone able to rewrite one can rewrite both.
Tamper-evidence is only evidence when the two sides sit in different trust
domains.
<!-- docref: end -->

<!-- docref: begin src=server/cmd/cadestro/config.go#validateArchiveIsolation:69e8ec75 -->
Which is why the archive **must be on a different filesystem from the database**,
compared by kernel device ID, and control refuses to start when they match. Note
the honest scope of that check, which its own comment states: it is a floor, not
a guarantee of remoteness — a second local disk passes while sharing a machine.
Getting the archive genuinely off-host is an operator responsibility.
<!-- docref: end -->

---

## 5. Secret handling

### Secrets in transit are sealed to their recipient

<!-- docref: begin src=sdk/crypto/seal.go#SealToPublicKey:6b2352e6 -->
Secret-bearing protocol fields are sealed with X25519: a fresh ephemeral key
pair per operation, a shared secret with the recipient's public key, HKDF-SHA256
to derive an AES-GCM key, and both a non-empty associated-data value and a
non-empty derivation label required by construction rather than by convention.
<!-- docref: end -->

<!-- docref: begin src=sdk/crypto/field_context.go#FieldSealContext:dc8c1166 -->
The associated data is what makes a sealed value non-portable. It binds, as
length-prefixed segments: the sealing-scheme version, the direction
(agent-to-control or the reverse), the fully-qualified message name, the field
name, and then the context — the device, and the specific action, delivery, or
terminal session. Length prefixing prevents two different contexts from encoding
identically, and an empty segment is an error rather than a silently-skipped
field.

So a captured sealed value cannot be moved to another field, another device,
another action, or the opposite direction. It opens only in the exact context it
was produced for. One field goes further: a rotated password additionally binds
its **username**, because a password is only meaningful as a pair, and without
that binding control could not verify that the password it stores under a name
is the one generated for it.
<!-- docref: end -->

<!-- docref: begin src=server/internal/agentsecrets/service.go#sealedFieldVersion:3c95c986 -->
The envelope carries its own scheme version, checked at every opening point. A
recipient refuses a version it does not implement rather than guessing.
<!-- docref: end -->

<!-- docref: begin src=contract/contract_rpc_surface_test.go#TestContract_SecretsAreSealedAndFramesAreUnsigned:4571db0d -->
Which fields must be sealed is not a matter of remembering. A test sweeps the
whole protobuf registry: every field marked as a secret must be a sealed
envelope, unless it appears in a short, justified allowlist of write-only
inputs. The same test bans the signature fields that a previous architecture
used, and it carries matches-zero guards so it cannot pass by scanning nothing.
<!-- docref: end -->

Application frames are **not** separately signed. The mTLS channel authenticates
and protects the transport; adding a second signature layer over ordinary frames
was deliberately removed rather than kept as defence in depth.

### Secrets at rest

<!-- docref: begin src=server/internal/crypto/crypto.go#Encryptor.EncryptWithContext:a5242e11 -->
Stored secrets are AES-256-GCM under the deployment key, with **mandatory**
associated data — there is deliberately no API that accepts an empty context, in
either direction. Decryption also refuses a value that is not tagged with the
current scheme, and refuses plaintext outright, so a stripped ciphertext cannot
be read as data.
<!-- docref: end -->

<!-- docref: begin src=server/internal/crypto/crypto.go#SecretAADForRow:5686972b -->
The associated data binds the device, the action, the secret type, **and the
individual row's immutable identifier** — so one rotation's ciphertext cannot be
relocated onto its sibling row to make an old credential appear current.
<!-- docref: end -->

<!-- docref: begin src=server/internal/crypto/pii.go#GenerateWrappedDEK:dca97952 -->
Personal detail in audit records is handled separately again: each subject gets
its own wrapped data-encryption key, and class-three detail on an audit row is
sealed under that subject's key with the field name as context. Erasing a
subject's key renders their sealed detail unreadable without deleting the
append-only rows around it.
<!-- docref: end -->

<!-- docref: begin src=server/cmd/cadestro/config.go#readSecretFile:60ffa83b -->
Key material is loaded from files that must be regular, small, and **not group-
or world-accessible** — control refuses to start otherwise. Naming both the
inline variable and the file variable for the same secret is an error rather
than a precedence rule to memorise.
<!-- docref: end -->

### Secrets and logs

<!-- docref: begin src=sdk/sys/exec/secret.go#Secret:0e14e52a -->
On the device, a secret is a type whose string and Go-syntax representations
both render as redacted. Formatting it, logging it, or including it in a
structured log record cannot emit the value; only an explicit reveal call can.
<!-- docref: end -->

<!-- docref: begin src=sdk/archtest/reveal_sink_test.go#TestRevealOnlyFromKnownSinks:7a1b97ae -->
And those reveal calls are enumerated. A build-time test parses every capability
source file, walks the syntax tree for reveal calls, and fails on any that is
not one of six named sinks — a password-change stdin, a LUKS keyfile, a TPM
enrolment stdin, and three network-credential paths. It has anti-vacuity guards
at both ends: it fails if it walked no files, fails if it found no reveal call
at all, and fails if a listed sink stops calling reveal. The allowlist can shrink
but cannot rot into a silent gap.
<!-- docref: end -->

**The scope of that guard is the system capability layer**, which is where
plaintext credentials are actually handled on a device. It does not scan the
control server. On the server side the property rests on the audit schema's
shape — which, as described above, has no field a secret could occupy — plus
review, not on a detector.

---

## Limits, stated plainly

Things a reader might reasonably assume that the code does **not** establish:

- **The audit log is hashed and anchored, not signed.** There is no signature
  over audit rows or archived prefixes. Tamper-evidence against a wholesale
  rewrite depends on off-host anchor objects and the digest in the append-only
  checkpoint table.
- **"Separate filesystem" is enforced; "off-host" is not.** A second local disk
  satisfies the startup check.
- **Peer-class checking is one-directional.** Control validates agents' classes;
  agents validate control by CA pinning alone.
- **Session tokens are readable by scripts on the origin.** They are in Web
  Storage, not `HttpOnly` cookies.
- **The secret-sink guard covers the device capability layer, not the server.**
- **Reading the audit log is not itself always audited.** Exporting it is, and
  searching it in the audit scope is; plain listing is not.
- **The absence of a create-user RPC is a property of the current contract, not
  a structural guarantee.** The absence of local *passwords* is structurally
  guaranteed, by both a permission test and a schema test; "no manual user
  creation" is not pinned the same way.
- **Control does not require TLS on its own public listener.** The shipped
  deployment configures it and the deployment test enforces that, but a
  hand-rolled deployment could start it in plaintext behind its own proxy.
- **Enrollment is token-authenticated, not mTLS.** A device has no certificate
  before it enrolls; the registration token and the CA pin are what secure that
  first exchange. See [enrollment](enrollment.md).

---

## Where to go next

- [Enrollment](enrollment.md) — the device trust bootstrap in detail.
- [Installation](installation.md) — generating the CA, keys, and archive mount.
- [Backup and restore](backup-restore.md) — what the archive isolation is
  protecting, and how to keep a verified copy.
- [SECURITY.md](../SECURITY.md) — reporting a vulnerability.
