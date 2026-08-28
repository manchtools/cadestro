# Self-hosted deployment

1. Copy `.env.example` to `.env`.
2. Set the two hostnames, ACME email, image tag, and OIDC client values.
3. Run `./deploy.sh`.

The browser/API hostname terminates public TLS at Traefik. The agent hostname
passes TLS through to Cadestro so the control server can authenticate device
certificates directly.
