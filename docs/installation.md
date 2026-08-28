# Installation

The reference deployment requires Docker Compose, OpenSSL, two DNS names, an
ACME email, and an OIDC client.

1. Copy `server/deploy/.env.example` to `server/deploy/.env`.
2. Set the browser/API domain, agent domain, ACME email, image tag, OIDC issuer,
   client ID, and client secret.
3. Point both DNS names at the host.
4. Run `server/deploy/deploy.sh`.

`setup.sh` parses only the documented environment keys, generates the
internal CA, control certificate, encryption key, and session signing key, and
writes owner-only service configuration.

The browser/API hostname terminates TLS at Traefik. The agent hostname passes
TLS through to the control process so it can verify device certificates
directly.
