# Security policy

Report vulnerabilities privately to the repository maintainers. Do not open a
public issue containing exploit details, credentials, or deployment data.

Administrators authenticate through OIDC. Agents bootstrap with a one-time
registration token, generate an Ed25519 key locally, pin the control CA, and
then authenticate directly to the agent listener with mTLS.

The public and agent listeners are distinct TLS endpoints. Validation runs
before authentication and authorization. Device identity comes from the
verified client certificate and active certificate serial.

CA keys, TLS keys, session signing keys, and the SQLite database require
owner-only storage.

Shell actions run as root and are therefore equivalent to administrator code
execution on assigned devices. Only trusted administrators should be able to
author or assign them. Agents accept desired actions only from their
authenticated control connection.

Traefik is the only component that publishes host ports in the reference
deployment. It terminates browser/API TLS and passes agent TLS through without
terminating the device certificate exchange.
