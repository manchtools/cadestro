# Enrolling a device

Enrollment is how a machine gets a certificate and becomes a device Cadestro can
command. It happens once per machine, it is a privileged local operation, and it
is the only moment where trust is established — everything afterwards rests on
the certificate it produces.

The short version:

```bash
curl -fsSL https://github.com/manchtools/cadestro/releases/latest/download/install.sh \
  | sudo bash -s -- \
      --server https://control.example.com \
      --token  <REGISTRATION_TOKEN> \
      --pin    <CA_SHA256>
```

The web UI hands you that exact line, filled in, when you create a token. The
rest of this page is what each part of it does and why.

---

## 1. Issue a registration token

<!-- docref: begin src=server/internal/registrationtoken/handlers.go#Handlers.CreateToken:5889b437,contract/proto/cadestro/v1/control.proto#CreateTokenRequest:294e61c8 -->
Tokens are minted by `ControlService.CreateToken`. A token has a name, a
required future expiry, and an optional global maximum use count where **zero
means unlimited**. Each successful new device enrollment is one immutable use
record on the device's token relation; retries by the same Ed25519 identity do
not consume another use.
<!-- docref: end -->

<!-- docref: begin src=server/internal/auth/permissions.go#DefaultUserPermissions:1ecf98d4 -->
Only a holder of the full `CreateToken` permission may mint enrollment tokens.
There is no token owner and enrollment never assigns a human owner; operators
use the existing device-user and device-group assignment controls afterwards.
<!-- docref: end -->

The token's plaintext exists exactly once. Control stores only its SHA-256
digest, and the read RPCs never return the value again — so a token that was not
copied at creation must be replaced, not recovered.

<!-- docref: begin src=contract/proto/cadestro/v1/control.proto#CreateTokenResponse:4772e9f2 -->
The creation response carries a second, equally important value: the **CA
fingerprint pin**. It travels beside the token because the two are useless apart
— the token authorizes the agent *to* control, and the pin is what tells the
agent it is talking to the right control in the first place.
<!-- docref: end -->

In the web UI this is **Tokens → New**, which renders the token, the pin, and a
ready-to-paste install command.

---

## 2. Run the installer

<!-- docref: begin src=agent/install.sh#parse_args:e5a44734 -->
The installer takes `-s/--server`, `-t/--token`, and `-p/--pin` for enrollment,
plus `-d/--data-dir` (default `/var/lib/cadestro`), `-b/--binary` (default
`/usr/local/bin/cadestrod`), `-v/--version` (default `latest`), `--pre` for
prereleases, `--skip-download`, `--enable-uri-handler`, and `--uninstall`.
<!-- docref: end -->

<!-- docref: begin src=agent/install.sh#enroll_agent:69bee6dc -->
Enrollment is attempted only when a server URL and token are both present, and
the three enrollment arguments are **all-or-nothing**: supplying none of them
installs the agent unenrolled and says so, but supplying some and not others is
a hard failure rather than a silent skip. In particular `--server` and `--token`
without `--pin` does not enroll — it errors.

The token is never passed on a command line to the agent. The installer writes
it to a `mktemp` file at mode `0600`, removed by a trap, and calls
`cadestrod enroll -token-file=…`.
<!-- docref: end -->

Enrolling later, by hand, is the same operation:

```bash
sudo cadestrod enroll -server=https://control.example.com \
                      -token-file=/path/to/token -pin=<CA_SHA256>
```

<!-- docref: begin src=agent/cmd/cadestrod/cmd_enroll.go#resolveEnrollToken:86821c40 -->
The token is resolved in a fixed order: `-token-file` first, then the
`CADESTRO_REGISTRATION_TOKEN` environment variable, then `-token` on the command
line. The last still works but prints a warning, because an argv token is
readable by any local process through `/proc/<pid>/cmdline`.
<!-- docref: end -->

<!-- docref: begin src=agent/cmd/cadestrod/cmd_enroll.go#runEnroll:5fae8105 -->
Server, token, and pin are all mandatory: a missing one is a usage error and a
non-zero exit, never a degraded enrollment.
<!-- docref: end -->

---

## 3. What actually happens: the local socket

Enrollment does not go straight out to the network. The `enroll` command is a
*client* that talks to the already-running agent over a local unix socket.

