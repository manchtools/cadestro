## 2026-08-28 Shallow analysis: Applied generic confidential-client guidance instead of Cadestro's actual OIDC flow

**What happened**: I initially recommended retaining the identity-provider client secret even though Cadestro only exchanges an authorization code with PKCE, verifies the resulting ID token, and then uses its own authentication layer. I treated general guidance for confidential web clients as a Cadestro requirement without identifying a concrete operation that needed client authentication.

**What the user said**: "okay so this is what i said \"we dont need to interact with the IDP as cadestro\" and you declined? Cadestro never did any of those things with the IDP, so PKCE was sufficient all along"

**Root cause**: I did not apply the repository rule requiring the existing authentication flow to be traced end to end before recommending that a credential be preserved. That allowed a standards preference to replace evidence about Cadestro's actual requirements.

**Harness fix**: None. AGENTS.md already requires tracing issuance, claims, authentication, refresh, revocation, and invalidation before proposing authentication credentials; the failure was not following that rule.

**Prevention**: Before retaining or introducing any authentication credential, enumerate every runtime operation that consumes it and reject the credential when its only justification is generic provider guidance rather than a required product flow.

## 2026-08-29 Shallow analysis: Conflated refresh-token expiry with session invalidation

**What happened**: I described version-based refresh/logout behavior in a way that implied an expired refresh token might still be accepted or handled "just as before".

**What the user said**: "an expored refresh token should never work "just as before" that is bad design. A expired refresh token should always require a new login"

**Root cause**: I conflated JWT expiration validation with session-version rotation/revocation semantics instead of treating expiry as an unconditional authentication boundary.

**Harness fix**: None; the existing AGENTS.md already requires tracing issuance, claim construction, refresh, revocation, and invalidation and forbids simplifying security measures.

**Prevention**: Verify and state expiration rejection independently before discussing replay, rotation, logout, or session-version behavior; never describe an expired refresh token as reusable or idempotently valid.

## 2026-08-29 Wrong scope: Reduced core authorization to one hardcoded permission

**What happened**: I chose a first-user-only `REVOKE_USER_SESSIONS` grant and explicitly excluded role management, even though user-controlled roles and permissions are a core administrative expectation.

**What the user said**: "hm okay, we should restore the permission and role system. Thats something every user will expect to be able to control"

**Root cause**: I applied product-feature descoping to the control plane's authority model instead of separating optional device features from expected administrator access control.

**Harness fix**: record the operator ruling in AGENTS.md.

**Prevention**: before removing or replacing an authority system during descoping, classify whether administrators still need to delegate every retained capability; if yes, preserve user-manageable roles and permissions.

## 2026-08-29 Assumed intent: Required DB-fresh permissions on every request

**What happened**: I required every authenticated request to reload the user's session version and effective permissions from SQLite, although the operator accepts access-token-lifetime permission freshness to avoid that database lookup.

**What the user said**: "could we not bake in the user roles in the access token? Id say that good enough for security and avoids a DB lookup"

**Root cause**: I inherited the archived DB-fresh authority semantics instead of first obtaining the explicit permission-freshness ruling required by AGENTS.md.

**Harness fix**: add the operator ruling to AGENTS.md.

**Prevention**: decide and record the permitted staleness window before designing token claims or request authorization; derive the enforcement path from that ruling.

## 2026-08-30 Wrong scope: Presumed immutable roles and reversible token disabling

**What happened**: I preserved immutable system roles and a separate registration-token disabled state as enterprise lifecycle features.

**What the user said**: "we should just preseed roles and let the admin be able to delete any role that exists if they dont want any roles at all" and "I honestly dont see any value in disabling a token if we cant enable it again."

**Root cause**: I presumed enterprise lifecycle requirements instead of following the requested ordinary seed-role and deletion-only token model.

**Harness fix**: record the operator ruling in AGENTS.md.

**Prevention**: treat seeded defaults as ordinary deletable data and use deletion as the sole registration-token revocation state unless the operator explicitly rules otherwise.

## 2026-08-30 Shallow analysis: Invented a separate storage proto for action definitions

**What happened**: The orchestrator said a separate storage protobuf was the only compliant action-blob option and left action storage unchanged, missing the operator's intended direct reuse of existing action proto messages.

**What the user said**: "Not a seperate storage protobuf. Just marshall the proto to binary and store it? we have the action types arealdy as proto messages".

**Root cause**: I applied the general API/storage separation rule without recognizing the operator's bounded action-storage exception, inventing a parallel storage message that defeats the simplification.

**Harness fix**: add a recorded operator ruling to root AGENTS.md that action definitions may persist the existing concrete action proto message as a binary blob with the action type stored separately; do not create a parallel storage proto.

**Prevention**: trace the bounded payload/query surface and ask whether direct wire/storage coupling is the explicit ruling before proposing duplicate schema types.

## 2026-08-30 Wrong bound: Treated cross-origin cookies as a blocking complication

**What happened**: We treated cross-origin cookie transport as a blocking complication without tracing the already credentialed exact-origin CORS middleware and browser credential mode.

**What the user said**: “The only complication is Cadestro’s configurable cross-origin control URL: cookie transport would then require credentials: include and credentialed CORS. With the default same-origin deployment, the cookie is the leaner design. -> Well the JS can set a cookie directly on the control URL instead of the weburl? If cors is correctly configured i dont see a probleme. Please implement my simplification”.

**Root cause**: We failed to distinguish a control-origin Set-Cookie response from JavaScript setting another origin's cookie and treated a supported credentialed CORS deployment as a blocker.

**Harness fix**: add the recorded OIDC control-origin cookie ruling to root AGENTS.md.

**Prevention**: trace cookie origin, browser credential mode, and exact-origin CORS behavior before rejecting cookie-backed transaction state for cross-origin deployments.

## 2026-08-30 Ignored project rule: Preserved a redundant action type discriminator

**What happened**: During the action-blob refactor, we preserved `Action.type`, `ManagedAction.type`, and `CreateActionRequest.type` even though each already has a concrete `oneof params` whose selected arm is the authoritative action kind.

**What the user said**: "I see, but why is the action type of the Action message not beeing infered by what is passed to params?"

**Root cause**: We carried the old relational discriminator through the storage refactor instead of reapplying Stallion proto §11 (`oneof` of concrete messages beats an enum kind plus parameters) to the entire contract class.

**Harness fix**: None; the binding Stallion proto rule already prohibits this redundant shape.

**Prevention**: whenever a proto gains or retains a concrete oneof, query the descriptor for sibling enum discriminators in every message using that oneof and remove them unless they encode independent state.

## 2026-08-30 Ignored project rule: Invented an update-time need for ActionType

**What happened**: I claimed the redundant `ActionType` could remain useful for detecting an attempt to change an existing action's parameter kind, despite the update path already loading the stored action before writing it.

**What the user said**: "The only thing i can see ActionType enum beeing usefull for is detecting a param change to an already existing action. But we could also just pull the dataset before updating to check that."

**Root cause**: I justified a stored discriminator from a derived invariant without tracing the existing read-before-update flow or applying the rule that the concrete `oneof` arm is authoritative.

**Harness fix**: None; Stallion's ladder and proto rules already require checking the existing flow and removing an enum discriminator duplicated by a concrete `oneof`.

**Prevention**: Before preserving derived metadata for validation, trace the full mutation path and compare authoritative values already available there; add separate storage only when it enables a retained query that cannot reasonably use the authoritative value.
