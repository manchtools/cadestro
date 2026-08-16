## 2026-08-16 Assumed intent: mischaracterized requested audit breadth

**What happened**: I called the whole-codebase sweep scope drift even though the user explicitly requested a thorough sweep beyond the files they had reviewed.

**What the user said**: They had asked Claude to apply the same care to the whole codebase because their own findings came from only six files.

**Root cause**: I focused on the stated simplification goal and failed to carry the request's final breadth requirement into my assessment.

**Harness fix**: This was a one-time intent-reading mistake, so no standing project rule was added; I will reread the complete request before classifying work as out of scope and ask when breadth and audit criteria appear to conflict.

**Prevention**: Future audit assessments will evaluate breadth and review criteria separately instead of treating a correctly broad investigation as scope drift because it used the wrong review axis.

## 2026-08-16 User correction: misclassified prebuilt SDK capability as dead surface

**What happened**: My audit prompt allowed implemented SDK capabilities with no current Cadestro consumer to be treated as removal candidates.

**What the user said**: Removing implemented SDK features merely because they have no consumers is not worthwhile; the capability is intentionally prebuilt.

**Root cause**: I applied application dead-code rules to a reusable SDK without distinguishing its public capability inventory from product wiring.

**Harness fix**: The review rubric for this task now exempts working SDK capabilities from consumer-count-based deletion; no persistent project rule was added because this is a report-only task and the repository has no root harness file in scope.

**Prevention**: SDK removal findings must show duplication, obsolescence, broken behavior, harmful dependency or maintenance cost, or an explicit product decision—not simply zero in-repository callers.