<!-- docref: begin src=agent/internal/deviceauth/enroll_server.go#EnrollSocketPath:a4fcf356,agent/internal/deviceauth/enroll_server.go#EnrollServer.Start:35b41ac8 -->
The unenrolled agent listens on `/run/cadestro/enroll.sock`, chmod'ed to `0600`
immediately after binding — and if that chmod fails, the listener is closed and
startup aborts rather than serving on a permissive socket.
<!-- docref: end -->

<!-- docref: begin src=agent/internal/deviceauth/enroll_peercred.go#peerAuthorized:cc646d6c -->
File permissions are not the only gate. The agent authenticates the connecting
process by its OS identity via `SO_PEERCRED`, and the rule is exact equality
with the agent's own uid — root, under the shipped unit. An unprivileged local
caller is refused and the connection dropped before any request is served.
<!-- docref: end -->

This is deliberate and worth stating plainly: **enrollment hands a control plane
the power to command this host.** The token authorizes the agent *to* control and
the pin defends an honest caller against a network attacker, but neither of them
authorizes an arbitrary local process to choose this machine's trust anchors.
That is what the peer check is for.

<!-- docref: begin src=agent/internal/deviceauth/enroll_peercred_other.go#peerUIDOf:bf014398 -->
On any platform where those peer credentials cannot be read, the lookup returns
an error unconditionally and every connection is refused. The feature is not
"best effort" off Linux; it is closed.
<!-- docref: end -->

<!-- docref: begin src=agent/internal/deviceauth/enroll.go#EnrollHandler.Enroll:3365e614 -->
The handler applies a global rate limit of five enrollment attempts per rolling
minute, and serializes the rest of the work under a mutex so that concurrent
callers cannot each pass the "already enrolled?" check and register duplicate
devices.
<!-- docref: end -->

<!-- docref: begin src=agent/internal/deviceauth/enroll_server.go#EnrollServer.Shutdown:a722d8ff -->
Once enrollment succeeds the socket is shut down and **the file is removed**. An
agent that already holds credentials never opens it at all, so the enrollment
surface exists only during the window it is needed.
<!-- docref: end -->

---

## 4. The key and the CSR

<!-- docref: begin src=sdk/crypto/csr.go#GenerateCSR:c49e8cf1 -->
The agent generates an **Ed25519** key pair locally and builds a certificate
signing request from it. The private key never leaves the machine — it is not
sent during enrollment and not sent during renewal.
<!-- docref: end -->

The CSR carries a subject common name and **nothing else** — no SANs of any
kind. That is not an omission; the server enforces it:

<!-- docref: begin src=server/internal/ca/ca.go#CA.issueFromCSR:3fbb020f -->
Issuance verifies the CSR's self-signature, requires an Ed25519 public key, and
**rejects any CSR that requests subject alternative names**. The CSR's common
name is then discarded. Everything identifying in the issued certificate is
chosen by the server: the subject common name and serial are the device's ULID,
the organization is `cadestro`, key usage is digital signature with client-auth
extended usage, and validity runs from one minute in the past (for clock skew)
to the configured lifetime.
<!-- docref: end -->

<!-- docref: begin src=server/internal/mtls/peer_class.go#PeerClassURI:8cfc8da1 -->
The one SAN that does appear is a SPIFFE-style URI the server puts there itself:
`spiffe://cadestro/agent` for a device, `spiffe://cadestro/control` for the
control plane. This is the certificate's *peer class*, and because a CSR may not
request URIs, a device cannot ask to be issued a control-plane identity.
<!-- docref: end -->

<!-- docref: begin src=server/cmd/cadestro/config.go#defaultEnvironment:950ca863 -->
Device certificate lifetime defaults to 8760 hours — one year — and is
configurable with `CADESTRO_CERTIFICATE_VALIDITY`.
<!-- docref: end -->

---

## 5. CA pinning

<!-- docref: begin src=sdk/crypto/cert.go#CAFingerprintFromPEM:5a8bdd28 -->
The pin is the lowercase-hex SHA-256 of the CA certificate's **DER bytes** — the
certificate, not the public key. Control computes the same value from the same
bytes, which is why the string the UI shows you and the string the agent
computes are directly comparable.
<!-- docref: end -->

<!-- docref: begin src=agent/internal/deviceauth/enroll.go#normalizePin:37b7cd96 -->
The supplied pin is normalised first: trimmed, colons stripped, required to be
exactly 64 hex characters, and lowercased. A malformed pin fails before any CSR
is generated and before any network call is made.
<!-- docref: end -->

