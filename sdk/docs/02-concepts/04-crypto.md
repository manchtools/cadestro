---
title: Crypto helpers
label: Crypto
description: AEAD and device certificate helpers.
---

# Crypto helpers

The SDK provides mechanisms, not server policy. The sole system security
design is the workspace target document.

## At-rest encryption

AES-256-GCM helpers require non-empty, domain-separated AAD. Nonces come from
the operating-system CSPRNG. Wrong keys, wrong AAD, malformed ciphertext, and
authentication failure return no plaintext.

Server code binds each secret to its resource context and purpose. Transport
uses the authenticated mTLS stream; at-rest AEAD is not reused as a transport
protocol.

## Certificates

The device generates an Ed25519 identity key and CSR locally. The private key
never leaves the device. Enrollment requires the CA fingerprint pin, and renewal
must preserve CA continuity or require clean re-enrollment.

Ordinary application frames are not separately signed. Direct mTLS authenticates
and protects the agent/control stream.

## Related

- [Client](/concepts/client)
- [Errors](/concepts/errors)
