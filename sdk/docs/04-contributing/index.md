---
title: Contributing
description: How to work on the SDK — module wiring, the leaf rule, and how changes reach the agent and server.
icon: "🛠️"
---

# Contributing

The SDK is consumed by the agent, control server, and web UI, all modules of
the same repository. The thing most worth understanding before you change the
SDK is how those modules are wired to it and what the SDK is not allowed to
depend on.

{% cards %}
  {% card title="Release coordination" href="/contributing/release-coordination" icon="🚦" %}
  How the modules resolve each other, the leaf rule the architecture test
  enforces, and the tagging conventions.
  {% /card %}
{% /cards %}

## Keeping these docs honest

This documentation is anchored to the code with
[docref](https://github.com/manchtools/open-docref): prose and snippets that
describe a symbol carry a hash of what the author last saw, and `docref check`
(run in CI) fails when the code drifts from the docs. After changing anchored
code, refresh snippets and re-approve claims in the same change. The authoring
rules and the docref workflow are in [`AGENTS.md`](https://github.com/manchtools/cadestro/blob/main/sdk/AGENTS.md).