<!-- docref: begin src=contract/proto/cadestro/v1/device_auth.proto#EnrollRequest:eb23f690 -->
The pin is a required field on the wire, constrained to 64 hexadecimal
characters. **There is no trust-on-first-use path.** An enrollment without a pin
is not a less secure enrollment; it is not an enrollment.
<!-- docref: end -->

After registration returns, the agent fingerprints the CA the server sent and
compares it to the pin. On mismatch it logs both values and refuses — no
credentials are saved, no callback fires, nothing is persisted. A network
attacker who can present a certificate for the control hostname still cannot
enroll a device against their own CA.

---

## 6. The outbound stream

Once enrolled, the agent connects out to control and keeps that connection open.
Devices never listen for inbound connections.

<!-- docref: begin src=agent/cmd/cadestrod/stream_url.go#requireHTTPSAgentAddr:34b7bd46 -->
Before every dial, the agent re-validates its stored control address: the scheme
must be `https`, the host non-empty, the URL non-opaque. A failure exits the
process rather than downgrading.
<!-- docref: end -->

<!-- docref: begin src=contract/client.go#WithMTLSFromPEM:7e7dc2c3 -->
The stream's TLS configuration presents the device certificate and trusts
**only** the enrolled CA — the system trust store is deliberately not consulted
for this connection — with TLS 1.3 as the minimum version. A public CA that has
been tricked into issuing for the control hostname is not a path in.
<!-- docref: end -->

<!-- docref: begin src=server/cmd/cadestro/httpserver.go#buildAgentServer:ccd04d34 -->
The other end is a dedicated listener, separate from the browser/API one, which
requires and verifies a client certificate against the deployment CA at TLS 1.3.
<!-- docref: end -->

<!-- docref: begin src=server/cmd/cadestro/httpserver.go#serveAgent:0543d07f -->
That listener sits behind a PROXY-protocol v2 reader restricted to the
configured proxy sources, so the real client address survives the reverse proxy
in front of it instead of every agent appearing to come from the proxy.
<!-- docref: end -->

<!-- docref: begin src=server/internal/agentstream/identity.go#MTLSMiddleware:f1b23680 -->
Every request on that listener is checked three ways before it reaches any
handler: the device ID is taken from the client certificate's subject (never
from anything the client says), the certificate's peer class must be `agent`, and
its fingerprint is looked up against the revocation table. A missing revocation
checker or a failed lookup is a **denial**, not a pass.
<!-- docref: end -->

<!-- docref: begin src=server/internal/agentstream/handler.go#Handler.recordHello:58681c47 -->
The application layer then re-checks the same thing rather than trusting the
transport once: the first frame must be a hello, its device id must equal the
mTLS identity, and the device must still exist and not be deleted. A mismatch is
permission-denied.
<!-- docref: end -->

<!-- docref: begin src=agent/cmd/cadestrod/runtime.go#runAgent:8847cb7a,agent/cmd/cadestrod/main.go#maxBackoff:2d5b57db -->
Reconnection uses jittered exponential backoff — a randomised 5–10 second first
wait, doubling, capped at five minutes — and resets once a session has lasted
longer than the current backoff, so a stable agent recovers a short retry
interval instead of staying pessimistic forever. The jitter matters at fleet
scale: it is what stops ten thousand agents from reconnecting in lockstep after
a control restart. Every reconnect reloads credentials from disk first, so a
freshly renewed certificate is picked up without a restart.
<!-- docref: end -->

---

## 7. Renewal

<!-- docref: begin src=agent/cmd/cadestrod/cert_rotation.go#renewAt:211ccaeb -->
The agent renews at **80% of its certificate's lifetime** — about 292 days into
a one-year certificate — computed from the certificate's own validity window
rather than a fixed interval, with a one-minute floor if that moment has already
passed.
<!-- docref: end -->

<!-- docref: begin src=agent/cmd/cadestrod/cert_rotation.go#applyRenewal:49ccae95 -->
Renewal reuses the existing private key: a new CSR is generated from it, and the
**current certificate** is presented as the identity. So renewal proves both
possession of the key and possession of the certificate.
<!-- docref: end -->

