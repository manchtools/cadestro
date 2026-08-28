# Policy model

An administrator authors an action and assigns it directly to a device or a
static device group.

The agent requests synchronization over its outbound stream. The server
compiles the currently assigned actions into a deterministic desired-policy
snapshot. The agent persists that snapshot, schedules each occurrence locally,
and stores results in a durable outbox until the server acknowledges them.

Authored desired state is pulled during synchronization. It is not delivered by
a separate server-push queue.

Compliance is deliberately thin: a shell detection script exits zero for
compliant and non-zero for non-compliant. Compliance actions do not include a
remediation script.
