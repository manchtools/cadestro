---
title: Cadestro SDK
label: Getting started
description: Package management, safe command execution, and agent infrastructure for Cadestro.
---

# Cadestro SDK

The SDK is the reusable Go module used by Cadestro's agent and control server.
Its supported surface is intentionally narrow: native package management,
non-interactive command execution, agent filesystem and systemd integration,
certificate enrollment, at-rest encryption, and logging.

```bash
go get github.com/manchtools/cadestro/sdk@latest
```

Cadestro is pre-1.0, so minor releases may contain breaking changes.
