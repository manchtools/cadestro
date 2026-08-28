## 2026-08-28 Shallow analysis: Applied generic confidential-client guidance instead of Cadestro's actual OIDC flow

**What happened**: I initially recommended retaining the identity-provider client secret even though Cadestro only exchanges an authorization code with PKCE, verifies the resulting ID token, and then uses its own authentication layer. I treated general guidance for confidential web clients as a Cadestro requirement without identifying a concrete operation that needed client authentication.

**What the user said**: "okay so this is what i said \"we dont need to interact with the IDP as cadestro\" and you declined? Cadestro never did any of those things with the IDP, so PKCE was sufficient all along"

**Root cause**: I did not apply the repository rule requiring the existing authentication flow to be traced end to end before recommending that a credential be preserved. That allowed a standards preference to replace evidence about Cadestro's actual requirements.

**Harness fix**: None. AGENTS.md already requires tracing issuance, claims, authentication, refresh, revocation, and invalidation before proposing authentication credentials; the failure was not following that rule.

**Prevention**: Before retaining or introducing any authentication credential, enumerate every runtime operation that consumes it and reject the credential when its only justification is generic provider guidance rather than a required product flow.
