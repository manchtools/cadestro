# Cadestro Contract

The contract contains the protobuf and Connect definitions shared by the
control plane, agent, and web console.

The core exposes:

- one bidirectional agent stream;
- enrollment and certificate renewal;
- OIDC browser authentication;
- devices and one-time registration tokens;
- package, update, and shell actions;
- static device groups and assignments;
- execution results, compliance findings, and audit events.

Generate code and run the standalone gate with:

```bash
make generate
./scripts/verify.sh
```

Generated Go and TypeScript files are outputs, not editing surfaces.
