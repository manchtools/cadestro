# Cadestro Agent

The agent runs as root on a managed Linux device. It opens an outbound mTLS
bidirectional stream to Cadestro, pulls assigned desired actions, persists them
in SQLite, executes due work even while disconnected, and uploads results after
reconnection.

Supported actions:

- install or remove one native package;
- refresh metadata and perform a full system update;
- run a bounded, non-interactive root shell script;
- run a detection-only shell script and report the detection result.

The agent has no inbound network listener. Initial enrollment is driven through
its owner-only local Unix socket with a one-time registration token and an
out-of-band CA fingerprint. The device generates its Ed25519 private key
locally; it never leaves the device.

Build and verify:

```bash
GOWORK=off go build ./...
./scripts/verify.sh
```

The local `go.work` is for editing only.
