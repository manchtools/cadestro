# Cadestro Web

The web module is a small Svelte 5 administration console for the retained
core. It supports:

- OIDC login;
- identity provider administration;
- devices, relationships, compliance, and execution history;
- registration token lifecycle;
- package, update, and shell action lifecycle;
- static device groups and membership;
- assignments;
- users, roles, permissions, and session revocation;
- audit events.

Every console operation is shown only when the signed-in user has its
permission. List and history cursors live in the URL so pages survive reloads
and browser navigation. Set `PUBLIC_CONTROL_URL` to an HTTPS URL for a
cross-origin control service; leaving it unset uses the web origin.

It intentionally has no self-service user portal, terminal, inventory,
OSQuery, log viewer, action sets, dynamic groups, SCIM, or API-token UI.

```bash
npm ci
npm run check
npm test
npm run build
```

The generated Connect client comes from `../contract/gen/ts`.
