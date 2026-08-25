# Capability reference

Every action type the agent can execute, what it does, and which real tool it
uses underneath. This page is the inventory; [the policy model](policy-model.md)
is how these get scheduled, delivered, and reported.

## How every action behaves

<!-- docref: begin src=agent/internal/executor/executor.go#Executor.ExecuteAction:35a5f88b -->
One action switch maps an action type to its implementation. An unrecognised
type is refused rather than ignored, so a contract the agent is too old to
understand fails loudly instead of silently doing nothing.
<!-- docref: end -->

<!-- docref: begin src=agent/internal/executor/executor.go#defaultTimeoutForAction:fa29b469 -->
**Timeouts.** An explicit `timeout_seconds` above zero always wins. Otherwise
shell and script actions default to one hour and package and update actions to
30 minutes; **every other action type runs with no timeout**. The contract caps
an explicit timeout at 3600 seconds.
<!-- docref: end -->

**Result classification.** A deadline produces `TIMEOUT`, a cancellation
produces `FAILED`, a structural inapplicability produces `NOT_APPLICABLE`, and
anything else non-nil produces `FAILED`. For shell and script actions a non-zero
exit code downgrades a would-be success to `FAILED`.

**Read-only roots.** Nearly every mutating branch first requires a writable
filesystem, remounting real device mounts read-write if needed — an immutable or
accidentally read-only root is repaired rather than silently producing a
succeeded-but-changed-nothing result.

**Desired state.** Which actions honour `PRESENT`/`ABSENT` is noted per section
below; it is not universal, and the exceptions are deliberate.

---

## Package management

### Package

<!-- docref: begin src=agent/internal/executor/action_package.go#Executor.executePackage:3a356e64,contract/proto/cadestro/v1/actions.proto#PackageParams:8ff14120 -->
Installs or removes a package through the host's detected package manager,
probing the installed state first and failing closed if that probe errors.

`name` is the generic package name. Because distributions disagree, there are
per-backend overrides — `apt_name`, `dnf_name`, `pacman_name`, `zypper_name` —
and the resolver picks the one for *this* host's backend, falling back to
`name`. If neither is set for the detected backend the action reports
`NOT_APPLICABLE` rather than guessing.

`version` pins an exact version. `allow_downgrade` is independent of it and must
be set explicitly. `pin` holds the package against upgrades, and a failure to
apply the hold is a real action failure, not a warning.

**Honours `PRESENT` and `ABSENT`.**
<!-- docref: end -->

### Update

<!-- docref: begin src=agent/internal/executor/action_update.go#Executor.executeUpdate:7edd0cd3,contract/proto/cadestro/v1/actions.proto#UpdateParams:cc7d27eb -->
A system-wide upgrade: refresh the index, upgrade everything, optionally
autoremove, then detect whether a reboot is now required.

`security_only` restricts to security updates. `autoremove` removes orphaned
dependencies, with the change detected by comparing installed counts before and
after. `reboot_if_required` schedules a reboot only when *this* run newly created
the requirement — not when one was already outstanding.

Does not read desired state. Reports `NOT_APPLICABLE` when `security_only` is
asked of a backend that cannot express it (see the platform table below).
<!-- docref: end -->

### Repository

<!-- docref: begin src=agent/internal/executor/action_repository.go#Executor.executeRepository:ad870dbc,contract/proto/cadestro/v1/actions.proto#RepositoryParams:108a9362 -->
Adds or removes an external package repository, after a network-free validation
pre-flight.

`name` identifies the repository and names its file. Then one config block per
backend — `apt`, `dnf`, `pacman`, `zypper` — each with a `disabled` flag that
makes the action a no-op on that backend, so one action can carry a fleet-wide
repository definition and apply correctly everywhere. Only the block matching
this host's backend is used; if none matches, `NOT_APPLICABLE`.

The APT block takes a signing key by URL (https only, body capped at 10 MiB and
**rejected rather than truncated** if larger) or inline.

**Honours `PRESENT` and `ABSENT`.**
<!-- docref: end -->

---

## Application installation

### AppImage

<!-- docref: begin src=agent/internal/executor/action_appimage.go#Executor.executeAppImage:9302672f,contract/proto/cadestro/v1/actions.proto#AppInstallParams:9f00505b -->
Fetches an AppImage to the install directory (default `/opt/appimages`) at mode
`0755`, atomically.

