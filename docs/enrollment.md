# Enrollment

1. An administrator creates a one-time registration token.
2. On the device, a root operator gives the token, control URL, and CA
   fingerprint to `cadestrod enroll`.
3. The local command talks to the agent's owner-only Unix socket.
4. The agent generates an Ed25519 key and CSR locally.
5. Control consumes the token, signs the CSR, and returns the device ID,
   certificate, CA certificate, agent URL, and CA fingerprint.
6. The agent verifies the pin, stores its credentials, and opens the outbound
   mTLS stream.

Certificate renewal proves possession of the existing private key and promotes
the new certificate only after it is used successfully.