<!-- docref: begin src=server/internal/enrollment/handlers.go#Handlers.RenewCertificate:d71052c1 -->
Control verifies the presented certificate against its trust pool for client
auth, requires the `agent` peer class, and then checks that the CSR's public key
matches the certificate's — certificates are public material, so possession of
one proves nothing without that check. The new certificate is issued and swapped
in one audited transaction, conditioned on the old fingerprint still being the
current one, and the old fingerprint is revoked in the same transaction with
reason "superseded by renewal". After commit, control closes the device's stream
so it reconnects with the new certificate.
<!-- docref: end -->

<!-- docref: begin src=sdk/crypto/cert.go#VerifyCAContinuity:30b656cc -->
The response also carries the active CA, and the agent will not blindly adopt
it. It accepts a byte-identical CA, or one that is **cross-signed by the CA it
enrolled against**; anything else is refused and the stored credentials are left
untouched. This makes CA rotation possible (issue a cross-signed successor, run
both) while making CA *substitution* impossible — a hard swap to an unrelated
root requires re-enrolling the device.
<!-- docref: end -->

<!-- docref: begin src=agent/cmd/cadestrod/cert_rotation.go#startCertRotation:b90fc359 -->
A failed renewal retries hourly. After three consecutive failures the log
message escalates and names how many hours the rotation has been stalled, so a
control server that has been unreachable for a week is visible on the device
rather than silently counting down to expiry.
<!-- docref: end -->

---

## 8. Revocation

<!-- docref: begin src=server/internal/mtls/peer_class.go#RevocationChecker:7ab2c659,server/internal/store/revocation.go#RevocationChecker.IsRevoked:82de0357 -->
There is **no CRL and no OCSP**. Revocation is a direct indexed database lookup
on the certificate fingerprint, performed on every handshake. A cached snapshot
was rejected deliberately: it creates a window in which a revoked certificate is
still admitted. A nil checker, or a lookup that errors, denies — there is no
permissive fallback and no opt-out.
<!-- docref: end -->

<!-- docref: begin src=server/internal/device/mutations.go#Handlers.DeleteDevice:d9f77d74,server/internal/store/revocation.go#RevokeInTx:836f92ed -->
Deleting a device *is* the revocation action — there is no separate revoke RPC.
The soft delete, the revocation row, and the audit effect all commit in one
transaction, so a device cannot end up deleted-but-not-revoked or the reverse.
<!-- docref: end -->

<!-- docref: begin src=server/internal/connection/manager.go#Manager.Unregister:138fa5ca -->
An agent that is connected *right now* does not keep its session. Revocation
cancels the live stream's context immediately, so the connection terminates at
once instead of surviving until the agent happens to disconnect. A reconnect
attempt then fails at the transport check, and even if it did not, the
application layer would refuse it because the device no longer exists.
<!-- docref: end -->

---

## What enrollment writes into the audit log

<!-- docref: begin src=server/internal/enrollment/handlers.go#Handlers.Register:43193cf2 -->
A successful new registration records the device creation as an effect of one
audited operation whose actor is the registration token itself, identified by
its digest rather than its value. A same-identity retry is audited as an
observed request without an applied mutation.
<!-- docref: end -->

<!-- docref: begin src=server/internal/enrollment/handlers.go#Handlers.recordRejected:42b7a96e -->
A rejected registration is audited too, as a rejected-authentication operation
by an anonymous actor with result code `INVALID_REGISTRATION_TOKEN`. Failed
attempts to join the fleet are evidence, so they are recorded like any other.
<!-- docref: end -->

Agent-side refusals — an unprivileged local caller, a rate-limited attempt, a CA
pin mismatch — are local log records on the device. They never reached control,
so control has nothing to audit.

---

## Known caveats

Two things a code reading proves that an operator should know:

<!-- docref: begin src=agent/internal/credentials/credentials.go#Store.Save:2a0990ab -->
- **A credential save failure after a successful registration is not rolled
  back.** If the agent registers and then cannot persist the result, control has
  a device row and a spent token use, while the device has no usable
  certificate. Delete the device and re-enroll; there is no automatic recovery
  path in the code.
<!-- docref: end -->

- **There is no way to revoke a device without deleting it.** If you want a
  device's certificate to stop working, delete the device.

---

## Where to go next

- [Policy model](policy-model.md) — what flows over the stream once it is up.
- [Security model](security-model.md) — the PKI, peer classes, and trust
  boundaries this page depends on.
- [Installation](installation.md) — standing up the control plane the device
  enrolls against.
