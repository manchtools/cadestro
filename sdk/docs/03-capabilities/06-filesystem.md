---
title: Filesystem
label: Filesystem
description: Read, write, and manage files and directories with permissions and ownership — TOCTOU-safe, never following a symlink out of bounds.
icon: "💾"
---

# Filesystem

`sys/fs` is the SDK's file primitive: read, write, copy, remove, set
mode/ownership, and remount-rw — done safely. It is a single-tool capability, so
`New` takes only a Runner, no Backend.

## Construct a manager

```go
r, err := exec.NewRunner(exec.Sudo) // writes to root-owned paths need root
if err != nil {
    return err
}
m, err := fs.New(r)
if err != nil {
    return err
}
```

## Read and write

```go
data, err := m.ReadFile(ctx, "/etc/hostname")
err = m.WriteFile(ctx, "/etc/cadestro/agent.conf", []byte(cfg), fs.WriteOptions{
    Mode:  0o644,
    Owner: "root",
    Group: "root",
})
ok, err := m.Exists(ctx, "/etc/cadestro")
entries, err := m.ReadDir(ctx, "/etc/cadestro")
```

<!-- docref: begin src=sys/fs/write.go#manager.WriteFile:19e446ca -->
`WriteFile` creates or replaces the file and applies the requested mode, owner,
and group in one call, through the same privilege-keyed safe backend described
below.
<!-- docref: end -->

## Directories, permissions, ownership

```go
err := m.Mkdir(ctx, "/var/lib/cadestro/state", fs.MkdirOptions{Mode: 0o750, Recursive: true})
err = m.SetMode(ctx, "/var/lib/cadestro/state", 0o700)
err = m.SetOwnership(ctx, "/var/lib/cadestro/state", "cadestro", "cadestro")
err = m.Remove(ctx, "/var/lib/cadestro/state/stale.tmp")
```

## Why use this instead of `os`

<!-- docref: begin src=sys/fs/fs.go#New:f3eda3e4 -->
`New` returns the filesystem Manager over the injected Runner; a nil Runner is
rejected.
<!-- docref: end -->

<!-- docref: begin src=sys/fs/fs.go#manager.direct:2f9b9073 -->
The operations are privilege-backend-keyed: as root (a `Direct` Runner) they
take a TOCTOU-safe, fd-anchored path — each step operates on an open directory
handle, so a symlink swapped in mid-operation can't redirect a write or delete
out of bounds; when escalation is via sudo, the same operations are driven
through the escalated tool.
<!-- docref: end -->

{% callout type="info" title="Read-only roots" %}
<!-- docref: begin src=sys/fs/mount.go#manager.IsReadOnly:1edb9acd,sys/fs/mount.go#manager.RemountRW:3eadcb2c,sys/fs/protected.go#IsUnderProtectedPrefix:8f8db390 -->
`IsReadOnly` (an unprivileged `findmnt` probe) and `RemountRW` (an escalated
`mount -o remount,rw`) handle the immutable-root case (ostree, a read-only
`/usr`): check, remount read-write for the change. There is no remount-ro
helper — restoring the read-only state afterwards is the caller's job.
Mutations refuse to write into protected system subtrees they don't own.
<!-- docref: end -->
{% /callout %}

## Related

- [Architecture](/concepts/architecture) — the injected Runner this builds on.
- [Remote sources](/capabilities/remote) — fetch a file from HTTPS/Git/S3 into a
  managed path.
