# Cadestro Web

The web module is a small Svelte 5 administration console for the retained
core. It supports:

- OIDC login;
- devices and their status, compliance, and results;
- one-time registration tokens;
- package, update, and shell actions;
- static device groups and membership;
- assignments.

It intentionally has no self-service user portal, terminal, inventory,
OSQuery, log viewer, action sets, dynamic groups, SCIM, or API-token UI.

```bash
npm ci
npm run check
npm test
npm run build
```

The generated Connect client comes from `../contract/gen/ts`.