`url` must be https, and a basename containing `..` or a slash is rejected.
`checksum_sha256` is **mandatory**: without it the agent would be installing a
binary whose only authenticity is TLS to a possibly-compromised origin. The
checksum doubles as the idempotency probe — an already-present file with the
right hash is not re-downloaded.

**Honours `PRESENT` and `ABSENT`.**
<!-- docref: end -->

### Deb

<!-- docref: begin src=agent/internal/executor/action_deb.go#Executor.executeDeb:cf2ca7b7 -->
Downloads a `.deb`, reads its canonical package name from the control file, then
installs it through apt so that dependencies resolve — not through a bare `dpkg
-i` that would leave them broken.

Uses `AppInstallParams`; https and a well-formed checksum are required before any
network access or remount. Removal trusts the fetched control file only when the
fetch was checksum-verified, and otherwise derives the package name from the
URL.

**Honours `PRESENT` and `ABSENT`.** `NOT_APPLICABLE` on a host without apt.
<!-- docref: end -->

### RPM

<!-- docref: begin src=agent/internal/executor/action_rpm.go#Executor.executeRpm:8ba617b4 -->
The same shape for `.rpm`, installed through dnf or zypper.

One deliberate difference from deb: removal also requires a reachable, verified
artifact, because rpm package names may contain hyphens and cannot be derived
from a filename reliably. A dead URL on removal is an explicit error rather than
a wrong guess.

**Honours `PRESENT` and `ABSENT`.** `NOT_APPLICABLE` without dnf or zypper.
<!-- docref: end -->

### Flatpak

<!-- docref: begin src=agent/internal/executor/action_flatpak.go#Executor.executeFlatpak:f914131b,contract/proto/cadestro/v1/actions.proto#FlatpakParams:57593e47 -->
Installs or removes a Flatpak application, system-wide or per signed-in desktop
user.

`app_id` is validated so it cannot be flag-shaped. `remote` defaults to
`flathub`. `system_wide` selects one system-scoped operation versus a fan-out
across desktop sessions. `pin` masks the app against automatic updates.

The per-user path is deliberately asymmetric: installing targets only *active*
sessions, while removing targets **every** account that has the app installed.
Uninstalling should not depend on who happens to be logged in.

**Honours `PRESENT` and `ABSENT`.** `NOT_APPLICABLE` without Flatpak.
<!-- docref: end -->

---

## Scripts

### Shell / Script run

<!-- docref: begin src=agent/internal/executor/executor.go#Executor.executeShell:6afece0a,contract/proto/cadestro/v1/actions.proto#ShellParams:144d2cec -->
The general escape hatch, with a detection/remediation/verify flow and
captured command output.

`detection_script` runs first; exit 0 means compliant and the remediation is
skipped; non-zero runs `script` and then re-runs detection to verify. With no
detection script, `script` simply runs. `is_compliance` makes the action
**detection-only** — see [the policy model](policy-model.md#6-compliance-is-detection-only).

`interpreter` defaults to `/bin/sh`. `run_as_root` set to false does **not** mean
"root without escalation" — it means fan out to every active desktop session and
run as that user. `working_directory` must be absolute. `environment` entries are
checked against an allowlist, with `PATH` and locale forced by the runner rather
than accepted from the action. Both scripts are capped at 1 MiB.

Does not read desired state. This is the only action that populates the separate
detection output and compliance flag on its result.
<!-- docref: end -->

---

## Services

<!-- docref: begin src=agent/internal/executor/action_service.go#Executor.executeService:1607f3f9,contract/proto/cadestro/v1/actions.proto#ServiceParams:5292a421 -->
Writes a unit file if one is supplied, converges enable/disable, and drives the
run state.

`unit_name` is validated, and **the agent's own unit is hard-refused** — an
action cannot be used to stop the thing executing it. `unit_content` (up to 64
KiB) is written under `/etc/systemd/system`, hash-compared so an unchanged unit
does not trigger a reload. `enable` set to false actively *disables*, and a
failure to do so is surfaced rather than swallowed.
<!-- docref: end -->

<!-- docref: begin src=contract/proto/cadestro/v1/actions.proto#ServiceUnitState:fcffc2f4 -->
Run state uses its own enum rather than the generic desired state:
`STARTED`, `STOPPED`, `RESTARTED`, or unspecified to leave the running state
alone. `RESTARTED` is intentionally non-idempotent — it restarts every time the
action fires, which is what "restart nightly" needs to mean.
<!-- docref: end -->

---

## Files and directories

### File

<!-- docref: begin src=agent/internal/executor/action_file.go#Executor.executeFile:5b6351ea,contract/proto/cadestro/v1/actions.proto#FileParams:e5ad3099 -->
Writes a file atomically with owner, group, and mode, or deletes it.

`path` must be absolute and is symlink-resolved before use. `content` is capped
at 10 MiB. `managed_block` switches to block mode: `PRESENT` appends the content
if it is not already there, and `ABSENT` strips only that block rather than the
whole file — which is how you own three lines of a config file you do not own.

**Guarded.** Creation refuses a denylist of critical files (`/etc/passwd`,
`/etc/shadow`, `/etc/sudoers`, `/etc/fstab`, `/etc/machine-id`, and others).
Deletion refuses those plus any protected path and any immediate child of `/`.
Both checks run on the resolved *and* the cleaned path, so neither a symlink nor
a `..` can walk out of the guard.

**Honours `PRESENT` and `ABSENT`.**
<!-- docref: end -->

### Directory

<!-- docref: begin src=agent/internal/executor/action_directory.go#Executor.executeDirectory:9675ee4c,contract/proto/cadestro/v1/actions.proto#DirectoryParams:d431ae82 -->
Creates a directory with mode and ownership applied through a no-follow
directory descriptor, or removes it recursively.

`recursive` creates parents. `mode` is octal, and an unparseable value surfaces
as an error rather than being quietly ignored.

**Guarded harder than files**: both creation and removal refuse protected paths
*and* anything under a protected prefix, so `/etc/sudoers.d`, a user's home, and
`/boot/efi` can be neither re-permissioned nor recursively deleted.

**Honours `PRESENT` and `ABSENT`.**
<!-- docref: end -->

---

## Live operations

<!-- docref: begin src=contract/proto/cadestro/v1/agent.proto#SyncDeviceCommand:bd14f8bc,contract/proto/cadestro/v1/agent.proto#RebootDeviceCommand:b5476de2 -->
Reboot and sync take no parameters and are not `ManagedAction`s: the operator
invokes them directly (`RebootDevice`, `SyncDevice` on `ControlService`), and
the server delivers each as a single stream message rather than a
policy-compiled manifest entry. See [live
operations](policy-model.md#3-pull-synchronization).
<!-- docref: end -->

### Reboot

<!-- docref: begin src=agent/internal/executor/action_reboot.go#Executor.executeReboot:36ef5b8c -->
Broadcasts a best-effort notification to logged-in users, then schedules a
reboot five minutes out. It **fails closed when no privileged runner is
configured** rather than reporting a success that never happens.
<!-- docref: end -->

### Sync

Pokes the agent's sync trigger and acknowledges the request; the agent's own
scheduler runs the actual sync, so this reports the request was received, not
that the sync completed.

---

## Users and groups

### User

<!-- docref: begin src=agent/internal/executor/action_user.go#Executor.executeUser:be2dec1c,contract/proto/cadestro/v1/actions.proto#UserParams:ea58c2fa -->
Creates, updates, locks, or removes a local account.

`username` is validated. `uid`/`gid` are applied only when greater than zero, so
the system assigns them by default. `home_dir` must be absolute and is rejected
if it points at a protected path. `shell` defaults to `/bin/bash`, or to nologin
for a disabled or system account. `primary_group` is a name-based alternative to
`gid` and is created if missing. `create_home` is honoured verbatim — an explicit
false is respected — and a missing home is repaired at mode `0700`.

`disabled` is the single driver of account locking; on uid 0 it locks the
password but deliberately leaves the shell intact, so you cannot brick root's
login shell with a policy. `hidden` hides the account from graphical login
screens and is silently skipped on headless machines.

`ssh_authorized_keys` are written through a no-follow directory descriptor with
ownership and mode fixed on the descriptor rather than the path. An embedded
newline in a key is a **fatal error** (it would inject a second authorized key);
an unrecognised key type is skipped with a warning.

`no_password` suppresses both temporary-password generation and password
reporting. It is for system accounts only ever reached via setuid — setting it on
a general-purpose account locks its PAM login path closed.

**Honours `PRESENT` and `ABSENT`.** Generated passwords never travel in result
metadata; they use the authenticated control stream.
<!-- docref: end -->

### Group

<!-- docref: begin src=agent/internal/executor/group.go#Executor.executeGroup:3a0a08cc,contract/proto/cadestro/v1/actions.proto#GroupParams:2389ce38 -->
Creates a group if missing and syncs its membership to **exactly** the declared
set — adding what is missing and removing what is extra. `gid` is applied only
when greater than zero; `system_group` places it below 1000.

`ABSENT` removes the group; every other value sets it up.
<!-- docref: end -->

---

## SSH access

### SSH

<!-- docref: begin src=agent/internal/executor/action_ssh.go#Executor.executeSsh:5d8532f4,contract/proto/cadestro/v1/actions.proto#SshParams:0d2115e1 -->
Grants SSH access to a named set of users, by creating a per-action Linux group
and a `sshd_config.d` drop-in containing a `Match Group` block for it.

`users` requires at least one entry. `allow_pubkey` and `allow_password` set the
corresponding sshd directives for that group only. The configuration is validated
with `sshd -t` before sshd is reloaded, so a bad drop-in cannot lock you out of
the machine.

`ABSENT` removes the group and the drop-in; every other value sets it up.
<!-- docref: end -->

### SSHD

<!-- docref: begin src=agent/internal/executor/action_ssh.go#Executor.executeSshd:57eeeef9,contract/proto/cadestro/v1/actions.proto#SshdParams:4b6b9970 -->
Sets global sshd directives through a numbered drop-in, where `priority` is the
numeric filename prefix that determines load order.

`directives` is a list of key/value pairs. Carriage returns, newlines, and NULs
are rejected in both key and value — sshd_config has no escape syntax, so a
newline would be a directive injection. Validated with `sshd -t` and reloaded.

`ABSENT` removes the drop-in; every other value sets it up.
<!-- docref: end -->

---

## Privilege delegation

<!-- docref: begin src=agent/internal/executor/sudo.go#Executor.executeSudo:8010e22a,contract/proto/cadestro/v1/actions.proto#AdminPolicyParams:c2a89b15 -->
Creates a per-action Linux group and a `/etc/sudoers.d/` drop-in at mode `0440
root:root`, validated with `visudo -c` before installation, and syncs the group's
members.

`users` requires at least one entry. `custom_config` is required when the access
level is `CUSTOM`, with `{group}` substituted for the generated group name.
<!-- docref: end -->

<!-- docref: begin src=contract/proto/cadestro/v1/actions.proto#AdminAccessLevel:b30f7293 -->
`FULL` grants unrestricted access with a password prompt; `LIMITED` restricts to
system-management commands, also with a password; `CUSTOM` carries operator-
authored policy text. The two `TERMINAL_ADMIN_*` levels are passwordless variants
the server's terminal reconciler uses for the dedicated TTY accounts, which have
no password to prompt for — operator-authored policies should use the first
three.
<!-- docref: end -->

> **Caveat, verified in code.** The contract has a `backend` field selecting sudo
> or doas, and the doas rendering exists in the web UI preview. The **agent does
> not read that field**: it always writes sudoers syntax to `/etc/sudoers.d/` and
> always validates with `visudo`. Setting `backend: DOAS` today produces sudoers
> output. The agent's own privilege backend is chosen process-wide by the
> `CADESTRO_PRIVILEGE_BACKEND` environment variable, which is a separate setting.

---

## Credential management

### Local password solution (LPS)

<!-- docref: begin src=agent/internal/executor/lps.go#Executor.executeLps:a3c9b7f9,contract/proto/cadestro/v1/actions.proto#LpsParams:3d4b4a57 -->
Rotates a random password per managed local account — the LAPS pattern — and
reports it to control.

The ordering is the important part: the new password is **reported to control
over the authenticated stream before it is set locally**. A crash between the two leaves control
holding a password that does not work yet, which is recoverable; the reverse
would leave an account nobody can log into.

`usernames` requires at least one. `password_length` is 8–128 and is clamped at
runtime with a warning rather than failing. `complexity` selects alphanumeric or
complex; unspecified falls back to alphanumeric with a warning.
`rotation_interval_days` is 1–365. `grace_period_hours` of zero disables the
post-authentication rotation; above zero, a login during the window triggers a
rotation to limit the exposure of a password that has now been used.

After setting the password the agent notifies the user and kills their sessions
after a 60-second grace. It fails closed **before touching any account** if there
is no local password store or device identity.

`ABSENT` deletes local state; every other value rotates.
<!-- docref: end -->

### Disk encryption (LUKS)

<!-- docref: begin src=agent/internal/executor/secret_transport.go#Executor.executeLuksAction:16f12830,agent/internal/executor/luks.go#Executor.executeLuks:6d5d79d1,contract/proto/cadestro/v1/actions.proto#EncryptionParams:503a1dfa -->
Takes ownership of the machine's LUKS volume and manages its passphrase.

`preshared_key` is delivered only on the authenticated device's mTLS stream and
used once to claim the volume. The agent then generates a managed passphrase,
**confirms with control that it has been stored, verifies the round trip, and
only then removes the old key**. Losing a machine to a half-completed rotation is
the failure this ordering exists to prevent.

`rotation_interval_days` is 1–365. `min_words` (3–10) sets the generated
passphrase length, with the generator enforcing its own floors.
<!-- docref: end -->

<!-- docref: begin src=contract/proto/cadestro/v1/actions.proto#EncryptionDeviceBoundKeyType:c35263d8 -->
Slot 7 is the device-bound key slot and holds one of three things: nothing (the
managed passphrase alone), a TPM2 enrollment for unattended boot, or a
user-defined passphrase entered through the local CLI. The field is
range-checked at the boundary because the agent's switch has a "no device-bound
key" default — an unvalidated out-of-range value would silently downgrade a
requested TPM enrollment instead of being refused.
<!-- docref: end -->

`ABSENT` drops local management state only; the keys stay on the device.

### WiFi

<!-- docref: begin src=agent/internal/executor/secret_transport.go#Executor.executeWifiAction:faa8baea,agent/internal/executor/wifi.go#Executor.executeWifi:cd15c3ba,contract/proto/cadestro/v1/actions.proto#WifiParams:bd82074d -->
Manages a NetworkManager connection profile.

`ssid` and `auth_type` are required, and the auth type selects which secret is
used: `psk` for WPA2/WPA3 Personal, or `client_key` alongside `ca_cert`,
`client_cert`, and `identity` for EAP-TLS. The secret arrives only on the
authenticated device's mTLS stream. `auto_connect`, `hidden`, and
`priority` (−1 to 999, higher wins) control selection behaviour.

Secrets are passed to the underlying tool as multi-line stdin secrets so they
never appear in a process argument list, and the plaintexts are zeroed after
use.

`ABSENT` deletes the profile, probing first so an already-absent profile reports
unchanged rather than failing.
<!-- docref: end -->

---

## Agent self-update

<!-- docref: begin src=agent/internal/executor/agent_update.go#Executor.executeAgentUpdate:b4a12c97,contract/proto/cadestro/v1/actions.proto#AgentUpdateParams:2cff2cac -->
Replaces the agent binary with a newer one, at most once per sync cycle.

The agent picks the entry matching its own architecture and **no matching entry
is a success no-op**, not a failure — so one action can target a mixed fleet.

`allow_downgrade` is required to install an older or equal version; without it,
anti-rollback refuses. `allow_redirect` permits a redirect that changes host or
scheme (GitHub release assets need this); an https-to-http downgrade is refused
regardless, and the signed checksum still gates the bytes either way. Both flags
arrive over the authenticated stream, so each is an explicit operator decision.

Before swapping, the agent runs the candidate binary's `version` and `self-test`
subcommands, then replaces atomically with a backup and signals a graceful
restart.
<!-- docref: end -->

<!-- docref: begin src=contract/proto/cadestro/v1/actions.proto#AgentUpdateArch:e6dec03f -->
Each architecture entry is a binary URL plus a checksum-manifest URL, both https.
The agent requires a detached signature next to the manifest and verifies the
**exact manifest bytes** against its embedded Ed25519 release-signing public key
before trusting any hash in it. That signature chain is the only update-integrity
path — there is no "trust the TLS connection" fallback.
<!-- docref: end -->

---

## What is behind each capability

The agent does not reimplement system administration; it drives the tools
already on the machine. This is what "supported platform" concretely means.

### Package managers

<!-- docref: begin src=sdk/pkg/pkg.go#Backend:11393461,sdk/pkg/pkg.go#Detect:8927e775 -->
Exactly five native backends are implemented: **apt, dnf, dnf5, pacman, and
zypper**. Detection lists the native managers whose primary binaries resolve on
`PATH`; callers select the manager they need. Flatpak is a separate desktop
capability, not a native package backend.

There is **no snap, apk, nix, portage, or homebrew backend.** (Some of those
names appear in the privilege-policy templates as permitted commands — that is a
sudoers grant, not a package backend.)
<!-- docref: end -->

Security-only updates are supported by apt (via unattended-upgrades), dnf/dnf5,
and zypper. Pacman cannot express the concept, so a security-only update reports
`NOT_APPLICABLE` and changes nothing rather than silently widening to a full
upgrade. Flatpak is outside the native update path.

### Everything else

| Capability | Backends implemented | Selection |
|---|---|---|
| Repositories | apt (deb822 `.sources` + keyrings), dnf (`.repo`), pacman (config sections), zypper (`addrepo`) | Follows the detected package backend |
| Services | **systemd only** | Requires both `systemctl` and `/run/systemd/system` |
| Users, groups, passwords | shadow-utils (`useradd`, `usermod`, `chpasswd`, `getent`, `loginctl`) | Single implementation |
| Disk encryption | LUKS via `cryptsetup`, TPM via `systemd-cryptenroll`, volume discovery via `lsblk` | Single implementation |
| WiFi / network | NetworkManager via `nmcli` | `nmcli` on `PATH` |
| Reboot | `shutdown -r` with a relative delay | Debian's `/var/run/reboot-required`, else `needs-restarting -r` |
| Notifications | `wall` for TTY sessions, `notify-send` for graphical ones | Missing tool is a graceful skip |
| Desktop fan-out | `loginctl` + `runuser`, local graphical active sessions only | Single implementation |
| Privilege escalation | sudo or doas | `CADESTRO_PRIVILEGE_BACKEND`, fail-fast if the tool is absent |

<!-- docref: begin src=sdk/sys/service/service.go#Detect:2e00d699 -->
The service backend deserves an explicit statement because the enum used to
promise more: **systemd is the only init system implemented.** OpenRC, runit, and
s6 scaffolds were removed rather than left as stubs that would fail at runtime.
<!-- docref: end -->

<!-- docref: begin src=sdk/sys/repo/repo.go#New:7c11139c -->
The repository manager accepts only the four real repository backends; flatpak
and the zero value are rejected at construction rather than producing a manager
that silently does nothing.
<!-- docref: end -->

<!-- docref: begin src=sdk/sys/exec/detect.go#Detect:1612c467 -->
Privilege detection returns sudo or doas and **never returns "run directly"** —
if neither escalation tool is present, that is an absence to handle, not a
licence to skip escalation.
<!-- docref: end -->

<!-- docref: begin src=sdk/sys/user/user.go#Manager:6e51c335,sdk/sys/encryption/encryption.go#Manager:637550ad,sdk/sys/network/detect.go#Detect:356657a1,sdk/sys/reboot/reboot.go#New:981f19ac,sdk/sys/notify/notify.go#New:9cbfe737,sdk/sys/desktop/desktop.go#New:de9849d1 -->
The user, encryption, network, reboot, notification, and desktop managers each
have exactly one implementation. Where a table row above says "single
implementation", that is what it means: there is no second backend to fall back
to, and no runtime choice to get wrong.
<!-- docref: end -->

### Capabilities present in the SDK with no action type

The system SDK implements several areas that **no action type currently
exposes**: firewall (nftables, firewalld, ufw), antivirus (ClamAV), DNS
(systemd-resolved, NetworkManager), network configuration (NetworkManager,
systemd-networkd), time synchronisation (timedatectl, chrony), CA trust store
management, and SMART disk health.

They are real, tested implementations, but there is no `ActionType` that reaches
them. If you are evaluating Cadestro on the strength of "it can manage the
firewall" — today, through a policy, it cannot.

---

## Where to go next

- [Policy model](policy-model.md) — how these actions are scheduled and
  delivered.
- [Security model](security-model.md) — secret transport and storage, and what the audit
  log records when an action runs.
